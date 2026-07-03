package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const historyDescription = `Return a listing's observed price and status change timeline.

Identify the listing the same way as get_listing: by listing_key (preferred) or by mls_number (add originating_system if the number exists in more than one feed). Returns each captured event (new listing, price change, status change, back on market, delisted) with its old/new value and when it was observed, plus total_reduction_pct (net list-price change as a percent of the first observed price; positive means the price came down) and days_since_last_change.

Important: history is best-effort from first sync forward — there is no event for a change that happened before this database began tracking the listing, so an empty or short timeline does not mean the listing never changed. Use it to spot motivated sellers (repeated cuts, long time since last change) and to explain a price's trajectory.`

// historyInput identifies the listing whose history to return.
type historyInput struct {
	ListingKey        string `json:"listing_key,omitempty" jsonschema:"MLS Grid cross-MLS unique key (preferred, unambiguous)"`
	MLSNumber         string `json:"mls_number,omitempty" jsonschema:"human-facing MLS number; unique only within an originating system"`
	OriginatingSystem string `json:"originating_system,omitempty" jsonschema:"originating system slug (e.g. mred) to disambiguate mls_number"`
}

type priceEventOut struct {
	EventType  string `json:"event_type" jsonschema:"one of new_listing, price_change, status_change, back_on_market, delisted"`
	OldValue   string `json:"old_value,omitempty" jsonschema:"prior value; empty for new_listing"`
	NewValue   string `json:"new_value,omitempty" jsonschema:"new value; empty for delisted"`
	ObservedAt string `json:"observed_at" jsonschema:"when the sync observed the change (RFC3339 UTC)"`
}

// historyOutput is the wire shape of price_history.
type historyOutput struct {
	ListingKey          string          `json:"listing_key" jsonschema:"the listing this history belongs to"`
	Events              []priceEventOut `json:"events" jsonschema:"observed changes, oldest first; may be empty"`
	TotalReductionPct   float64         `json:"total_reduction_pct" jsonschema:"net list-price change as a percent of the first observed price; positive means a reduction"`
	DaysSinceLastChange int             `json:"days_since_last_change" jsonschema:"whole days since the most recent observed change; 0 if none"`
	DataAsOf            string          `json:"data_as_of" jsonschema:"how current this listing's record is (RFC3339 UTC)"`
}

func registerHistory(srv *mcp.Server, source mls.Source) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "price_history",
		Description: historyDescription,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in historyInput) (*mcp.CallToolResult, historyOutput, error) {
		ref := mls.ListingRef{Key: in.ListingKey, MLSNumber: in.MLSNumber, OriginatingSystem: in.OriginatingSystem}
		if ref.Empty() {
			return nil, historyOutput{}, errors.New("provide listing_key or mls_number")
		}
		h, err := source.PriceHistory(ctx, ref)
		if err != nil {
			switch {
			case errors.Is(err, mls.ErrNotFound):
				return nil, historyOutput{}, fmt.Errorf("no listing matches that reference")
			case errors.Is(err, mls.ErrAmbiguousRef):
				return nil, historyOutput{}, fmt.Errorf("mls_number %q matches multiple originating systems — set originating_system", in.MLSNumber)
			default:
				return nil, historyOutput{}, err
			}
		}
		return nil, toHistoryOutput(h), nil
	})
}

func toHistoryOutput(h *mls.PriceHistory) historyOutput {
	out := historyOutput{
		ListingKey:          h.ListingKey,
		TotalReductionPct:   h.TotalReductionPct,
		DaysSinceLastChange: h.DaysSinceLastChange,
		DataAsOf:            formatTime(h.DataAsOf),
	}
	out.Events = make([]priceEventOut, 0, len(h.Events))
	for _, e := range h.Events {
		out.Events = append(out.Events, priceEventOut{
			EventType:  e.EventType,
			OldValue:   e.OldValue,
			NewValue:   e.NewValue,
			ObservedAt: formatTime(e.ObservedAt),
		})
	}
	return out
}
