package server

import (
	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// addressOut is the wire shape of a listing address. Locality fields are always
// present; unit is omitted when blank.
type addressOut struct {
	StreetNumber string `json:"street_number,omitempty" jsonschema:"street number as delivered"`
	StreetName   string `json:"street_name,omitempty" jsonschema:"street name including directional prefix/suffix as delivered"`
	UnitNumber   string `json:"unit_number,omitempty" jsonschema:"unit or apartment number, if any"`
	City         string `json:"city,omitempty" jsonschema:"city"`
	State        string `json:"state,omitempty" jsonschema:"two-letter state or province"`
	PostalCode   string `json:"postal_code,omitempty" jsonschema:"postal code"`
	County       string `json:"county,omitempty" jsonschema:"county or parish"`
}

// listingSummaryOut is the compact listing shape returned by search_listings.
// Its JSON is a public contract locked by the tools/list golden test.
type listingSummaryOut struct {
	ListingKey      string     `json:"listing_key" jsonschema:"MLS Grid cross-MLS unique key, e.g. MRD12345678; use it with get_listing"`
	MLSNumber       string     `json:"mls_number" jsonschema:"human-facing MLS number (ListingId); unique only within an originating system"`
	StandardStatus  string     `json:"standard_status,omitempty" jsonschema:"RESO StandardStatus, e.g. Active, Closed, Pending"`
	PropertyType    string     `json:"property_type,omitempty" jsonschema:"RESO PropertyType, e.g. Residential, Residential Income"`
	PropertySubType string     `json:"property_sub_type,omitempty" jsonschema:"RESO PropertySubType, e.g. Single Family Residence, Condominium"`
	ListPrice       int64      `json:"list_price,omitempty" jsonschema:"list price in whole US dollars"`
	ClosePrice      int64      `json:"close_price,omitempty" jsonschema:"sale price in whole US dollars; present only for closed listings"`
	Address         addressOut `json:"address" jsonschema:"decomposed street address and locality"`
	Bedrooms        int        `json:"bedrooms,omitempty" jsonschema:"total bedrooms"`
	BathroomsFull   int        `json:"bathrooms_full,omitempty" jsonschema:"full bathrooms"`
	BathroomsHalf   int        `json:"bathrooms_half,omitempty" jsonschema:"half bathrooms"`
	LivingArea      int64      `json:"living_area,omitempty" jsonschema:"living area in square feet"`
	YearBuilt       int        `json:"year_built,omitempty" jsonschema:"year the structure was built"`
	DaysOnMarket    int        `json:"days_on_market,omitempty" jsonschema:"days on market"`
	Latitude        *float64   `json:"latitude,omitempty" jsonschema:"latitude; often absent — many MLSs (incl. MRED) omit coordinates"`
	Longitude       *float64   `json:"longitude,omitempty" jsonschema:"longitude; often absent"`
	ModificationTS  string     `json:"modification_ts" jsonschema:"when MLS Grid last modified this record (RFC3339 UTC)"`
}

func toSummaryOut(s mls.ListingSummary) listingSummaryOut {
	return listingSummaryOut{
		ListingKey:      s.ListingKey,
		MLSNumber:       s.MLSNumber,
		StandardStatus:  s.StandardStatus,
		PropertyType:    s.PropertyType,
		PropertySubType: s.PropertySubType,
		ListPrice:       s.ListPrice,
		ClosePrice:      s.ClosePrice,
		Address:         toAddressOut(s.Address),
		Bedrooms:        s.Bedrooms,
		BathroomsFull:   s.BathroomsFull,
		BathroomsHalf:   s.BathroomsHalf,
		LivingArea:      s.LivingArea,
		YearBuilt:       s.YearBuilt,
		DaysOnMarket:    s.DaysOnMarket,
		Latitude:        s.Latitude,
		Longitude:       s.Longitude,
		ModificationTS:  formatTime(s.ModificationTS),
	}
}

func toAddressOut(a mls.Address) addressOut {
	return addressOut{
		StreetNumber: a.StreetNumber,
		StreetName:   a.StreetName,
		UnitNumber:   a.UnitNumber,
		City:         a.City,
		State:        a.State,
		PostalCode:   a.PostalCode,
		County:       a.County,
	}
}
