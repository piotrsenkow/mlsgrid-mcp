package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const searchDescription = `Search the MLS listing corpus by area, status, type, price, size, and free text.

Use this to find candidate properties before drilling into one with get_listing. All filters are optional and AND together; omit a filter to leave that dimension unconstrained. Money is in whole US dollars. Results are ordered newest-updated first.

Notes for accurate use:
- Area: set at most one of city / postal_code / county (state narrows further). Matching is case-insensitive and exact — pass "Chicago", not "chicago area".
- statuses / property_types accept several values (OR within a field) and are matched case-insensitively; a property_types value matches either PropertyType or PropertySubType. Their valid values are listed on each field; call describe_dataset if unsure.
- keywords is a best-effort substring match over remarks and address (no full-text index), so it is a filter of last resort, not a semantic search.
- Coordinates (latitude/longitude) are frequently absent — many MLSs, including MRED, omit them.
- total_matches is the count across all pages for these filters; paginate with next_cursor (pass it back as cursor), and an empty next_cursor means the last page.

Personas: buyers/investors filter by area+price+beds; agents pull recent activity in a market; analysts sweep a status/type across a county.`

// searchInput is the argument shape of search_listings. Field names/tags are a
// public contract (locked by the tools/list golden test).
type searchInput struct {
	City            string   `json:"city,omitempty" jsonschema:"city to search within (case-insensitive exact match)"`
	PostalCode      string   `json:"postal_code,omitempty" jsonschema:"postal code to search within"`
	County          string   `json:"county,omitempty" jsonschema:"county or parish to search within"`
	State           string   `json:"state,omitempty" jsonschema:"two-letter state or province to narrow the area"`
	Statuses        []string `json:"statuses,omitempty" jsonschema:"StandardStatus values to include (case-insensitive); valid: Active, Closed, Active Under Contract, Pending, Canceled, Expired, Hold. Empty means any"`
	PropertyTypes   []string `json:"property_types,omitempty" jsonschema:"PropertyType (or PropertySubType) values to include (case-insensitive); valid PropertyType: Residential, Residential Lease, Residential Income, Land, Commercial Sale, Commercial Lease, Manufactured In Park, Farm, Business Opportunity. Empty means any"`
	MinPrice        int64    `json:"min_price,omitempty" jsonschema:"minimum list price in whole US dollars"`
	MaxPrice        int64    `json:"max_price,omitempty" jsonschema:"maximum list price in whole US dollars"`
	MinBeds         int      `json:"min_beds,omitempty" jsonschema:"minimum total bedrooms"`
	MinBathsFull    int      `json:"min_baths_full,omitempty" jsonschema:"minimum full bathrooms"`
	MinLivingArea   int64    `json:"min_living_area,omitempty" jsonschema:"minimum living area in square feet"`
	MaxLivingArea   int64    `json:"max_living_area,omitempty" jsonschema:"maximum living area in square feet"`
	MinYearBuilt    int      `json:"min_year_built,omitempty" jsonschema:"earliest year built"`
	MaxDaysOnMarket int      `json:"max_days_on_market,omitempty" jsonschema:"maximum days on market"`
	Keywords        string   `json:"keywords,omitempty" jsonschema:"best-effort substring match over remarks and address"`
	Cursor          string   `json:"cursor,omitempty" jsonschema:"opaque pagination cursor from a prior result's next_cursor"`
	Limit           int      `json:"limit,omitempty" jsonschema:"maximum results to return (default 25, capped at 100)"`
}

// searchOutput is the wire shape of search_listings.
type searchOutput struct {
	Listings     []listingSummaryOut `json:"listings" jsonschema:"matching listings, newest-updated first"`
	Count        int                 `json:"count" jsonschema:"number of listings on this page"`
	TotalMatches *int64              `json:"total_matches,omitempty" jsonschema:"total listings matching the filters across all pages; absent if the source cannot count"`
	NextCursor   string              `json:"next_cursor,omitempty" jsonschema:"pass back as cursor to fetch the next page; empty on the last page"`
	DataAsOf     string              `json:"data_as_of,omitempty" jsonschema:"how current the underlying data is (newest record timestamp, RFC3339 UTC)"`
}

func registerSearch(srv *mcp.Server, source mls.Source) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_listings",
		Description: searchDescription,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
		page, err := source.SearchListings(ctx, toSearchQuery(in))
		if err != nil {
			return nil, searchOutput{}, err
		}
		out := searchOutput{
			Count:      len(page.Items),
			NextCursor: page.NextCursor,
			DataAsOf:   formatTime(page.DataAsOf),
		}
		if page.Total >= 0 {
			total := page.Total
			out.TotalMatches = &total
		}
		out.Listings = make([]listingSummaryOut, 0, len(page.Items))
		for _, s := range page.Items {
			out.Listings = append(out.Listings, toSummaryOut(s))
		}
		return nil, out, nil
	})
}

func toSearchQuery(in searchInput) mls.SearchQuery {
	return mls.SearchQuery{
		Area: mls.Area{
			City:       in.City,
			PostalCode: in.PostalCode,
			County:     in.County,
			State:      in.State,
		},
		Statuses:        in.Statuses,
		PropertyTypes:   in.PropertyTypes,
		MinPrice:        in.MinPrice,
		MaxPrice:        in.MaxPrice,
		MinBeds:         in.MinBeds,
		MinBathsFull:    in.MinBathsFull,
		MinLivingArea:   in.MinLivingArea,
		MaxLivingArea:   in.MaxLivingArea,
		MinYearBuilt:    in.MinYearBuilt,
		MaxDaysOnMarket: in.MaxDaysOnMarket,
		Keywords:        in.Keywords,
		Cursor:          in.Cursor,
		Limit:           in.Limit,
	}
}
