//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

func TestDescribeDataset(t *testing.T) {
	ctx := context.Background()
	d, err := testAdapter.DescribeDataset(ctx)
	if err != nil {
		t.Fatalf("DescribeDataset: %v", err)
	}

	tbl := map[string]mls.TableDescription{}
	for _, td := range d.Tables {
		tbl[td.Name] = td
	}
	for _, want := range []string{"property", "listing_event", "media", "open_house", "room", "unit_type"} {
		if _, ok := tbl[want]; !ok {
			t.Errorf("description missing table %q", want)
		}
	}
	for _, skip := range describeSkipTables {
		if _, ok := tbl[skip]; ok {
			t.Errorf("internal table %q should be excluded from the description", skip)
		}
	}

	// property carries its columns with SQL types and nullability.
	prop := tbl["property"]
	if len(prop.Columns) < 20 {
		t.Errorf("property has %d columns, want many", len(prop.Columns))
	}
	col := map[string]mls.ColumnDescription{}
	for _, c := range prop.Columns {
		col[c.Name] = c
	}
	if c, ok := col["list_price"]; !ok || c.Type != "numeric" {
		t.Errorf("list_price = %+v, want numeric", c)
	}
	if c, ok := col["standard_status"]; !ok || !c.Nullable {
		t.Errorf("standard_status = %+v, want a nullable text column", c)
	}

	// The status enum must reflect the seed's real, exact-cased values.
	status := enumValuesFor(d, "property", "standard_status")
	if status == nil {
		t.Fatal("no standard_status enum in description")
	}
	for _, want := range []string{"Active", "Closed"} {
		if _, ok := status[want]; !ok {
			t.Errorf("standard_status enum missing %q (got %v)", want, status)
		}
	}
	if d.DataAsOf.IsZero() {
		t.Error("DataAsOf is zero, want the newest property timestamp")
	}
}

func enumValuesFor(d *mls.DatasetDescription, table, column string) map[string]int64 {
	for _, e := range d.Enums {
		if e.Table == table && e.Column == column {
			m := map[string]int64{}
			for _, v := range e.Values {
				m[v.Value] = v.Count
			}
			return m
		}
	}
	return nil
}

func TestSearchTotalAndCaseInsensitive(t *testing.T) {
	ctx := context.Background()

	// Exact-case status: 8 active in the seed (see TestSearchFilters).
	exact, err := testAdapter.SearchListings(ctx, mls.SearchQuery{Statuses: []string{"Active"}})
	if err != nil {
		t.Fatalf("search exact: %v", err)
	}
	if exact.Total != 8 {
		t.Errorf("Total = %d, want 8", exact.Total)
	}

	// Lowercase must match the same rows — casing can't silently zero-out.
	lower, err := testAdapter.SearchListings(ctx, mls.SearchQuery{Statuses: []string{"active"}})
	if err != nil {
		t.Fatalf("search lowercase: %v", err)
	}
	if lower.Total != exact.Total || len(lower.Items) != len(exact.Items) {
		t.Errorf("case-insensitive status mismatch: lower(Total=%d,Items=%d) vs exact(Total=%d,Items=%d)",
			lower.Total, len(lower.Items), exact.Total, len(exact.Items))
	}

	// Property type is case-insensitive too (matches PropertyType or SubType).
	up, err := testAdapter.SearchListings(ctx, mls.SearchQuery{PropertyTypes: []string{"CONDOMINIUM"}})
	if err != nil {
		t.Fatalf("search uppercase type: %v", err)
	}
	if up.Total != 4 {
		t.Errorf("Condominium Total = %d, want 4", up.Total)
	}
}

func TestSearchTotalExceedsPage(t *testing.T) {
	ctx := context.Background()
	// A page limit below the match count: Total is all matches, not the page.
	p, err := testAdapter.SearchListings(ctx, mls.SearchQuery{Limit: 3})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(p.Items) != 3 {
		t.Errorf("page items = %d, want 3", len(p.Items))
	}
	if p.Total != 12 {
		t.Errorf("Total = %d, want 12 (whole seed)", p.Total)
	}
	if p.NextCursor == "" {
		t.Error("expected a next cursor when more rows remain")
	}
}

func TestEndToEndDescribeOverMCP(t *testing.T) {
	ctx := context.Background()
	cs := mcpClient(t, testAdapter) // describe_dataset registers automatically

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "describe_dataset"})
	if err != nil {
		t.Fatalf("describe_dataset CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("describe_dataset tool error: %+v", res.Content)
	}
	var out struct {
		Tables []struct {
			Name string `json:"name"`
		} `json:"tables"`
		Enums []struct {
			Column string `json:"column"`
			Values []struct {
				Value string `json:"value"`
			} `json:"values"`
		} `json:"enums"`
	}
	decodeStructured(t, res.StructuredContent, &out)
	if len(out.Tables) < 5 {
		t.Errorf("tables = %d over MCP, want several", len(out.Tables))
	}
	found := false
	for _, e := range out.Enums {
		if e.Column == "standard_status" {
			for _, v := range e.Values {
				if v.Value == "Active" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("standard_status enum with 'Active' not surfaced over MCP")
	}
}
