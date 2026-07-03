package mls

import "time"

// ----------------------------------------------------------------------------
// Capabilities & Freshness — fully exercised by B-M1 (get_data_freshness).
// ----------------------------------------------------------------------------

// Capabilities describes what a Source can answer and over what data, so tools
// can adapt their behavior and set honest expectations in their output.
type Capabilities struct {
	// SchemaContractVersion is the mlsgrid-sync schema-contract version the
	// source was built against (e.g. "1.0.0"). Tools may surface it; the
	// adapter asserts compatibility at startup.
	SchemaContractVersion string
	// OriginatingSystems lists the MLS feeds present in the source (e.g.
	// ["mred"]).
	OriginatingSystems []string
	// Geo is true when listings carry latitude/longitude, enabling
	// distance-aware comps. Many MLSs (including MRED) omit coordinates.
	Geo bool
	// PriceHistory is true when change-capture events are available.
	PriceHistory bool
	// HistorySince, when non-zero, is the earliest point from which change
	// history is complete — history is best-effort from first sync forward.
	HistorySince time.Time
	// OpenHouses is true when open-house data is synced.
	OpenHouses bool
	// SQL is true when the source also implements SQLQuerier and the server is
	// configured to expose the query_sql tool.
	SQL bool
	// Statuses lists the distinct StandardStatus values present, so tools can
	// hint valid filter values.
	Statuses []string
}

// Freshness is a point-in-time report of how current the data is. It is the
// structured result behind the get_data_freshness tool.
type Freshness struct {
	// SchemaContractVersion is the live contract version read from the store.
	SchemaContractVersion string
	// Cursors reports replication state per resource + originating system.
	Cursors []ResourceCursor
	// ListingStatusCounts breaks the property corpus down by StandardStatus,
	// most populous first.
	ListingStatusCounts []StatusCount
	// TotalListings is the total property rows in the source.
	TotalListings int64
	// MediaCounts breaks media rows down by storage status (downloaded,
	// pending, failed, skipped); empty when the source tracks no media.
	MediaCounts []StatusCount
	// DataAsOf is the newest modification timestamp observed across resources —
	// the practical "data is current as of" moment. Zero when unknown.
	DataAsOf time.Time
	// GeneratedAt is when this report was produced.
	GeneratedAt time.Time
}

// ResourceCursor is the replication state for one synced resource.
type ResourceCursor struct {
	// Resource is the RESO resource name ("Property", "OpenHouse").
	Resource string
	// OriginatingSystem is the MLS feed slug (e.g. "mred").
	OriginatingSystem string
	// StoredRows is how many rows of this resource are held locally.
	StoredRows int64
	// Watermark is the newest ModificationTimestamp synced (the incremental
	// cursor). Nil before any sync has run.
	Watermark *time.Time
	// BackfillComplete reports whether the initial full import finished;
	// incremental sync only runs once it has.
	BackfillComplete bool
	// LastReconcile is when the last full-feed reconcile sweep completed, or
	// nil if never.
	LastReconcile *time.Time
}

// StatusCount is a labeled count used for status/media breakdowns.
type StatusCount struct {
	Status string
	Count  int64
}

// ----------------------------------------------------------------------------
// Search — search_listings (B-M2). Fields track the tool spec; refined later.
// ----------------------------------------------------------------------------

// Area targets a geographic scope. At most one of City / PostalCode / County
// should be set; an empty Area means "anywhere in the source".
type Area struct {
	City       string
	PostalCode string
	County     string // CountyOrParish
	State      string // StateOrProvince
}

// SearchQuery filters the listing corpus. Zero-valued fields are not applied.
type SearchQuery struct {
	Area            Area
	Statuses        []string // StandardStatus values; empty means any
	PropertyTypes   []string // PropertyType / PropertySubType
	MinPrice        int64    // whole dollars; 0 = unbounded
	MaxPrice        int64
	MinBeds         int
	MinBathsFull    int
	MinLivingArea   int64 // sqft
	MaxLivingArea   int64
	MinYearBuilt    int
	MaxDaysOnMarket int
	Keywords        string // free-text match over remarks/address
	// Cursor is an opaque pagination cursor from a prior Page.NextCursor.
	Cursor string
	// Limit caps the page size; the source clamps it to a sane maximum.
	Limit int
}

// ListingSummary is the compact listing shape returned by searches and comps.
type ListingSummary struct {
	ListingKey      string
	MLSNumber       string
	StandardStatus  string
	PropertyType    string
	PropertySubType string
	ListPrice       int64 // whole dollars
	ClosePrice      int64 // whole dollars; 0 unless closed
	Address         Address
	Bedrooms        int
	BathroomsFull   int
	BathroomsHalf   int
	LivingArea      int64 // sqft
	YearBuilt       int
	DaysOnMarket    int
	Latitude        *float64
	Longitude       *float64
	ModificationTS  time.Time
}

// Address is a decomposed street address plus normalized locality fields.
type Address struct {
	StreetNumber string
	StreetName   string // includes dir prefix/suffix as delivered
	UnitNumber   string
	City         string
	State        string
	PostalCode   string
	County       string
}

// ----------------------------------------------------------------------------
// Listing detail — get_listing (B-M2).
// ----------------------------------------------------------------------------

// ListingOptions tunes GetListing output.
type ListingOptions struct {
	// IncludeRaw surfaces the source's scope-filtered raw JSONB extras.
	IncludeRaw bool
}

// ListingDetail is the full single-listing shape.
type ListingDetail struct {
	ListingSummary
	OriginalListPrice int64
	PublicRemarks     string
	LotSizeAcres      float64
	AssociationFee    int64
	TaxAnnualAmount   int64
	TaxYear           int
	ListAgentName     string
	ListOfficeName    string
	PhotosCount       int
	VirtualTourURL    string
	// Raw holds scope-filtered extra fields when ListingOptions.IncludeRaw is
	// set; nil otherwise.
	Raw      map[string]any
	DataAsOf time.Time
}

// ----------------------------------------------------------------------------
// Comparables — get_comps (B-M3).
// ----------------------------------------------------------------------------

// CompsQuery describes the subject and comp-selection constraints. Either
// Subject (an existing listing) or Spec (an inline description) is required.
type CompsQuery struct {
	Subject      ListingRef
	Spec         *CompSpec
	RadiusMiles  float64       // default applied by the source (~1mi)
	ClosedWithin time.Duration // default ~6mo; 0 uses the source default
	Limit        int
}

// CompSpec describes a subject property inline when no listing exists to point at.
type CompSpec struct {
	Area          Area
	PropertyType  string
	LivingArea    int64
	Bedrooms      int
	BathroomsFull int
	YearBuilt     int
	Latitude      *float64
	Longitude     *float64
}

// Comparable is one scored comp.
type Comparable struct {
	ListingSummary
	DistanceMiles *float64 // nil when the source lacks coordinates
	Similarity    float64  // 0..1 weighted score
	AdjustNotes   []string // human-readable rationale for the score
}

// CompsResult wraps comps with a suggested valuation summary.
type CompsResult struct {
	Comparables      []Comparable
	MedianClosePrice int64
	MedianPPSF       int64 // median price per square foot, whole dollars
	SuggestedLow     int64
	SuggestedHigh    int64
	DataAsOf         time.Time
}

// ----------------------------------------------------------------------------
// Market stats — market_stats (B-M4).
// ----------------------------------------------------------------------------

// StatsQuery scopes a market-statistics aggregation.
type StatsQuery struct {
	Area          Area
	PropertyTypes []string
	// Period is the trailing window to aggregate over; the source applies a
	// default (e.g. 90 days) when zero.
	Period time.Duration
	// CompareToPrior requests a delta versus the immediately preceding period.
	CompareToPrior bool
}

// MarketStats is an aggregate market snapshot.
type MarketStats struct {
	Area               Area
	MedianListPrice    int64
	MedianClosePrice   int64
	AvgClosePrice      int64
	MedianPPSF         int64
	MedianDaysOnMarket int
	SaleToListRatio    float64
	ActiveInventory    int64
	ClosedInPeriod     int64
	MonthsOfSupply     float64
	Prior              *MarketStats // populated when CompareToPrior is set
	DataAsOf           time.Time
}

// ----------------------------------------------------------------------------
// Price history — price_history (B-M3).
// ----------------------------------------------------------------------------

// PriceEvent is one observed change in a listing's life.
type PriceEvent struct {
	// EventType is one of new_listing, price_change, status_change,
	// back_on_market, delisted.
	EventType  string
	OldValue   string
	NewValue   string
	ObservedAt time.Time
}

// PriceHistory is a listing's change timeline plus derived summary metrics.
type PriceHistory struct {
	ListingKey          string
	Events              []PriceEvent
	TotalReductionPct   float64
	DaysSinceLastChange int
	DataAsOf            time.Time
}

// ----------------------------------------------------------------------------
// Open houses — get_open_houses (B-M4).
// ----------------------------------------------------------------------------

// OpenHouseQuery scopes an open-house lookup.
type OpenHouseQuery struct {
	Area  Area
	From  time.Time // inclusive; zero means now
	To    time.Time // inclusive; zero means From + ~7 days
	Limit int
}

// OpenHouse is one scheduled open house.
type OpenHouse struct {
	ListingKey string
	MLSNumber  string
	Address    Address
	Date       time.Time
	StartTime  time.Time
	EndTime    time.Time
	Remarks    string
}
