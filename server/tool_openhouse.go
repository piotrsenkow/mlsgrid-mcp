package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const openHousesDescription = `List scheduled open houses for an area and date range.

Optionally scope by area (city / postal_code / county / state) and bound with from / to dates (YYYY-MM-DD or RFC3339). Defaults to the next 7 days from today. Results are ordered by date, then start time; limit defaults to 25 (capped at 100). Each entry carries the listing_key (use it with get_listing for full detail), the address, the date and start/end times, and any public remarks.

Use it to plan a showing route (agents/buyers) or to see which comparable listings are actively being marketed this week. The schedule is only as current as the last sync — data_as_of tells you when that was, and an open house synced days ago may since have been cancelled.`

// openHousesInput scopes an open-house lookup. Field names/tags are a public
// contract locked by the tools/list golden test.
type openHousesInput struct {
	City       string `json:"city,omitempty" jsonschema:"area: city (case-insensitive exact match)"`
	PostalCode string `json:"postal_code,omitempty" jsonschema:"area: postal code"`
	County     string `json:"county,omitempty" jsonschema:"area: county or parish"`
	State      string `json:"state,omitempty" jsonschema:"area: two-letter state or province"`
	From       string `json:"from,omitempty" jsonschema:"start of the date window, YYYY-MM-DD or RFC3339 (default today)"`
	To         string `json:"to,omitempty" jsonschema:"end of the date window, inclusive, YYYY-MM-DD or RFC3339 (default from + 7 days)"`
	Limit      int    `json:"limit,omitempty" jsonschema:"maximum open houses to return (default 25, capped at 100)"`
}

type openHouseOut struct {
	ListingKey string     `json:"listing_key,omitempty" jsonschema:"MLS Grid key of the listing; use with get_listing"`
	MLSNumber  string     `json:"mls_number,omitempty" jsonschema:"human-facing MLS number of the listing"`
	Address    addressOut `json:"address" jsonschema:"listing address (from the joined property record)"`
	Date       string     `json:"date" jsonschema:"open-house date, YYYY-MM-DD"`
	StartTime  string     `json:"start_time,omitempty" jsonschema:"start time (RFC3339 UTC)"`
	EndTime    string     `json:"end_time,omitempty" jsonschema:"end time (RFC3339 UTC)"`
	Remarks    string     `json:"remarks,omitempty" jsonschema:"public open-house remarks"`
}

// openHousesOutput is the wire shape of get_open_houses.
type openHousesOutput struct {
	OpenHouses []openHouseOut `json:"open_houses" jsonschema:"scheduled open houses, earliest first"`
	Count      int            `json:"count" jsonschema:"number of open houses returned"`
	DataAsOf   string         `json:"data_as_of,omitempty" jsonschema:"how current the open-house schedule is (RFC3339 UTC)"`
}

func registerOpenHouses(srv *mcp.Server, source mls.Source) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_open_houses",
		Description: openHousesDescription,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in openHousesInput) (*mcp.CallToolResult, openHousesOutput, error) {
		q, err := toOpenHouseQuery(in)
		if err != nil {
			return nil, openHousesOutput{}, err
		}
		res, err := source.OpenHouses(ctx, q)
		if err != nil {
			return nil, openHousesOutput{}, err
		}
		return nil, toOpenHousesOutput(res), nil
	})
}

func toOpenHouseQuery(in openHousesInput) (mls.OpenHouseQuery, error) {
	from, err := parseFlexibleDate(in.From)
	if err != nil {
		return mls.OpenHouseQuery{}, fmt.Errorf("from: %w", err)
	}
	to, err := parseFlexibleDate(in.To)
	if err != nil {
		return mls.OpenHouseQuery{}, fmt.Errorf("to: %w", err)
	}
	return mls.OpenHouseQuery{
		Area:  mls.Area{City: in.City, PostalCode: in.PostalCode, County: in.County, State: in.State},
		From:  from,
		To:    to,
		Limit: in.Limit,
	}, nil
}

func toOpenHousesOutput(res mls.OpenHouseResult) openHousesOutput {
	out := openHousesOutput{
		Count:    len(res.OpenHouses),
		DataAsOf: formatTime(res.DataAsOf),
	}
	out.OpenHouses = make([]openHouseOut, 0, len(res.OpenHouses))
	for _, oh := range res.OpenHouses {
		out.OpenHouses = append(out.OpenHouses, openHouseOut{
			ListingKey: oh.ListingKey,
			MLSNumber:  oh.MLSNumber,
			Address:    toAddressOut(oh.Address),
			Date:       formatDate(oh.Date),
			StartTime:  formatTime(oh.StartTime),
			EndTime:    formatTime(oh.EndTime),
			Remarks:    oh.Remarks,
		})
	}
	return out
}

// parseFlexibleDate accepts a calendar date (YYYY-MM-DD) or a full RFC3339
// timestamp, returning the zero time for an empty string (the source applies
// its default window).
func parseFlexibleDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q (use YYYY-MM-DD or RFC3339)", s)
}

// formatDate renders a calendar date, or "" for the zero time.
func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}
