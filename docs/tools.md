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

## Planned

| Tool | Milestone | Purpose |
|---|---|---|
| `search_listings` | B-M2 | Area + status/type/price/beds/baths/sqft/year/DOM/keyword filters; paginated summaries |
| `get_listing` | B-M2 | Full detail by ListingKey or MLS number; `include_raw` surfaces JSONB extras |
| `get_comps` | B-M3 | Comparable sales for a subject: distance + weighted similarity + suggested range |
| `price_history` | B-M3 | Observed change timeline (price/status), total reduction, days since last change |
| `market_stats` | B-M4 | Median/avg price, $/sqft, DOM, sale-to-list, inventory, months-of-supply |
| `get_open_houses` | B-M4 | Scheduled open houses for an area and date range |
| `query_sql` | B-M5 | Opt-in read-only SQL escape hatch (off by default; DB-role + guard enforced) |
