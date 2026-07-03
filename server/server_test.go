package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// fakeSource is a canned mls.Source for protocol-level tests. Fields let a test
// pin the value (or error) each tool returns so the wire path — input decoding,
// handler, structured output — can be exercised without a database.
type fakeSource struct {
	freshness   mls.Freshness
	freshErr    error
	search      mls.Page[mls.ListingSummary]
	searchErr   error
	lastSearch  mls.SearchQuery // captures the decoded query the tool passed in
	listing     *mls.ListingDetail
	listErr     error
	lastRef     mls.ListingRef
	lastOpts    mls.ListingOptions
	history     *mls.PriceHistory
	histErr     error
	lastHistRef mls.ListingRef
	comps       *mls.CompsResult
	compsErr    error
	lastComps   mls.CompsQuery
	stats       *mls.MarketStats
	statsErr    error
	lastStats   mls.StatsQuery
	openHouses  mls.OpenHouseResult
	ohErr       error
	lastOH      mls.OpenHouseQuery
	sqlResult   *mls.ResultSet
	sqlErr      error
	lastSQL     string
	lastMaxRows int
	closed      bool
}

func (f *fakeSource) Capabilities(context.Context) (mls.Capabilities, error) {
	return mls.Capabilities{SchemaContractVersion: f.freshness.SchemaContractVersion}, nil
}
func (f *fakeSource) Freshness(context.Context) (mls.Freshness, error) {
	return f.freshness, f.freshErr
}
func (f *fakeSource) SearchListings(_ context.Context, q mls.SearchQuery) (mls.Page[mls.ListingSummary], error) {
	f.lastSearch = q
	return f.search, f.searchErr
}
func (f *fakeSource) GetListing(_ context.Context, ref mls.ListingRef, opts mls.ListingOptions) (*mls.ListingDetail, error) {
	f.lastRef = ref
	f.lastOpts = opts
	return f.listing, f.listErr
}
func (f *fakeSource) FindComparables(_ context.Context, q mls.CompsQuery) (*mls.CompsResult, error) {
	f.lastComps = q
	return f.comps, f.compsErr
}
func (f *fakeSource) MarketStats(_ context.Context, q mls.StatsQuery) (*mls.MarketStats, error) {
	f.lastStats = q
	return f.stats, f.statsErr
}
func (f *fakeSource) PriceHistory(_ context.Context, ref mls.ListingRef) (*mls.PriceHistory, error) {
	f.lastHistRef = ref
	return f.history, f.histErr
}
func (f *fakeSource) OpenHouses(_ context.Context, q mls.OpenHouseQuery) (mls.OpenHouseResult, error) {
	f.lastOH = q
	return f.openHouses, f.ohErr
}
func (f *fakeSource) QueryReadOnly(_ context.Context, query string, maxRows int) (*mls.ResultSet, error) {
	f.lastSQL = query
	f.lastMaxRows = maxRows
	return f.sqlResult, f.sqlErr
}
func (f *fakeSource) Close() error { f.closed = true; return nil }

func sampleFreshness() mls.Freshness {
	wm := time.Date(2026, 7, 3, 6, 18, 3, 0, time.UTC)
	return mls.Freshness{
		SchemaContractVersion: "1.0.0",
		TotalListings:         8322,
		DataAsOf:              wm,
		GeneratedAt:           time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC),
		Cursors: []mls.ResourceCursor{{
			Resource:          "Property",
			OriginatingSystem: "mred",
			StoredRows:        8322,
			Watermark:         &wm,
			BackfillComplete:  true,
		}},
		ListingStatusCounts: []mls.StatusCount{
			{Status: "Active", Count: 4668},
			{Status: "Closed", Count: 1619},
		},
		MediaCounts: []mls.StatusCount{{Status: "skipped", Count: 207028}},
	}
}

// connect wires an in-memory client to a server backed by source and returns
// the client session.
func connect(t *testing.T, source mls.Source) *mcp.ClientSession {
	return connectOpts(t, source)
}

// connectSQL is connect with the opt-in query_sql tool enabled.
func connectSQL(t *testing.T, source mls.Source) *mcp.ClientSession {
	return connectOpts(t, source, WithSQL(true))
}

func connectOpts(t *testing.T, source mls.Source, extra ...Option) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	srv, err := New(source, append([]Option{WithInfo("mlsgrid-mcp-test", "test")}, extra...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	clientT, serverT := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestNewRejectsNilSource(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("expected error for nil source")
	}
}

func TestListToolsExposesRegisteredTools(t *testing.T) {
	cs := connect(t, &fakeSource{freshness: sampleFreshness()})

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	// Every registered tool must advertise a description and both schemas so
	// clients can call it and validate structured output. The exact wire shape
	// is locked separately by TestToolsListGolden.
	want := map[string]bool{
		"get_data_freshness": false,
		"search_listings":    false,
		"get_listing":        false,
		"price_history":      false,
		"get_comps":          false,
		"market_stats":       false,
		"get_open_houses":    false,
	}
	for _, tool := range res.Tools {
		if _, ok := want[tool.Name]; !ok {
			t.Errorf("unexpected tool registered: %q", tool.Name)
			continue
		}
		want[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("%s: empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("%s: no input schema", tool.Name)
		}
		if tool.OutputSchema == nil {
			t.Errorf("%s: no output schema (structured output should be inferred)", tool.Name)
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("expected tool %q was not registered", name)
		}
	}
}

func TestCallGetDataFreshness(t *testing.T) {
	cs := connect(t, &fakeSource{freshness: sampleFreshness()})

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_data_freshness"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported error: %+v", res.Content)
	}
	if len(res.Content) == 0 {
		t.Error("expected text content alongside structured output")
	}

	// StructuredContent round-trips through JSON; decode it into the wire shape.
	var got freshnessOutput
	remarshal(t, res.StructuredContent, &got)

	if got.ContractVersion != "1.0.0" {
		t.Errorf("contract_version = %q, want 1.0.0", got.ContractVersion)
	}
	if got.TotalListings != 8322 {
		t.Errorf("total_listings = %d, want 8322", got.TotalListings)
	}
	if got.DataAsOf != "2026-07-03T06:18:03Z" {
		t.Errorf("data_as_of = %q, want 2026-07-03T06:18:03Z", got.DataAsOf)
	}
	if len(got.Cursors) != 1 || got.Cursors[0].Resource != "Property" || !got.Cursors[0].BackfillComplete {
		t.Errorf("unexpected cursors: %+v", got.Cursors)
	}
	if got.Cursors[0].StoredRows != 8322 {
		t.Errorf("cursor stored_rows = %d, want 8322", got.Cursors[0].StoredRows)
	}
	if len(got.ListingStatusCounts) != 2 || got.ListingStatusCounts[0].Status != "Active" {
		t.Errorf("unexpected status counts: %+v", got.ListingStatusCounts)
	}
	if len(got.MediaCounts) != 1 || got.MediaCounts[0].Count != 207028 {
		t.Errorf("unexpected media counts: %+v", got.MediaCounts)
	}
}

func TestCallGetDataFreshnessPropagatesError(t *testing.T) {
	cs := connect(t, &fakeSource{freshErr: errors.New("db down")})

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_data_freshness"})
	if err != nil {
		// A handler error may surface either as a transport error or as an
		// IsError result depending on the SDK; both are acceptable failures.
		return
	}
	if !res.IsError {
		t.Fatal("expected IsError result when the source fails")
	}
}

func TestCallSearchListings(t *testing.T) {
	lat := 41.88
	src := &fakeSource{
		search: mls.Page[mls.ListingSummary]{
			Items: []mls.ListingSummary{
				{
					ListingKey: "MRD1001", MLSNumber: "1001", StandardStatus: "Active",
					PropertyType: "Residential", PropertySubType: "Single Family Residence",
					ListPrice: 350000, Bedrooms: 3, BathroomsFull: 2, LivingArea: 1800,
					YearBuilt: 1995, DaysOnMarket: 10, Latitude: &lat,
					Address:        mls.Address{StreetNumber: "934", StreetName: "Wolcott Ave", City: "Chicago", State: "IL", PostalCode: "60601"},
					ModificationTS: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC),
				},
				{ListingKey: "MRD1002", MLSNumber: "1002", StandardStatus: "Active", ListPrice: 250000},
			},
			NextCursor: "abc123",
			DataAsOf:   time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC),
		},
	}
	cs := connect(t, src)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "search_listings",
		Arguments: map[string]any{
			"city":      "Chicago",
			"statuses":  []string{"Active"},
			"min_price": 200000,
			"limit":     2,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported error: %+v", res.Content)
	}

	// The tool decoded the arguments into the source query.
	if src.lastSearch.Area.City != "Chicago" || src.lastSearch.MinPrice != 200000 ||
		len(src.lastSearch.Statuses) != 1 || src.lastSearch.Statuses[0] != "Active" || src.lastSearch.Limit != 2 {
		t.Errorf("decoded query = %+v", src.lastSearch)
	}

	var got searchOutput
	remarshal(t, res.StructuredContent, &got)
	if got.Count != 2 || len(got.Listings) != 2 {
		t.Fatalf("count = %d, listings = %d, want 2/2", got.Count, len(got.Listings))
	}
	if got.NextCursor != "abc123" {
		t.Errorf("next_cursor = %q, want abc123", got.NextCursor)
	}
	if got.DataAsOf != "2026-06-12T09:00:00Z" {
		t.Errorf("data_as_of = %q", got.DataAsOf)
	}
	first := got.Listings[0]
	if first.ListingKey != "MRD1001" || first.Address.City != "Chicago" || first.ListPrice != 350000 {
		t.Errorf("first listing = %+v", first)
	}
	if first.Latitude == nil || *first.Latitude != 41.88 {
		t.Errorf("latitude = %v, want 41.88", first.Latitude)
	}
}

func TestCallGetListing(t *testing.T) {
	src := &fakeSource{
		listing: &mls.ListingDetail{
			ListingSummary: mls.ListingSummary{
				ListingKey: "MRD1003", MLSNumber: "1003", StandardStatus: "Closed",
				ListPrice: 500000, ClosePrice: 485000, Bedrooms: 4,
				Address:        mls.Address{StreetName: "N Ridge Ave", City: "Evanston", State: "IL"},
				ModificationTS: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC),
			},
			OriginalListPrice: 520000, TaxAnnualAmount: 8200, TaxYear: 2025,
			ListAgentName: "Jane Broker", PhotosCount: 25,
			Raw:      map[string]any{"MRD_extra": "legacy-field"},
			DataAsOf: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC),
		},
	}
	cs := connect(t, src)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_listing",
		Arguments: map[string]any{"listing_key": "MRD1003", "include_raw": true},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported error: %+v", res.Content)
	}
	if src.lastRef.Key != "MRD1003" || !src.lastOpts.IncludeRaw {
		t.Errorf("decoded ref = %+v opts = %+v", src.lastRef, src.lastOpts)
	}

	var got listingDetailOut
	remarshal(t, res.StructuredContent, &got)
	if got.ListingKey != "MRD1003" || got.ListPrice != 500000 || got.OriginalListPrice != 520000 {
		t.Errorf("detail = %+v", got)
	}
	if got.Address.StreetName != "N Ridge Ave" {
		t.Errorf("street_name = %q", got.Address.StreetName)
	}
	if got.Raw["MRD_extra"] != "legacy-field" {
		t.Errorf("raw = %v", got.Raw)
	}
	if got.DataAsOf != "2026-05-20T09:00:00Z" {
		t.Errorf("data_as_of = %q", got.DataAsOf)
	}
}

func TestCallGetListingMapsSourceErrors(t *testing.T) {
	cases := map[string]error{
		"not found": mls.ErrNotFound,
		"ambiguous": mls.ErrAmbiguousRef,
	}
	for name, srcErr := range cases {
		t.Run(name, func(t *testing.T) {
			cs := connect(t, &fakeSource{listErr: srcErr})
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "get_listing",
				Arguments: map[string]any{"mls_number": "9999"},
			})
			if err != nil {
				return // transport-level error is also an acceptable failure
			}
			if !res.IsError {
				t.Errorf("expected IsError result for %s", name)
			}
		})
	}
}

func TestCallPriceHistory(t *testing.T) {
	src := &fakeSource{
		history: &mls.PriceHistory{
			ListingKey: "MRD1003",
			Events: []mls.PriceEvent{
				{EventType: "price_change", OldValue: "520000", NewValue: "500000", ObservedAt: time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)},
				{EventType: "status_change", OldValue: "Active", NewValue: "Closed", ObservedAt: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)},
			},
			TotalReductionPct:   3.846,
			DaysSinceLastChange: 44,
			DataAsOf:            time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC),
		},
	}
	cs := connect(t, src)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "price_history",
		Arguments: map[string]any{"listing_key": "MRD1003"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported error: %+v", res.Content)
	}
	if src.lastHistRef.Key != "MRD1003" {
		t.Errorf("decoded ref = %+v", src.lastHistRef)
	}

	var got historyOutput
	remarshal(t, res.StructuredContent, &got)
	if got.ListingKey != "MRD1003" || len(got.Events) != 2 {
		t.Fatalf("history = %+v", got)
	}
	if got.Events[0].EventType != "price_change" || got.Events[0].NewValue != "500000" {
		t.Errorf("event[0] = %+v", got.Events[0])
	}
	if got.Events[0].ObservedAt != "2026-05-10T09:00:00Z" {
		t.Errorf("event[0] observed_at = %q", got.Events[0].ObservedAt)
	}
	if got.DaysSinceLastChange != 44 {
		t.Errorf("days_since_last_change = %d, want 44", got.DaysSinceLastChange)
	}
}

func TestCallPriceHistoryMapsErrors(t *testing.T) {
	cases := map[string]error{"not found": mls.ErrNotFound, "ambiguous": mls.ErrAmbiguousRef}
	for name, srcErr := range cases {
		t.Run(name, func(t *testing.T) {
			cs := connect(t, &fakeSource{histErr: srcErr})
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "price_history",
				Arguments: map[string]any{"mls_number": "9999"},
			})
			if err != nil {
				return
			}
			if !res.IsError {
				t.Errorf("expected IsError result for %s", name)
			}
		})
	}
}

func TestCallGetComps(t *testing.T) {
	dist := 0.8
	src := &fakeSource{
		comps: &mls.CompsResult{
			Comparables: []mls.Comparable{
				{
					ListingSummary: mls.ListingSummary{ListingKey: "MRD1003", ClosePrice: 485000, LivingArea: 2600},
					DistanceMiles:  &dist,
					Similarity:     0.92,
					AdjustNotes:    []string{"100 sqft smaller", "same beds"},
				},
			},
			MedianClosePrice: 485000,
			MedianPPSF:       187,
			SuggestedLow:     470000,
			SuggestedHigh:    510000,
			DataAsOf:         time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC),
		},
	}
	cs := connect(t, src)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_comps",
		Arguments: map[string]any{
			"listing_key": "MRD1010", "radius_miles": 5, "closed_within_days": 365, "limit": 3,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported error: %+v", res.Content)
	}
	// Arguments decoded into the source query (subject ref + tuning).
	if src.lastComps.Subject.Key != "MRD1010" || src.lastComps.RadiusMiles != 5 ||
		src.lastComps.Limit != 3 || src.lastComps.ClosedWithin != 365*24*time.Hour {
		t.Errorf("decoded comps query = %+v", src.lastComps)
	}

	var got compsOutput
	remarshal(t, res.StructuredContent, &got)
	if got.Count != 1 || len(got.Comparables) != 1 {
		t.Fatalf("count = %d comparables = %d, want 1/1", got.Count, len(got.Comparables))
	}
	if got.MedianClosePrice != 485000 || got.SuggestedLow != 470000 || got.SuggestedHigh != 510000 {
		t.Errorf("valuation = %+v", got)
	}
	c := got.Comparables[0]
	if c.ListingKey != "MRD1003" || c.Similarity != 0.92 {
		t.Errorf("comp = %+v", c)
	}
	if c.DistanceMiles == nil || *c.DistanceMiles != 0.8 {
		t.Errorf("distance = %v, want 0.8", c.DistanceMiles)
	}
	if len(c.AdjustNotes) != 2 {
		t.Errorf("adjust_notes = %v", c.AdjustNotes)
	}
}

func TestCallGetCompsSpecScope(t *testing.T) {
	// An inline spec that carries an area is accepted and forwarded as Spec.
	src := &fakeSource{comps: &mls.CompsResult{}}
	cs := connect(t, src)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_comps",
		Arguments: map[string]any{"city": "Evanston", "property_type": "Residential", "living_area": 2700},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res.Content)
	}
	if src.lastComps.Spec == nil || src.lastComps.Spec.Area.City != "Evanston" || src.lastComps.Spec.LivingArea != 2700 {
		t.Errorf("spec not forwarded: %+v", src.lastComps.Spec)
	}
}

func TestCallGetCompsRequiresScope(t *testing.T) {
	// Neither a subject ref nor a geographic scope → the tool rejects it before
	// touching the source (a whole-market scan is never implied).
	cs := connect(t, &fakeSource{comps: &mls.CompsResult{}})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_comps",
		Arguments: map[string]any{"living_area": 2000},
	})
	if err != nil {
		return
	}
	if !res.IsError {
		t.Error("expected IsError result when no subject or area/coords is given")
	}
}

func TestCallGetCompsMapsSubjectNotFound(t *testing.T) {
	cs := connect(t, &fakeSource{compsErr: mls.ErrNotFound})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_comps",
		Arguments: map[string]any{"listing_key": "NOPE"},
	})
	if err != nil {
		return
	}
	if !res.IsError {
		t.Error("expected IsError result when the subject is not found")
	}
}

func TestCallGetListingRequiresRef(t *testing.T) {
	cs := connect(t, &fakeSource{})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_listing"})
	if err != nil {
		return
	}
	if !res.IsError {
		t.Error("expected IsError result when neither listing_key nor mls_number is given")
	}
}

func TestCallMarketStats(t *testing.T) {
	src := &fakeSource{
		stats: &mls.MarketStats{
			PeriodDays:                   90,
			MedianListPrice:              500000,
			ActiveInventory:              4,
			MonthsOfSupply:               2.4,
			MedianClosePrice:             500000,
			AvgClosePrice:                500000,
			MedianPPSF:                   200,
			MedianDaysOnMarket:           40,
			MedianCumulativeDaysOnMarket: 45,
			SaleToListRatio:              0.9783,
			SaleToOriginalRatio:          0.9574,
			ClosedInPeriod:               5,
			Prior: &mls.MarketStats{
				PeriodDays:       90,
				MedianClosePrice: 420000,
				ClosedInPeriod:   3,
			},
			DataAsOf: time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC),
		},
	}
	cs := connect(t, src)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "market_stats",
		Arguments: map[string]any{
			"city": "Rivertown", "property_types": []string{"Residential"},
			"period_days": 90, "compare_to_prior": true,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported error: %+v", res.Content)
	}
	// Arguments decoded into the source query.
	if src.lastStats.Area.City != "Rivertown" || len(src.lastStats.PropertyTypes) != 1 ||
		!src.lastStats.CompareToPrior || src.lastStats.Period != 90*24*time.Hour {
		t.Errorf("decoded stats query = %+v", src.lastStats)
	}

	var got marketStatsOut
	remarshal(t, res.StructuredContent, &got)
	if got.MedianClosePrice != 500000 || got.ActiveInventory != 4 || got.ClosedInPeriod != 5 {
		t.Errorf("stats = %+v", got)
	}
	if got.MonthsOfSupply != 2.4 || got.SaleToListRatio != 0.9783 || got.SaleToOriginalRatio != 0.9574 {
		t.Errorf("derived metrics = %+v", got)
	}
	if got.MedianCumulativeDaysOnMarket != 45 {
		t.Errorf("median_cumulative_days_on_market = %d, want 45", got.MedianCumulativeDaysOnMarket)
	}
	if got.Prior == nil || got.Prior.MedianClosePrice != 420000 || got.Prior.ClosedInPeriod != 3 {
		t.Errorf("prior = %+v", got.Prior)
	}
	if got.DataAsOf != "2026-06-30T09:00:00Z" {
		t.Errorf("data_as_of = %q", got.DataAsOf)
	}
}

func TestCallMarketStatsRequiresArea(t *testing.T) {
	// No area → the tool rejects it before touching the source (a whole-feed
	// aggregate is never implied).
	cs := connect(t, &fakeSource{stats: &mls.MarketStats{}})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "market_stats",
		Arguments: map[string]any{"period_days": 30},
	})
	if err != nil {
		return
	}
	if !res.IsError {
		t.Error("expected IsError result when no area is given")
	}
}

func TestCallGetOpenHouses(t *testing.T) {
	src := &fakeSource{
		openHouses: mls.OpenHouseResult{
			OpenHouses: []mls.OpenHouse{
				{
					ListingKey: "RVT-A1", MLSNumber: "A1",
					Address:   mls.Address{StreetName: "Main St", City: "Rivertown", State: "IL"},
					Date:      time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
					StartTime: time.Date(2026, 7, 4, 16, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 7, 4, 18, 0, 0, 0, time.UTC),
					Remarks:   "Weekend open house",
				},
			},
			DataAsOf: time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC),
		},
	}
	cs := connect(t, src)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_open_houses",
		Arguments: map[string]any{
			"city": "Rivertown", "from": "2026-07-01", "to": "2026-07-31", "limit": 10,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported error: %+v", res.Content)
	}
	// Arguments decoded, including the parsed date window.
	if src.lastOH.Area.City != "Rivertown" || src.lastOH.Limit != 10 ||
		!src.lastOH.From.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) ||
		!src.lastOH.To.Equal(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("decoded open-house query = %+v", src.lastOH)
	}

	var got openHousesOutput
	remarshal(t, res.StructuredContent, &got)
	if got.Count != 1 || len(got.OpenHouses) != 1 {
		t.Fatalf("count = %d open_houses = %d, want 1/1", got.Count, len(got.OpenHouses))
	}
	oh := got.OpenHouses[0]
	if oh.ListingKey != "RVT-A1" || oh.Address.City != "Rivertown" {
		t.Errorf("open house = %+v", oh)
	}
	if oh.Date != "2026-07-04" || oh.StartTime != "2026-07-04T16:00:00Z" {
		t.Errorf("date/start = %q/%q", oh.Date, oh.StartTime)
	}
	if got.DataAsOf != "2026-07-01T09:00:00Z" {
		t.Errorf("data_as_of = %q", got.DataAsOf)
	}
}

func TestCallGetOpenHousesBadDate(t *testing.T) {
	// An unparseable date is rejected before the source is called.
	cs := connect(t, &fakeSource{openHouses: mls.OpenHouseResult{}})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_open_houses",
		Arguments: map[string]any{"from": "next tuesday"},
	})
	if err != nil {
		return
	}
	if !res.IsError {
		t.Error("expected IsError result for an invalid date")
	}
}

func TestQuerySQLDisabledByDefault(t *testing.T) {
	// The source implements SQLQuerier, but without WithSQL the tool must not be
	// registered — the escape hatch is opt-in.
	cs := connect(t, &fakeSource{freshness: sampleFreshness()})
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name == "query_sql" {
			t.Fatal("query_sql registered without WithSQL; it must be opt-in")
		}
	}
}

func TestQuerySQLRegisteredWhenEnabled(t *testing.T) {
	cs := connectSQL(t, &fakeSource{freshness: sampleFreshness()})
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := false
	for _, tool := range res.Tools {
		if tool.Name == "query_sql" {
			found = true
			if tool.OutputSchema == nil {
				t.Error("query_sql: no output schema inferred")
			}
		}
	}
	if !found {
		t.Error("query_sql not registered despite WithSQL(true)")
	}
}

func TestCallQuerySQL(t *testing.T) {
	src := &fakeSource{
		sqlResult: &mls.ResultSet{
			Columns:   []string{"city", "n"},
			Rows:      [][]any{{"Chicago", int64(7)}, {"Evanston", int64(2)}},
			Truncated: true,
			DataAsOf:  time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC),
		},
	}
	cs := connectSQL(t, src)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "query_sql",
		Arguments: map[string]any{
			"query":    "SELECT city, count(*) AS n FROM property GROUP BY city",
			"max_rows": 2,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported error: %+v", res.Content)
	}
	// Arguments decoded and forwarded verbatim to the source.
	if src.lastSQL != "SELECT city, count(*) AS n FROM property GROUP BY city" || src.lastMaxRows != 2 {
		t.Errorf("forwarded query = %q max_rows = %d", src.lastSQL, src.lastMaxRows)
	}

	var got querySQLOutput
	remarshal(t, res.StructuredContent, &got)
	if len(got.Columns) != 2 || got.Columns[0] != "city" || got.Columns[1] != "n" {
		t.Errorf("columns = %v", got.Columns)
	}
	if got.RowCount != 2 || len(got.Rows) != 2 {
		t.Fatalf("row_count = %d rows = %d, want 2/2", got.RowCount, len(got.Rows))
	}
	if !got.Truncated {
		t.Error("truncated = false, want true")
	}
	if got.Rows[0][0] != "Chicago" {
		t.Errorf("rows[0][0] = %v, want Chicago", got.Rows[0][0])
	}
	if got.DataAsOf != "2026-06-30T09:00:00Z" {
		t.Errorf("data_as_of = %q", got.DataAsOf)
	}
}

func TestCallQuerySQLPropagatesError(t *testing.T) {
	// A guard rejection (or any adapter error) surfaces as an IsError result.
	cs := connectSQL(t, &fakeSource{sqlErr: errors.New("query_sql: disallowed keyword DELETE")})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "query_sql",
		Arguments: map[string]any{"query": "DELETE FROM property"},
	})
	if err != nil {
		return // transport-level error is also an acceptable failure
	}
	if !res.IsError {
		t.Fatal("expected IsError result when the query is rejected")
	}
}

// remarshal re-encodes v (typically a decoded any) and decodes it into dst.
func remarshal(t *testing.T, v any, dst any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		t.Fatalf("unmarshal into %T: %v", dst, err)
	}
}
