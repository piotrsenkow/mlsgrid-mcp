# CLAUDE.md — mlsgrid-mcp

## What this is

Public OSS Go **Model Context Protocol (MCP) server** that exposes curated
real-estate query tools over the PostgreSQL database produced by its companion
project [`mlsgrid-sync`](https://github.com/piotrsenkow/mlsgrid-sync). Built
milestone by milestone; **start every session by reading `docs/ROADMAP.md` and
picking the next unchecked milestone.**

The design centers on one seam: the public `mls.Source` interface
(capability-shaped, zero driver/SDK deps). An adapter implements it over a data
store; `server.New` turns any `Source` into MCP tools. This is what lets a
private data source implement `Source` out-of-tree and reuse every tool.

## Commands

- `make build` / `make test` (unit, -race) / `make test-integration` (needs Docker; `//go:build integration`; lands B-M2)
- `make lint` (golangci-lint) / `make fmt`
- Run locally: `go run ./cmd/mlsgrid-mcp` (serves stdio) or `... check` (freshness readout)

## Hard rules

1. **This server is read-only.** It never writes to the mlsgrid-sync database.
   The Postgres adapter opens sessions with `default_transaction_read_only=on`;
   keep it that way. The opt-in SQL tool (B-M5) is additionally gated on a
   read-only DB role.
2. **No secrets in files.** The connection string comes only from
   `MLSGRID_MCP_DATABASE_URL` (or `database.url`, which the example leaves
   empty). Never log DSNs.
3. **The schema is a pinned contract, not ours to change.** The schema is owned
   by mlsgrid-sync (`docs/schema-contract.md` there). This repo pins a contract
   **major** version (`adapters/postgres.ExpectedContractMajor`) and the adapter
   asserts it against `schema_meta` at startup, failing loud on a major
   mismatch. Never reach for columns outside the contract.
4. **stdout belongs to the transport.** While serving over stdio, stdout is the
   JSON-RPC channel. All logs go to stderr (slog is configured that way). Never
   `fmt.Print` to stdout from server code paths.
5. **No test may ever touch `api.mlsgrid.com`** (carried over from mlsgrid-sync)
   and no test needs live MLS data — tools are tested against fixtures and fakes.
6. **This is a clean-room public project.** Do not copy code from, or refer in
   code/comments/docs to, the private `gosyncmls` repo or Realytica internals.
   Forbidden concepts: county allowlists, `idx_eligible` logic, Cook County PIN
   geocoding, `ra_pid` naming, hardcoded `mred` (the originating system is
   always data-driven).
7. **No AI attribution anywhere public.** No Claude/AI co-author trailers,
   "Generated with" lines, or AI mentions in commits, PR titles/bodies, release
   notes, or issues. Commit as the repo owner only.

## Conventions

- Go, cobra + viper, pgx/v5 (no ORM), official MCP Go SDK
  (`github.com/modelcontextprotocol/go-sdk`). The SDK API still moves between
  minor versions — pull current docs before changing MCP wiring.
- Money is whole dollars (int64); every tool result carries a data-as-of
  timestamp so agents can reason about staleness.
- Tool schemas are inferred from typed Go structs. Their JSON shape is a public
  contract — a `tools/list` golden test locks it. Change field names/tags
  deliberately.
- Tool descriptions are persona-aware (agent/investor/buyer/seller routing
  cues) and honest about limits (e.g. history is best-effort from first sync).
- Package layout: `mls` (interface + types, public, no deps), `server`
  (+ tools, registers on the SDK), `adapters/postgres` (default adapter),
  `internal/config` (viper loader), `internal/cli` (thin cobra), `internal/sqlguard`
  (SQL escape-hatch validation, B-M5).

## Key references

- `docs/ROADMAP.md` — milestone checklist; pick the next unchecked box
- mlsgrid-sync `docs/schema-contract.md` — the schema this server reads
- `docs/tools.md` — tool catalog (grows per milestone)
- `docs/adapters.md` — how to write a private out-of-tree adapter
- MCP Go SDK: https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp
