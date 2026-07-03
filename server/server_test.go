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

// fakeSource is a canned mls.Source for protocol-level tests. Only the methods
// B-M1 exercises return data; the rest report ErrNotImplemented, matching how a
// partially-built adapter behaves.
type fakeSource struct {
	freshness mls.Freshness
	freshErr  error
	closed    bool
}

func (f *fakeSource) Capabilities(context.Context) (mls.Capabilities, error) {
	return mls.Capabilities{SchemaContractVersion: f.freshness.SchemaContractVersion}, nil
}
func (f *fakeSource) Freshness(context.Context) (mls.Freshness, error) {
	return f.freshness, f.freshErr
}
func (f *fakeSource) SearchListings(context.Context, mls.SearchQuery) (mls.Page[mls.ListingSummary], error) {
	return mls.Page[mls.ListingSummary]{}, mls.ErrNotImplemented
}
func (f *fakeSource) GetListing(context.Context, mls.ListingRef, mls.ListingOptions) (*mls.ListingDetail, error) {
	return nil, mls.ErrNotImplemented
}
func (f *fakeSource) FindComparables(context.Context, mls.CompsQuery) (*mls.CompsResult, error) {
	return nil, mls.ErrNotImplemented
}
func (f *fakeSource) MarketStats(context.Context, mls.StatsQuery) (*mls.MarketStats, error) {
	return nil, mls.ErrNotImplemented
}
func (f *fakeSource) PriceHistory(context.Context, mls.ListingRef) (*mls.PriceHistory, error) {
	return nil, mls.ErrNotImplemented
}
func (f *fakeSource) OpenHouses(context.Context, mls.OpenHouseQuery) ([]mls.OpenHouse, error) {
	return nil, mls.ErrNotImplemented
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
	t.Helper()
	ctx := context.Background()
	srv, err := New(source, WithInfo("mlsgrid-mcp-test", "test"))
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

func TestListToolsExposesFreshness(t *testing.T) {
	cs := connect(t, &fakeSource{freshness: sampleFreshness()})

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(res.Tools) != 1 {
		t.Fatalf("expected exactly 1 tool, got %d", len(res.Tools))
	}
	tool := res.Tools[0]
	if tool.Name != "get_data_freshness" {
		t.Errorf("tool name = %q, want get_data_freshness", tool.Name)
	}
	if tool.Description == "" {
		t.Error("tool description is empty")
	}
	if tool.InputSchema == nil {
		t.Error("tool has no input schema")
	}
	if tool.OutputSchema == nil {
		t.Error("tool has no output schema (structured output should be inferred)")
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
