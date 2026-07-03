# mlsgrid-mcp Roadmap

Milestones are sized so each is one focused working session. Check the box when
the milestone's deliverables are merged and CI is green. The database schema
this server reads is owned by [mlsgrid-sync](https://github.com/piotrsenkow/mlsgrid-sync)
(`docs/schema-contract.md` there); this repo pins a contract **major** version.

- [x] **M1 — Scaffold + the seam + first tool.** GitHub repo (public, Apache-2.0
  + NOTICE), CI (lint / test -race / build matrix), Makefile, Dockerfile, cobra
  CLI (`serve` default, `check`, `version`), viper config loader
  (`MLSGRID_MCP_` env), CLAUDE.md, README. The public `mls.Source` interface +
  query/result types. A stdio MCP server on the official Go SDK
  (`server.New(source)`). The Postgres adapter's connection + startup
  contract-version assertion + `Capabilities` + `Freshness`. **`get_data_freshness`
  end-to-end** (in-memory protocol test + live check against a real synced DB) —
  proving the pipe from MCP client → tool → adapter → database.
- [x] **M2 — Postgres adapter query core.** `search_listings` + `get_listing`,
  fixtures built from mlsgrid-sync's migrations at a pinned tag + `seed.sql`
  (this is the cross-repo contract test), `tools/list` golden file, protocol
  `tools/call` tests per tool.
- [x] **M3 — Valuation.** `get_comps` (bbox prefilter + haversine + weighted
  similarity, no PostGIS so it stays portable) and `price_history` (listing_event
  timeline + total reduction + days-since-last-change).
- [x] **M4 — Market + events.** `market_stats` (median/avg price, $/sqft, DOM
  and cumulative DOM, sale-to-list and sale-to-original, inventory,
  months-of-supply; period comparisons via `compare_to_prior`) and
  `get_open_houses` (area + date range). Medians computed server-side with
  `percentile_cont`; a separate `mlsgrid_market` fixture schema keeps the
  aggregate assertions off the main seed's exact counts.
- [ ] **M5 — SQL escape hatch.** Opt-in `query_sql` behind `internal/sqlguard`
  (single read-only statement, deny-list, auto-LIMIT, statement timeout) plus a
  provisioned read-only DB role documented in the quickstart; injection-corpus
  tests.
- [ ] **M6 — v0.1.0 release.** `docs/tools.md` + `docs/adapters.md` polish,
  Claude Desktop / Claude Code config snippets, cross-repo CI (apply
  mlsgrid-sync migrations at a pinned tag), goreleaser + tag-triggered release,
  badges. Submit to the MCP registry / awesome-mcp — the first OSS MLS Grid MCP
  server.

## Post-1.0 candidates (demand-driven)

- Streamable HTTP transport (drags in auth questions the go-sdk makes tractable)
- SQLite adapter (once mlsgrid-sync ships its SQLite store)
- Member / Office resources as tools (agent/office lookups)
- Private out-of-tree adapters over richer stores (see `docs/adapters.md`)
