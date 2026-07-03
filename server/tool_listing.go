package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const listingDescription = `Fetch full detail for one listing by its ListingKey or MLS number.

Prefer listing_key (the cross-MLS unique key, e.g. MRD12345678) — it is unambiguous and is what search_listings returns. mls_number is the human-facing MLS number and is unique only within an originating system, so if the same number exists in more than one feed you must also pass originating_system; otherwise the lookup reports the reference as ambiguous.

Set include_raw to also return the source's scope-filtered extra fields (investment/expense fields, feature arrays, MLS-local keys) as a raw JSON object — useful for details the curated columns do not cover.

Returns list/original/close price (whole US dollars), beds/baths/size/year, address, agent and office, taxes and association fee, photo count and virtual tour, and a data_as_of timestamp. Money is whole dollars; coordinates are often absent.`

// listingInput is the argument shape of get_listing.
type listingInput struct {
	ListingKey        string `json:"listing_key,omitempty" jsonschema:"MLS Grid cross-MLS unique key (preferred, unambiguous)"`
	MLSNumber         string `json:"mls_number,omitempty" jsonschema:"human-facing MLS number; unique only within an originating system"`
	OriginatingSystem string `json:"originating_system,omitempty" jsonschema:"originating system slug (e.g. mred) to disambiguate mls_number"`
	IncludeRaw        bool   `json:"include_raw,omitempty" jsonschema:"also return the scope-filtered raw JSONB extras as a raw object"`
}

// listingDetailOut is the wire shape of get_listing. It carries every
// listingSummaryOut field (promoted) plus the detail-only fields.
type listingDetailOut struct {
	listingSummaryOut
	OriginalListPrice int64          `json:"original_list_price,omitempty" jsonschema:"original list price in whole US dollars"`
	PublicRemarks     string         `json:"public_remarks,omitempty" jsonschema:"public marketing remarks"`
	LotSizeAcres      float64        `json:"lot_size_acres,omitempty" jsonschema:"lot size in acres"`
	AssociationFee    int64          `json:"association_fee,omitempty" jsonschema:"association/HOA fee in whole US dollars"`
	TaxAnnualAmount   int64          `json:"tax_annual_amount,omitempty" jsonschema:"annual property tax in whole US dollars"`
	TaxYear           int            `json:"tax_year,omitempty" jsonschema:"tax year the amount applies to"`
	ListAgentName     string         `json:"list_agent_name,omitempty" jsonschema:"listing agent full name (attribution)"`
	ListOfficeName    string         `json:"list_office_name,omitempty" jsonschema:"listing office name (attribution)"`
	PhotosCount       int            `json:"photos_count,omitempty" jsonschema:"number of photos on the listing"`
	VirtualTourURL    string         `json:"virtual_tour_url,omitempty" jsonschema:"unbranded virtual tour URL, if any"`
	Raw               map[string]any `json:"raw,omitempty" jsonschema:"scope-filtered raw extras; present only when include_raw is set"`
	DataAsOf          string         `json:"data_as_of" jsonschema:"how current this record is (its modification timestamp, RFC3339 UTC)"`
}

func registerListing(srv *mcp.Server, source mls.Source) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_listing",
		Description: listingDescription,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listingInput) (*mcp.CallToolResult, listingDetailOut, error) {
		ref := mls.ListingRef{Key: in.ListingKey, MLSNumber: in.MLSNumber, OriginatingSystem: in.OriginatingSystem}
		if ref.Empty() {
			return nil, listingDetailOut{}, errors.New("provide listing_key or mls_number")
		}
		detail, err := source.GetListing(ctx, ref, mls.ListingOptions{IncludeRaw: in.IncludeRaw})
		if err != nil {
			switch {
			case errors.Is(err, mls.ErrNotFound):
				return nil, listingDetailOut{}, fmt.Errorf("no listing matches that reference")
			case errors.Is(err, mls.ErrAmbiguousRef):
				return nil, listingDetailOut{}, fmt.Errorf("mls_number %q matches multiple originating systems — set originating_system", in.MLSNumber)
			default:
				return nil, listingDetailOut{}, err
			}
		}
		return nil, toDetailOut(detail), nil
	})
}

func toDetailOut(d *mls.ListingDetail) listingDetailOut {
	return listingDetailOut{
		listingSummaryOut: toSummaryOut(d.ListingSummary),
		OriginalListPrice: d.OriginalListPrice,
		PublicRemarks:     d.PublicRemarks,
		LotSizeAcres:      d.LotSizeAcres,
		AssociationFee:    d.AssociationFee,
		TaxAnnualAmount:   d.TaxAnnualAmount,
		TaxYear:           d.TaxYear,
		ListAgentName:     d.ListAgentName,
		ListOfficeName:    d.ListOfficeName,
		PhotosCount:       d.PhotosCount,
		VirtualTourURL:    d.VirtualTourURL,
		Raw:               d.Raw,
		DataAsOf:          formatTime(d.DataAsOf),
	}
}
