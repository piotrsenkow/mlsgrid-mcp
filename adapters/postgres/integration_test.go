//go:build integration

// Integration tests run the real adapter against a disposable testcontainers
// Postgres seeded from the *pinned* mlsgrid-sync schema migration
// (testdata/contract/0001_init.sql) plus testdata/seed.sql. Applying the
// upstream schema at a pinned tag is the cross-repo contract test: if
// mlsgrid-sync's schema drifts from what this adapter reads, these tests break
// and the pin (testdata/contract/PIN) must be refreshed deliberately.
//
// No test may touch a real database or api.mlsgrid.com; the container is created
// and destroyed within the package.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
	"github.com/piotrsenkow/mlsgrid-mcp/server"
)

const testSchema = "mlsgrid"

var (
	testDSN     string
	testAdapter *Adapter
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("mlsgrid_mcp_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "starting postgres container:", err)
		os.Exit(1)
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	testDSN, err = ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, "container connection string:", err)
		os.Exit(1)
	}
	if err := setupFixture(ctx, testDSN); err != nil {
		fmt.Fprintln(os.Stderr, "seeding fixture:", err)
		os.Exit(1)
	}

	testAdapter, err = New(ctx, testDSN, Options{Schema: testSchema})
	if err != nil {
		fmt.Fprintln(os.Stderr, "opening adapter:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = testAdapter.Close()
	os.Exit(code)
}

// setupFixture applies the pinned contract migration and the seed into the test
// schema on a single read-write connection. The migration uses unqualified
// names, so search_path is pinned to the schema first — exactly how
// mlsgrid-sync's migrator applies it.
func setupFixture(ctx context.Context, dsn string) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+pgxQuoteIdent(testSchema)); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO "+pgxQuoteIdent(testSchema)); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}
	for _, f := range []string{"testdata/contract/0001_init.sql", "testdata/seed.sql"} {
		body, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		// No args → pgx uses the simple protocol, which runs the whole
		// multi-statement script in one round trip.
		if _, err := conn.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("exec %s: %w", f, err)
		}
	}
	return nil
}

// expectedOrder is the full keyset ordering (modification_timestamp DESC,
// listing_key DESC) of the seed. MRD1007/MRD1001 share a timestamp, so their
// relative order (key DESC) exercises the cursor tiebreak.
var expectedOrder = []string{
	"MRD1008", "MRD1004", "CML3001", "MRD2001", "MRD1002", "MRD1007",
	"MRD1001", "MRD1010", "MRD1003", "MRD1005", "MRD1006", "MRD1009",
}

func keysOf(items []mls.ListingSummary) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = s.ListingKey
	}
	return out
}

func TestSearchOrderAndDefaults(t *testing.T) {
	page, err := testAdapter.SearchListings(context.Background(), mls.SearchQuery{})
	if err != nil {
		t.Fatalf("SearchListings: %v", err)
	}
	if got := keysOf(page.Items); !equalStrings(got, expectedOrder) {
		t.Errorf("order:\n got  %v\n want %v", got, expectedOrder)
	}
	if page.NextCursor != "" {
		t.Errorf("NextCursor = %q, want empty (12 < default 25)", page.NextCursor)
	}
	if page.Total != -1 {
		t.Errorf("Total = %d, want -1 (unknown by design)", page.Total)
	}
	want := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	if !page.DataAsOf.Equal(want) {
		t.Errorf("DataAsOf = %v, want %v", page.DataAsOf, want)
	}
}

func TestSearchKeysetPagination(t *testing.T) {
	ctx := context.Background()
	var got []string
	cursor := ""
	for page := 0; page < 10; page++ { // generous safety bound
		p, err := testAdapter.SearchListings(ctx, mls.SearchQuery{Limit: 5, Cursor: cursor})
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if len(p.Items) > 5 {
			t.Fatalf("page %d returned %d > limit 5", page, len(p.Items))
		}
		got = append(got, keysOf(p.Items)...)
		if p.NextCursor == "" {
			break
		}
		cursor = p.NextCursor
	}
	if !equalStrings(got, expectedOrder) {
		t.Errorf("paged order:\n got  %v\n want %v", got, expectedOrder)
	}
}

func TestSearchFilters(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		q    mls.SearchQuery
		want int
	}{
		{"city Chicago", mls.SearchQuery{Area: mls.Area{City: "chicago"}}, 7}, // case-insensitive
		{"active only", mls.SearchQuery{Statuses: []string{"Active"}}, 8},
		{"min price 600k", mls.SearchQuery{MinPrice: 600000}, 3},
		{"min beds 4", mls.SearchQuery{MinBeds: 4}, 4},
		{"subtype condominium", mls.SearchQuery{PropertyTypes: []string{"Condominium"}}, 4},
		{"keyword condo", mls.SearchQuery{Keywords: "condo"}, 2},
		{"max dom 30", mls.SearchQuery{MaxDaysOnMarket: 30}, 6},
		{"min year 2015", mls.SearchQuery{MinYearBuilt: 2015}, 3},
		{"min living 3000", mls.SearchQuery{MinLivingArea: 3000}, 2},
		{"county DuPage", mls.SearchQuery{Area: mls.Area{County: "DuPage"}}, 2},
		{"active + Chicago + <=400k", mls.SearchQuery{
			Area: mls.Area{City: "Chicago"}, Statuses: []string{"Active"}, MaxPrice: 400000,
		}, 5}, // MRD1001(350k), MRD1002(250k), MRD1009(275k), MRD2001(300k), CML3001(310k); MRD1005(675k) drops
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := testAdapter.SearchListings(ctx, tc.q)
			if err != nil {
				t.Fatalf("SearchListings: %v", err)
			}
			if len(p.Items) != tc.want {
				t.Errorf("count = %d, want %d (keys %v)", len(p.Items), tc.want, keysOf(p.Items))
			}
		})
	}
}

func TestSearchCoordinatesNullable(t *testing.T) {
	ctx := context.Background()
	byKey := func(key string) mls.ListingSummary {
		p, err := testAdapter.SearchListings(ctx, mls.SearchQuery{Keywords: ""})
		if err != nil {
			t.Fatalf("SearchListings: %v", err)
		}
		for _, s := range p.Items {
			if s.ListingKey == key {
				return s
			}
		}
		t.Fatalf("key %s not found", key)
		return mls.ListingSummary{}
	}
	if s := byKey("MRD1001"); s.Latitude == nil || *s.Latitude == 0 {
		t.Errorf("MRD1001 latitude = %v, want non-nil coordinate", s.Latitude)
	}
	if s := byKey("MRD1002"); s.Latitude != nil {
		t.Errorf("MRD1002 latitude = %v, want nil (seed omits coordinates)", *s.Latitude)
	}
}

func TestGetListingByKey(t *testing.T) {
	d, err := testAdapter.GetListing(context.Background(), mls.ListingRef{Key: "MRD1003"}, mls.ListingOptions{})
	if err != nil {
		t.Fatalf("GetListing: %v", err)
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"listing_key", d.ListingKey, "MRD1003"},
		{"mls_number", d.MLSNumber, "1003"},
		{"status", d.StandardStatus, "Closed"},
		{"list_price", d.ListPrice, int64(500000)},
		{"original_list_price", d.OriginalListPrice, int64(520000)},
		{"close_price", d.ClosePrice, int64(485000)},
		{"bedrooms", d.Bedrooms, 4},
		{"bathrooms_full", d.BathroomsFull, 3},
		{"living_area", d.LivingArea, int64(2600)},
		{"lot_size_acres", d.LotSizeAcres, 0.25},
		{"association_fee", d.AssociationFee, int64(0)},
		{"tax_annual_amount", d.TaxAnnualAmount, int64(8200)},
		{"tax_year", d.TaxYear, 2025},
		{"list_agent_name", d.ListAgentName, "Jane Broker"},
		{"list_office_name", d.ListOfficeName, "North Shore Realty"},
		{"photos_count", d.PhotosCount, 25},
		{"virtual_tour_url", d.VirtualTourURL, "https://tours.example.com/1003"},
		{"street_name", d.Address.StreetName, "N Ridge Ave"},
		{"city", d.Address.City, "Evanston"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	want := time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)
	if !d.DataAsOf.Equal(want) {
		t.Errorf("DataAsOf = %v, want %v", d.DataAsOf, want)
	}
}

func TestGetListingRawGating(t *testing.T) {
	ctx := context.Background()
	off, err := testAdapter.GetListing(ctx, mls.ListingRef{Key: "MRD1003"}, mls.ListingOptions{IncludeRaw: false})
	if err != nil {
		t.Fatalf("GetListing: %v", err)
	}
	if off.Raw != nil {
		t.Errorf("Raw = %v, want nil when include_raw is off", off.Raw)
	}
	on, err := testAdapter.GetListing(ctx, mls.ListingRef{Key: "MRD1003"}, mls.ListingOptions{IncludeRaw: true})
	if err != nil {
		t.Fatalf("GetListing: %v", err)
	}
	if on.Raw == nil {
		t.Fatal("Raw = nil, want populated when include_raw is on")
	}
	if on.Raw["MRD_extra"] != "legacy-field" {
		t.Errorf("Raw[MRD_extra] = %v, want legacy-field", on.Raw["MRD_extra"])
	}
}

func TestGetListingByMLSNumber(t *testing.T) {
	ctx := context.Background()

	// Unique number resolves.
	if d, err := testAdapter.GetListing(ctx, mls.ListingRef{MLSNumber: "1003"}, mls.ListingOptions{}); err != nil {
		t.Fatalf("unique number: %v", err)
	} else if d.ListingKey != "MRD1003" {
		t.Errorf("number 1003 → %s, want MRD1003", d.ListingKey)
	}

	// Number shared across two feeds is ambiguous without a system.
	if _, err := testAdapter.GetListing(ctx, mls.ListingRef{MLSNumber: "9999"}, mls.ListingOptions{}); !errors.Is(err, mls.ErrAmbiguousRef) {
		t.Errorf("ambiguous number err = %v, want ErrAmbiguousRef", err)
	}

	// Scoping by originating system disambiguates.
	if d, err := testAdapter.GetListing(ctx, mls.ListingRef{MLSNumber: "9999", OriginatingSystem: "mred"}, mls.ListingOptions{}); err != nil {
		t.Fatalf("scoped mred: %v", err)
	} else if d.ListingKey != "MRD2001" {
		t.Errorf("9999@mred → %s, want MRD2001", d.ListingKey)
	}
	if d, err := testAdapter.GetListing(ctx, mls.ListingRef{MLSNumber: "9999", OriginatingSystem: "connectmls"}, mls.ListingOptions{}); err != nil {
		t.Fatalf("scoped connectmls: %v", err)
	} else if d.ListingKey != "CML3001" {
		t.Errorf("9999@connectmls → %s, want CML3001", d.ListingKey)
	}
}

func TestGetListingNotFound(t *testing.T) {
	ctx := context.Background()
	if _, err := testAdapter.GetListing(ctx, mls.ListingRef{Key: "NOPE"}, mls.ListingOptions{}); !errors.Is(err, mls.ErrNotFound) {
		t.Errorf("missing key err = %v, want ErrNotFound", err)
	}
	if _, err := testAdapter.GetListing(ctx, mls.ListingRef{}, mls.ListingOptions{}); !errors.Is(err, mls.ErrNotFound) {
		t.Errorf("empty ref err = %v, want ErrNotFound", err)
	}
}

// TestFreshnessAndCapabilities is a light smoke over the B-M1 reads against the
// real seeded schema, confirming the whole adapter agrees with the contract.
func TestFreshnessAndCapabilities(t *testing.T) {
	ctx := context.Background()

	f, err := testAdapter.Freshness(ctx)
	if err != nil {
		t.Fatalf("Freshness: %v", err)
	}
	if f.TotalListings != 12 {
		t.Errorf("TotalListings = %d, want 12", f.TotalListings)
	}
	if f.SchemaContractVersion != "1.0.0" {
		t.Errorf("contract = %q, want 1.0.0", f.SchemaContractVersion)
	}

	caps, err := testAdapter.Capabilities(ctx)
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if !equalStrings(caps.OriginatingSystems, []string{"connectmls", "mred"}) {
		t.Errorf("OriginatingSystems = %v, want [connectmls mred]", caps.OriginatingSystems)
	}
	if !caps.Geo {
		t.Error("Geo = false, want true (seed has coordinates)")
	}
	if !caps.PriceHistory {
		t.Error("PriceHistory = false, want true (seed has listing_event rows)")
	}
	if caps.OpenHouses {
		t.Error("OpenHouses = true, want false (seed has no open houses)")
	}
}

// TestContractMajorMismatch proves the startup assertion rejects an incompatible
// schema — the safety property the pin protects.
func TestContractMajorMismatch(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDSN)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	const badSchema = "contract_bad"
	for _, stmt := range []string{
		"CREATE SCHEMA IF NOT EXISTS " + pgxQuoteIdent(badSchema),
		fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s.schema_meta (key text primary key, value text)", pgxQuoteIdent(badSchema)),
		fmt.Sprintf("INSERT INTO %s.schema_meta(key,value) VALUES('contract_version','2.0.0') ON CONFLICT (key) DO UPDATE SET value='2.0.0'", pgxQuoteIdent(badSchema)),
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prep: %v", err)
		}
	}

	if _, err := New(ctx, testDSN, Options{Schema: badSchema}); err == nil {
		t.Error("New succeeded on a major-version-2 schema, want mismatch error")
	}
}

// TestEndToEndOverMCP proves the full pipe for the B-M2 tools: MCP client →
// tool → real adapter → seeded database, over the SDK's in-memory transport.
func TestEndToEndOverMCP(t *testing.T) {
	ctx := context.Background()
	srv, err := server.New(testAdapter, server.WithInfo("mlsgrid-mcp-it", "test"))
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	clientT, serverT := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "it-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	// search_listings: Active in Chicago should return the 6 active Chicago rows.
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_listings",
		Arguments: map[string]any{"city": "Chicago", "statuses": []string{"Active"}},
	})
	if err != nil {
		t.Fatalf("search CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("search tool error: %+v", res.Content)
	}
	var search struct {
		Count    int `json:"count"`
		Listings []struct {
			ListingKey string `json:"listing_key"`
		} `json:"listings"`
	}
	decodeStructured(t, res.StructuredContent, &search)
	if search.Count != 6 {
		t.Errorf("search count = %d, want 6 (active Chicago)", search.Count)
	}

	// get_listing: fetch one by key and confirm it round-trips through the wire.
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_listing",
		Arguments: map[string]any{"listing_key": "MRD1003"},
	})
	if err != nil {
		t.Fatalf("get_listing CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("get_listing tool error: %+v", res.Content)
	}
	var detail struct {
		ListingKey string `json:"listing_key"`
		ListPrice  int64  `json:"list_price"`
		ClosePrice int64  `json:"close_price"`
	}
	decodeStructured(t, res.StructuredContent, &detail)
	if detail.ListingKey != "MRD1003" || detail.ListPrice != 500000 || detail.ClosePrice != 485000 {
		t.Errorf("detail = %+v, want MRD1003/500000/485000", detail)
	}
}

func decodeStructured(t *testing.T, v any, dst any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
