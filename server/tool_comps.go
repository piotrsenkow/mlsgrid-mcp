package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const compsDescription = `Find comparable recent sales for a property and summarize a suggested value range.

Give a subject either by reference (listing_key, or mls_number [+ originating_system]) or inline: an area (city / postal_code / county — required unless you pass latitude+longitude) plus whatever you know (property_type, living_area, bedrooms, bathrooms_full, year_built).

Comps are recent CLOSED sales of the same product type in the subject's area (default: last ~180 days). When the subject has coordinates the pool is limited to a radius and distance feeds the score; most feeds (including MRED) omit coordinates, so comps then fall back to the same city/area and distance is omitted. Each comp carries a 0..1 similarity score (a heuristic over size, beds/baths, age, and distance when available) with short adjust_notes explaining it. The result includes median_close_price, median_ppsf, and a suggested_low/suggested_high range (interquartile $/sqft × the subject's size, or interquartile close price when size is unknown).

Use it for a quick CMA (agents), resale/ARV sanity checks (investors), or to justify an offer (buyers). It is decision support, not an appraisal: with few nearby sales the range widens and confidence drops.`

// compsInput identifies the subject (by reference or inline spec) and tunes the
// comp search. Field names/tags are a public contract (golden test).
type compsInput struct {
	ListingKey        string   `json:"listing_key,omitempty" jsonschema:"subject by MLS Grid key (preferred)"`
	MLSNumber         string   `json:"mls_number,omitempty" jsonschema:"subject by MLS number; add originating_system if it exists in more than one feed"`
	OriginatingSystem string   `json:"originating_system,omitempty" jsonschema:"originating system slug to disambiguate mls_number"`
	City              string   `json:"city,omitempty" jsonschema:"inline subject area: city (required unless coordinates or another area field is given)"`
	PostalCode        string   `json:"postal_code,omitempty" jsonschema:"inline subject area: postal code"`
	County            string   `json:"county,omitempty" jsonschema:"inline subject area: county or parish"`
	State             string   `json:"state,omitempty" jsonschema:"inline subject area: two-letter state"`
	PropertyType      string   `json:"property_type,omitempty" jsonschema:"inline subject product type, e.g. Residential"`
	LivingArea        int64    `json:"living_area,omitempty" jsonschema:"inline subject living area in square feet (drives the suggested range)"`
	Bedrooms          int      `json:"bedrooms,omitempty" jsonschema:"inline subject bedrooms"`
	BathroomsFull     int      `json:"bathrooms_full,omitempty" jsonschema:"inline subject full bathrooms"`
	YearBuilt         int      `json:"year_built,omitempty" jsonschema:"inline subject year built"`
	Latitude          *float64 `json:"latitude,omitempty" jsonschema:"inline subject latitude; enables radius + distance scoring"`
	Longitude         *float64 `json:"longitude,omitempty" jsonschema:"inline subject longitude"`
	RadiusMiles       float64  `json:"radius_miles,omitempty" jsonschema:"search radius in miles when the subject has coordinates (default ~1)"`
	ClosedWithinDays  int      `json:"closed_within_days,omitempty" jsonschema:"only consider sales closed within this many days (default ~180)"`
	Limit             int      `json:"limit,omitempty" jsonschema:"maximum comps to return (default 5, capped at 25)"`
}

type compOut struct {
	listingSummaryOut
	DistanceMiles *float64 `json:"distance_miles,omitempty" jsonschema:"miles from the subject; omitted when coordinates are unavailable"`
	Similarity    float64  `json:"similarity" jsonschema:"0..1 heuristic similarity to the subject"`
	AdjustNotes   []string `json:"adjust_notes,omitempty" jsonschema:"human-readable rationale for the score (size/bed/age/distance deltas)"`
}

// compsOutput is the wire shape of get_comps.
type compsOutput struct {
	Comparables      []compOut `json:"comparables" jsonschema:"comparable sales, most similar first"`
	Count            int       `json:"count" jsonschema:"number of comparables returned"`
	MedianClosePrice int64     `json:"median_close_price,omitempty" jsonschema:"median comp sale price in whole US dollars"`
	MedianPPSF       int64     `json:"median_ppsf,omitempty" jsonschema:"median comp price per square foot in whole US dollars"`
	SuggestedLow     int64     `json:"suggested_low,omitempty" jsonschema:"low end of the suggested value range in whole US dollars"`
	SuggestedHigh    int64     `json:"suggested_high,omitempty" jsonschema:"high end of the suggested value range in whole US dollars"`
	DataAsOf         string    `json:"data_as_of,omitempty" jsonschema:"how current the underlying data is (RFC3339 UTC)"`
}

func registerComps(srv *mcp.Server, source mls.Source) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_comps",
		Description: compsDescription,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in compsInput) (*mcp.CallToolResult, compsOutput, error) {
		q, err := toCompsQuery(in)
		if err != nil {
			return nil, compsOutput{}, err
		}
		res, err := source.FindComparables(ctx, q)
		if err != nil {
			switch {
			case errors.Is(err, mls.ErrNotFound):
				return nil, compsOutput{}, fmt.Errorf("subject listing not found")
			case errors.Is(err, mls.ErrAmbiguousRef):
				return nil, compsOutput{}, fmt.Errorf("mls_number %q matches multiple originating systems — set originating_system", in.MLSNumber)
			default:
				return nil, compsOutput{}, err
			}
		}
		return nil, toCompsOutput(res), nil
	})
}

func toCompsQuery(in compsInput) (mls.CompsQuery, error) {
	q := mls.CompsQuery{RadiusMiles: in.RadiusMiles, Limit: in.Limit}
	if in.ClosedWithinDays > 0 {
		q.ClosedWithin = time.Duration(in.ClosedWithinDays) * 24 * time.Hour
	}

	if ref := (mls.ListingRef{Key: in.ListingKey, MLSNumber: in.MLSNumber, OriginatingSystem: in.OriginatingSystem}); !ref.Empty() {
		q.Subject = ref
		return q, nil
	}

	spec := mls.CompSpec{
		Area:          mls.Area{City: in.City, PostalCode: in.PostalCode, County: in.County, State: in.State},
		PropertyType:  in.PropertyType,
		LivingArea:    in.LivingArea,
		Bedrooms:      in.Bedrooms,
		BathroomsFull: in.BathroomsFull,
		YearBuilt:     in.YearBuilt,
		Latitude:      in.Latitude,
		Longitude:     in.Longitude,
	}
	// Require a geographic scope so comps are focused rather than a whole-market
	// scan: an area field, or coordinates.
	hasArea := spec.Area.City != "" || spec.Area.PostalCode != "" || spec.Area.County != "" || spec.Area.State != ""
	hasGeo := spec.Latitude != nil && spec.Longitude != nil
	if !hasArea && !hasGeo {
		return q, errors.New("provide a subject listing_key/mls_number, or an inline spec with an area (city/postal_code/county) or latitude+longitude")
	}
	q.Spec = &spec
	return q, nil
}

func toCompsOutput(r *mls.CompsResult) compsOutput {
	out := compsOutput{
		Count:            len(r.Comparables),
		MedianClosePrice: r.MedianClosePrice,
		MedianPPSF:       r.MedianPPSF,
		SuggestedLow:     r.SuggestedLow,
		SuggestedHigh:    r.SuggestedHigh,
		DataAsOf:         formatTime(r.DataAsOf),
	}
	out.Comparables = make([]compOut, 0, len(r.Comparables))
	for _, c := range r.Comparables {
		out.Comparables = append(out.Comparables, compOut{
			listingSummaryOut: toSummaryOut(c.ListingSummary),
			DistanceMiles:     c.DistanceMiles,
			Similarity:        c.Similarity,
			AdjustNotes:       c.AdjustNotes,
		})
	}
	return out
}
