package server

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const freshnessDescription = `Report how current the underlying MLS data is before you rely on the other tools.

Returns, per synced resource, the replication cursor (newest listing update seen, whether the initial backfill finished, and when the last full reconcile ran), plus the listing corpus broken down by status, media download coverage, the schema-contract version the data conforms to, and when this snapshot was taken.

Use it to decide whether the dataset is fresh enough to trust for an answer, to explain staleness to the user, or simply to confirm the sync pipeline is alive. Takes no arguments.`

// freshnessInput carries no parameters; get_data_freshness is a nullary tool.
type freshnessInput struct{}

// freshnessOutput is the wire shape of get_data_freshness. Its JSON schema is
// part of the tool contract (locked by the tools/list golden test), so field
// names and descriptions are deliberate.
type freshnessOutput struct {
	ContractVersion     string      `json:"contract_version" jsonschema:"the mlsgrid-sync schema-contract version the data conforms to"`
	TotalListings       int64       `json:"total_listings" jsonschema:"total number of property listings stored"`
	DataAsOf            string      `json:"data_as_of,omitempty" jsonschema:"newest listing modification timestamp (RFC3339 UTC) — how current the data is; empty if unknown"`
	GeneratedAt         string      `json:"generated_at" jsonschema:"when this freshness snapshot was produced (RFC3339 UTC)"`
	Cursors             []cursorOut `json:"cursors" jsonschema:"replication state per synced resource and MLS feed"`
	ListingStatusCounts []countOut  `json:"listing_status_counts" jsonschema:"listing counts by standard status, most populous first"`
	MediaCounts         []countOut  `json:"media_counts,omitempty" jsonschema:"media rows by storage status (downloaded/pending/failed/skipped)"`
}

type cursorOut struct {
	Resource          string `json:"resource" jsonschema:"RESO resource name, e.g. Property or OpenHouse"`
	OriginatingSystem string `json:"originating_system" jsonschema:"MLS feed slug, e.g. mred"`
	StoredRows        int64  `json:"stored_rows" jsonschema:"rows of this resource held locally"`
	Watermark         string `json:"watermark,omitempty" jsonschema:"newest ModificationTimestamp synced (RFC3339 UTC); empty before first sync"`
	BackfillComplete  bool   `json:"backfill_complete" jsonschema:"whether the initial full import finished"`
	LastReconcile     string `json:"last_reconcile,omitempty" jsonschema:"when the last full-feed reconcile completed (RFC3339 UTC); empty if never"`
}

type countOut struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

func registerFreshness(srv *mcp.Server, source mls.Source) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_data_freshness",
		Description: freshnessDescription,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ freshnessInput) (*mcp.CallToolResult, freshnessOutput, error) {
		f, err := source.Freshness(ctx)
		if err != nil {
			return nil, freshnessOutput{}, err
		}
		return nil, toFreshnessOutput(f), nil
	})
}

func toFreshnessOutput(f mls.Freshness) freshnessOutput {
	out := freshnessOutput{
		ContractVersion: f.SchemaContractVersion,
		TotalListings:   f.TotalListings,
		DataAsOf:        formatTime(f.DataAsOf),
		GeneratedAt:     formatTime(f.GeneratedAt),
	}
	for _, c := range f.Cursors {
		out.Cursors = append(out.Cursors, cursorOut{
			Resource:          c.Resource,
			OriginatingSystem: c.OriginatingSystem,
			StoredRows:        c.StoredRows,
			Watermark:         formatTimePtr(c.Watermark),
			BackfillComplete:  c.BackfillComplete,
			LastReconcile:     formatTimePtr(c.LastReconcile),
		})
	}
	for _, sc := range f.ListingStatusCounts {
		out.ListingStatusCounts = append(out.ListingStatusCounts, countOut{Status: sc.Status, Count: sc.Count})
	}
	for _, sc := range f.MediaCounts {
		out.MediaCounts = append(out.MediaCounts, countOut{Status: sc.Status, Count: sc.Count})
	}
	return out
}

// formatTime renders a UTC RFC3339 timestamp, or "" for the zero time.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}
