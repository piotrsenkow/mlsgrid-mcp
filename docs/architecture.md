# Architecture

mlsgrid-mcp is a thin, read-only projection of a mlsgrid-sync database onto the
Model Context Protocol. The whole design is one seam plus a set of tools built
on it.

```
  MCP client (Claude Code / Desktop)
        │  JSON-RPC over stdio
        ▼
  ┌───────────────┐     registers tools on
  │  server (New) │───────────────────────────▶  mcp.Server  (go-sdk)
  └───────┬───────┘
          │ calls
          ▼
  ┌───────────────┐   mls.Source (capability-shaped, no deps)
  │     mls       │◀───────────────────────────┐
  └───────────────┘                             │ implements
                                                │
                                    ┌───────────────────────┐
                                    │  adapters/postgres     │  read-only pgx pool
                                    └───────────┬───────────┘
                                                │ SELECTs (contract v1)
                                                ▼
                                    PostgreSQL  (mlsgrid-sync output)
```

## Layers

- **`mls`** — the public interface (`Source`) and the query/result types tools
  exchange. Zero dependencies on any driver or the MCP SDK, so anyone can
  implement it. Money is whole dollars; every result carries a data-as-of time.
- **`server`** — `New(source, opts...)` builds a go-sdk `mcp.Server` and
  registers the curated tools. Tools are typed Go functions; their JSON schemas
  are inferred from the input/output structs and locked by a golden test.
- **`adapters/postgres`** — the default `Source`, reading the mlsgrid-sync
  schema over a pgx/v5 pool. Read-only at the session level; asserts the schema
  contract major version at startup.
- **`internal/cli`, `internal/config`** — a thin cobra/viper front end. `serve`
  (the default) runs the stdio server; `check` prints a freshness readout.

## Why a pinned contract, not a shared module

The database schema is owned and versioned by mlsgrid-sync (its
`docs/schema-contract.md`). mlsgrid-mcp pins a contract **major** version and
asserts it at startup rather than sharing a Go module, so the two projects
release independently. An additive (minor) schema change never breaks this
server; a breaking (major) change fails loud at connect time instead of
returning wrong data.

## Transport

stdio only in v1 — it covers the daily-driver targets (Claude Code, Claude
Desktop). Streamable HTTP is a post-1.0 candidate; the go-sdk makes the swap
small, but it brings authentication questions worth handling deliberately.

## Read-only by construction

The server never writes. The Postgres adapter sets
`default_transaction_read_only=on` on every session, and the (future) opt-in SQL
tool is additionally gated behind a provisioned read-only database role. Belt
and suspenders: the point is that no tool, and no SQL an agent might pass, can
mutate the feed.
