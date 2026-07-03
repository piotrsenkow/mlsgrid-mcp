# Tools

mlsgrid-mcp exposes a small set of curated tools over a mlsgrid-sync database.
Every tool returns structured JSON, expresses money in whole dollars, and
carries a data-as-of timestamp so an agent can reason about staleness. Tools are
added milestone by milestone (see [ROADMAP.md](ROADMAP.md)); this catalog lists
what is live today and what is planned.

## Live

### `get_data_freshness`

Reports how current the underlying MLS data is before an agent relies on the
other tools. Takes no arguments. Returns, per synced resource, the replication
cursor (newest listing update seen, whether the initial backfill finished, when
the last full reconcile ran), plus the listing corpus by status, media download
coverage, the schema-contract version the data conforms to, and when the
snapshot was taken.

Use it as a trust signal (is the data fresh enough to answer?) and as a
pipeline-liveness check (is the sync still running?).

Example (abridged):

```json
{
  "contract_version": "1.0.0",
  "total_listings": 8322,
  "data_as_of": "2026-07-03T06:18:03Z",
  "generated_at": "2026-07-03T12:00:00Z",
  "cursors": [
    {
      "resource": "Property",
      "originating_system": "mred",
      "stored_rows": 8322,
      "watermark": "2026-07-03T06:18:03Z",
      "backfill_complete": true
    }
  ],
  "listing_status_counts": [
    { "status": "Active", "count": 4668 },
    { "status": "Closed", "count": 1619 }
  ],
  "media_counts": [{ "status": "skipped", "count": 207028 }]
}
```

### `search_listings`

Finds listings by area, status, type, price, size, and free text. All filters
are optional and AND together. Results are ordered newest-updated first and
paginated with an opaque keyset cursor (pass a result's `next_cursor` back as
`cursor`; an empty `next_cursor` is the last page). No grand total is returned —
page through instead.

Filter notes: set at most one of `city` / `postal_code` / `county` (case-insensitive
exact match; `state` narrows further); `statuses` and `property_types` accept
several values, and a `property_types` value matches either PropertyType or
PropertySubType; `keywords` is a best-effort substring match over remarks and
address (there is no full-text index). `limit` defaults to 25 and is capped at
100. Coordinates are frequently absent (many MLSs, including MRED, omit them).

Example (abridged):

```json
// request
{ "city": "Chicago", "statuses": ["Active"], "min_price": 300000, "min_beds": 3, "limit": 25 }
// response (a full page of 25 → next_cursor set; listings array truncated here)
{
  "count": 25,
  "next_cursor": "eyJ0IjoiMjAyNi0wNi0wMVQwOTowMDowMFoiLCJrIjoiTVJEMTAwMSJ9",
  "data_as_of": "2026-06-12T09:00:00Z",
  "listings": [
    {
      "listing_key": "MRD1008", "mls_number": "1008", "standard_status": "Active",
      "property_type": "Residential", "property_sub_type": "Single Family Residence",
      "list_price": 1200000, "address": { "city": "Naperville", "state": "IL", "postal_code": "60565" },
      "bedrooms": 5, "bathrooms_full": 4, "living_area": 4200, "year_built": 2020,
      "days_on_market": 5, "modification_ts": "2026-06-12T09:00:00Z"
    }
    // … 24 more
  ]
}
```

### `get_listing`

Returns full detail for one listing by `listing_key` (the cross-MLS unique key,
preferred and unambiguous) or by `mls_number`. Because an MLS number is unique
only within an originating system, a number that exists in more than one feed is
reported as ambiguous unless you also pass `originating_system`. Set
`include_raw` to also return the source's scope-filtered extra fields (investment
/ expense fields, feature arrays, MLS-local keys) as a raw JSON object.

Example (abridged):

```json
// request
{ "listing_key": "MRD1003", "include_raw": false }
// response
{
  "listing_key": "MRD1003", "mls_number": "1003", "standard_status": "Closed",
  "list_price": 500000, "original_list_price": 520000, "close_price": 485000,
  "address": { "street_number": "1500", "street_name": "N Ridge Ave", "city": "Evanston", "state": "IL", "postal_code": "60201" },
  "bedrooms": 4, "bathrooms_full": 3, "living_area": 2600, "year_built": 1988,
  "lot_size_acres": 0.25, "tax_annual_amount": 8200, "tax_year": 2025,
  "list_agent_name": "Jane Broker", "list_office_name": "North Shore Realty",
  "photos_count": 25, "data_as_of": "2026-05-20T09:00:00Z"
}
```

### `price_history`

Returns a listing's observed price/status change timeline from mlsgrid-sync's
append-only change capture — new listing, price change, status change, back on
market, delisted — with `total_reduction_pct` (net list-price change as a percent
of the first observed price; positive means the price came down) and
`days_since_last_change`. Identify the listing the same way as `get_listing`.

**Best-effort from first sync forward:** there is no event for a change that
predates this database's tracking of the listing, so a short or empty timeline
does not mean the listing never changed. Good for spotting motivated sellers
(repeated cuts, long time since last change).

```json
// request
{ "listing_key": "MRD1003" }
// response
{
  "listing_key": "MRD1003",
  "total_reduction_pct": 3.85,
  "days_since_last_change": 44,
  "data_as_of": "2026-05-20T09:00:00Z",
  "events": [
    { "event_type": "price_change", "old_value": "520000", "new_value": "500000", "observed_at": "2026-05-10T09:00:00Z" },
    { "event_type": "status_change", "old_value": "Active", "new_value": "Closed", "observed_at": "2026-05-20T09:00:00Z" }
  ]
}
```

### `get_comps`

Finds comparable recent CLOSED sales for a subject and summarizes a suggested
value range. Identify the subject by reference (`listing_key`, or `mls_number`
[+ `originating_system`]) or inline: an area (`city` / `postal_code` / `county` —
required unless you pass `latitude`+`longitude`) plus what you know
(`property_type`, `living_area`, `bedrooms`, `bathrooms_full`, `year_built`).

Comps are same-product-type closings in the subject's area within the lookback
window (`closed_within_days`, default ~180). When the subject has coordinates the
pool is limited to `radius_miles` (default ~1) and distance feeds the score;
**most feeds, including MRED, omit coordinates**, so comps then fall back to the
same city/area and `distance_miles` is omitted. Each comp has a 0..1 `similarity`
(a heuristic over size, beds/baths, age, and distance when available — living
area ~40%, beds/baths ~35%, age ~15%, distance ~10%, renormalized over whatever
the subject specifies) plus `adjust_notes`. The suggested range is the
interquartile $/sqft × the subject's size, or interquartile close price when size
is unknown. It is decision support, not an appraisal — with few nearby sales the
range widens.

```json
// request (inline spec, no coordinates → area fallback, distance omitted)
{ "city": "Evanston", "property_type": "Residential", "living_area": 2700, "bedrooms": 4 }
// response (comparables abridged)
{
  "count": 2,
  "median_close_price": 542500, "median_ppsf": 197,
  "suggested_low": 517410, "suggested_high": 544887,
  "data_as_of": "2026-06-12T09:00:00Z",
  "comparables": [
    {
      "listing_key": "MRD1010", "standard_status": "Closed", "close_price": 600000,
      "living_area": 2900, "similarity": 0.94,
      "adjust_notes": ["200 sqft larger", "same beds", "6 yrs newer"]
    }
    // … 1 more
  ]
}
```

### `market_stats`

Aggregates an area's market over a trailing window. Requires an area (`city` /
`postal_code` / `county` / `state` — it will not aggregate the whole feed);
optionally narrow by `property_types` and set `period_days` (default ~90). Money
is in whole dollars.

Returns a current for-sale snapshot — `median_list_price`, `active_inventory`,
and `months_of_supply` (active inventory ÷ the monthly closed-sale rate; under ~6
favors sellers, over ~6 favors buyers) — plus metrics over sales that closed in
the period: median/avg close price, `median_ppsf`, `median_days_on_market` and
`median_cumulative_days_on_market` (a gap between the two flags relist / DOM-reset
gaming), and sale-to-list two ways — `sale_to_list_ratio` (vs the final list
price) and `sale_to_original_ratio` (vs the original ask). Set `compare_to_prior`
to also get the immediately preceding window as `prior` (closed-sale metrics
only; inventory is a current snapshot and is not reconstructed for the past).

These are medians over whatever closed in the window, so a thin area or short
period yields small samples — widen `period_days` when counts are low.

```json
// request
{ "city": "Evanston", "property_types": ["Residential"], "period_days": 90, "compare_to_prior": true }
// response (abridged)
{
  "period_days": 90,
  "median_list_price": 615000, "active_inventory": 48, "months_of_supply": 2.4,
  "median_close_price": 500000, "avg_close_price": 500000, "median_ppsf": 200,
  "median_days_on_market": 40, "median_cumulative_days_on_market": 45,
  "sale_to_list_ratio": 0.9783, "sale_to_original_ratio": 0.9574,
  "closed_in_period": 60,
  "prior": {
    "period_days": 90, "median_close_price": 485000, "closed_in_period": 71,
    "median_days_on_market": 33, "sale_to_list_ratio": 0.99
  },
  "data_as_of": "2026-06-30T09:00:00Z"
}
```

### `get_open_houses`

Lists scheduled open houses for an area and date range. Optionally scope by area
(`city` / `postal_code` / `county` / `state`) and bound with `from` / `to`
(`YYYY-MM-DD` or RFC3339); defaults to the next 7 days from today. Ordered by
date then start time; `limit` defaults to 25 (capped at 100). Each entry carries
the `listing_key` (use it with `get_listing`), the address, date, start/end
times, and any remarks. The schedule is only as current as the last sync —
`data_as_of` says when that was, and an open house synced days ago may since have
been cancelled.

```json
// request
{ "city": "Evanston", "from": "2026-07-04", "to": "2026-07-05" }
// response (abridged)
{
  "count": 2,
  "data_as_of": "2026-07-01T09:00:00Z",
  "open_houses": [
    {
      "listing_key": "MRD1010", "mls_number": "1010",
      "address": { "street_name": "Sheridan Rd", "city": "Evanston", "state": "IL" },
      "date": "2026-07-04", "start_time": "2026-07-04T16:00:00Z", "end_time": "2026-07-04T18:00:00Z",
      "remarks": "Twilight open house"
    }
    // … 1 more
  ]
}
```

## Planned

| Tool | Milestone | Purpose |
|---|---|---|
| `query_sql` | B-M5 | Opt-in read-only SQL escape hatch (off by default; DB-role + guard enforced) |
