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

## Planned

| Tool | Milestone | Purpose |
|---|---|---|
| `get_comps` | B-M3 | Comparable sales for a subject: distance + weighted similarity + suggested range |
| `price_history` | B-M3 | Observed change timeline (price/status), total reduction, days since last change |
| `market_stats` | B-M4 | Median/avg price, $/sqft, DOM, sale-to-list, inventory, months-of-supply |
| `get_open_houses` | B-M4 | Scheduled open houses for an area and date range |
| `query_sql` | B-M5 | Opt-in read-only SQL escape hatch (off by default; DB-role + guard enforced) |
