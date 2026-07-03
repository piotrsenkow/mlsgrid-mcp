package postgres

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// rowScanner is satisfied by both pgx.Row and pgx.Rows, so the same scan
// helpers serve single-row lookups and result iteration.
type rowScanner interface {
	Scan(dest ...any) error
}

// summaryColumns is the ordered projection behind a mls.ListingSummary. Money
// and area columns are numeric in the contract; they are cast to bigint here
// (prices carry no cents) so they scan straight into int64. Nullable text is
// coalesced to ” and nullable ints to 0 to match the summary's value-typed
// fields; latitude/longitude stay nullable because absent coordinates are
// meaningful (many MLSs, including MRED, omit them). The column order MUST match
// scanSummary's Scan call.
const summaryColumns = `listing_key, listing_id,
	coalesce(standard_status, ''), coalesce(property_type, ''), coalesce(property_sub_type, ''),
	coalesce(list_price, 0)::bigint, coalesce(close_price, 0)::bigint,
	coalesce(street_number, ''), coalesce(street_dir_prefix, ''), coalesce(street_name, ''),
	coalesce(street_suffix, ''), coalesce(unit_number, ''),
	coalesce(city, ''), coalesce(postal_code, ''), coalesce(county_or_parish, ''), coalesce(state_or_province, ''),
	coalesce(bedrooms_total, 0), coalesce(bathrooms_full, 0), coalesce(bathrooms_half, 0),
	coalesce(living_area, 0)::bigint, coalesce(year_built, 0), coalesce(days_on_market, 0),
	latitude, longitude, modification_timestamp`

// detailColumns extends summaryColumns with the fields get_listing adds. It MUST
// stay aligned with scanDetail: the summary columns first, then the extras.
const detailColumns = summaryColumns + `,
	coalesce(original_list_price, 0)::bigint, coalesce(public_remarks, ''),
	coalesce(lot_size_acres, 0)::double precision,
	coalesce(association_fee, 0)::bigint, coalesce(tax_annual_amount, 0)::bigint, coalesce(tax_year, 0),
	coalesce(list_agent_full_name, ''), coalesce(list_office_name, ''),
	coalesce(photos_count, 0), coalesce(virtual_tour_url, ''), raw`

// scanSummary reads one summaryColumns row into a ListingSummary.
func scanSummary(row rowScanner) (mls.ListingSummary, error) {
	var s mls.ListingSummary
	var streetNumber, streetDir, streetName, streetSuffix, unit string
	var city, postal, county, state string
	if err := row.Scan(
		&s.ListingKey, &s.MLSNumber,
		&s.StandardStatus, &s.PropertyType, &s.PropertySubType,
		&s.ListPrice, &s.ClosePrice,
		&streetNumber, &streetDir, &streetName, &streetSuffix, &unit,
		&city, &postal, &county, &state,
		&s.Bedrooms, &s.BathroomsFull, &s.BathroomsHalf,
		&s.LivingArea, &s.YearBuilt, &s.DaysOnMarket,
		&s.Latitude, &s.Longitude, &s.ModificationTS,
	); err != nil {
		return mls.ListingSummary{}, err
	}
	s.ModificationTS = s.ModificationTS.UTC()
	s.Address = mls.Address{
		StreetNumber: streetNumber,
		// StreetName carries the directional prefix/suffix as delivered, per the
		// mls.Address contract — the contract stores them as separate columns.
		StreetName: joinNonEmpty(" ", streetDir, streetName, streetSuffix),
		UnitNumber: unit,
		City:       city,
		State:      state,
		PostalCode: postal,
		County:     county,
	}
	return s, nil
}

// scanDetail reads one detailColumns row into a ListingDetail. raw is decoded
// into ListingDetail.Raw only when includeRaw is set; otherwise it is discarded
// so the caller never pays to materialize the JSONB it did not ask for.
func scanDetail(row rowScanner, includeRaw bool) (*mls.ListingDetail, error) {
	var d mls.ListingDetail
	var streetNumber, streetDir, streetName, streetSuffix, unit string
	var city, postal, county, state string
	var rawJSON []byte
	if err := row.Scan(
		&d.ListingKey, &d.MLSNumber,
		&d.StandardStatus, &d.PropertyType, &d.PropertySubType,
		&d.ListPrice, &d.ClosePrice,
		&streetNumber, &streetDir, &streetName, &streetSuffix, &unit,
		&city, &postal, &county, &state,
		&d.Bedrooms, &d.BathroomsFull, &d.BathroomsHalf,
		&d.LivingArea, &d.YearBuilt, &d.DaysOnMarket,
		&d.Latitude, &d.Longitude, &d.ModificationTS,
		&d.OriginalListPrice, &d.PublicRemarks, &d.LotSizeAcres,
		&d.AssociationFee, &d.TaxAnnualAmount, &d.TaxYear,
		&d.ListAgentName, &d.ListOfficeName, &d.PhotosCount, &d.VirtualTourURL,
		&rawJSON,
	); err != nil {
		return nil, err
	}
	d.ModificationTS = d.ModificationTS.UTC()
	d.Address = mls.Address{
		StreetNumber: streetNumber,
		StreetName:   joinNonEmpty(" ", streetDir, streetName, streetSuffix),
		UnitNumber:   unit,
		City:         city,
		State:        state,
		PostalCode:   postal,
		County:       county,
	}
	// A listing's own modification timestamp is the honest "current as of" for a
	// single-record lookup.
	d.DataAsOf = d.ModificationTS
	if includeRaw && len(rawJSON) > 0 {
		m := map[string]any{}
		if err := json.Unmarshal(rawJSON, &m); err == nil {
			d.Raw = m
		}
		// A malformed/non-object raw payload is left as nil rather than failing
		// the whole lookup — raw is best-effort extra context, not core data.
	}
	return &d, nil
}

// joinNonEmpty joins the non-blank parts with sep, trimming surrounding space.
func joinNonEmpty(sep string, parts ...string) string {
	kept := parts[:0]
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			kept = append(kept, s)
		}
	}
	return strings.Join(kept, sep)
}

// orZeroTime returns t (in UTC) when ok, else the zero time — a small guard for
// callers that stamp a page's DataAsOf from an optional aggregate.
func orZeroTime(t time.Time, ok bool) time.Time {
	if !ok {
		return time.Time{}
	}
	return t.UTC()
}
