# mlsgrid-mcp

[![CI](https://github.com/piotrsenkow/mlsgrid-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/piotrsenkow/mlsgrid-mcp/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/piotrsenkow/mlsgrid-mcp.svg)](https://pkg.go.dev/github.com/piotrsenkow/mlsgrid-mcp)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

A [Model Context Protocol](https://modelcontextprotocol.io) server that lets AI
agents query real-estate (RESO / MLS Grid) data — search listings, pull comps,
read market stats — over a database you populate with its companion project,
[`mlsgrid-sync`](https://github.com/piotrsenkow/mlsgrid-sync).

> **Status: early (v0, pre-release).** The architecture is in place — the public
> `mls.Source` seam, a stdio server on the official Go SDK, and the Postgres
> adapter — and the first tool, `get_data_freshness`, works end to end. The
> query tools (`search_listings`, `get_comps`, `market_stats`, …) are landing
> milestone by milestone; see the [roadmap](docs/ROADMAP.md).

## How it fits together

```
MLS Grid API ──▶ mlsgrid-sync ──▶ PostgreSQL ──▶ mlsgrid-mcp ──▶ your AI agent
                 (replication)     (your data)    (this repo)     (Claude, …)
```

`mlsgrid-sync` does the replication and owns the [schema
contract](https://github.com/piotrsenkow/mlsgrid-sync/blob/main/docs/schema-contract.md).
`mlsgrid-mcp` is a thin, **read-only** projection of that schema onto MCP tools.
It pins the contract's major version and checks it at startup, so the two
projects release independently.

## Compliance — read this first

**This server ships no MLS data and no credentials.** It reads a database you
populated under your own license. When you expose that data to an agent:

1. You must hold an **executed [MLS Grid Data License Agreement](https://www.mlsgrid.com/s/MLS-Grid-Data-License-Agreement.pdf)**
   and per-MLS approval for the feeds in your database.
2. Anything an agent produces from the data is still bound by your license
   tier's display and distribution rules (IDX/VOW/back-office).
3. The server is read-only by construction — it opens read-only database
   sessions and never writes to your feed.

## Quickstart

Install a [release binary](https://github.com/piotrsenkow/mlsgrid-mcp/releases)
or:

```sh
go install github.com/piotrsenkow/mlsgrid-mcp/cmd/mlsgrid-mcp@latest
```

Point it at the database `mlsgrid-sync` produced and verify the connection:

```sh
export MLSGRID_MCP_DATABASE_URL=postgres://user:pass@host:5432/mls?sslmode=disable
mlsgrid-mcp check
```

`check` asserts the schema-contract version and prints a data-freshness readout
(the same information the `get_data_freshness` tool returns). Running the binary
with no arguments serves the MCP protocol over stdio — that is how MCP clients
launch it.

### Use it from Claude Code

```sh
claude mcp add mlsgrid -- \
  env MLSGRID_MCP_DATABASE_URL=postgres://user:pass@host:5432/mls mlsgrid-mcp
```

### Use it from Claude Desktop

Add to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "mlsgrid": {
      "command": "mlsgrid-mcp",
      "env": {
        "MLSGRID_MCP_DATABASE_URL": "postgres://user:pass@host:5432/mls"
      }
    }
  }
}
```

For production, point the URL at a **read-only** database role — see
[docs/adapters.md](docs/adapters.md).

## Tools

Structured JSON out, whole-dollar money, a data-as-of timestamp on every
response. Full catalog in [docs/tools.md](docs/tools.md).

| Tool | Status | Purpose |
|---|---|---|
| `get_data_freshness` | **live** | Sync cursors, listing counts by status, media coverage, contract version — trust + liveness check |
| `search_listings` | planned | Area + status/type/price/beds/baths/sqft/year/DOM/keyword filters |
| `get_listing` | planned | Full detail by ListingKey or MLS number |
| `get_comps` | planned | Comparable sales: distance + similarity + suggested range |
| `price_history` | planned | Observed price/status timeline and total reduction |
| `market_stats` | planned | Median/avg price, $/sqft, DOM, sale-to-list, inventory, months-of-supply |
| `get_open_houses` | planned | Scheduled open houses by area and date range |
| `query_sql` | planned | Opt-in, read-only SQL escape hatch (off by default) |

## Extending it

The tools are written against one interface, `mls.Source`, with no knowledge of
where the data lives. Implement that interface over your own store — in a
separate, even private, repository — and reuse every tool via plain Go
composition. See [docs/adapters.md](docs/adapters.md) and
[docs/architecture.md](docs/architecture.md).

## License

[Apache-2.0](LICENSE). Not affiliated with or endorsed by MLS Grid LLC.
