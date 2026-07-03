# Adapters

mlsgrid-mcp's tools are written against one interface, `mls.Source`, and know
nothing about where the data lives. An **adapter** is any type that implements
`mls.Source`; `server.New(source)` turns it into a running MCP server. This is
the whole extensibility story — no plugins, just Go composition.

## The interface

`mls.Source` is *capability-shaped*, not table-shaped: each method answers a
question a real-estate agent would ask (search, detail, comps, market stats,
price history, open houses, freshness), plus `Capabilities` so tools can degrade
gracefully when a source lacks a feature (e.g. no coordinates → no distance in
comps). See [`mls/source.go`](../mls/source.go) for the full contract.

A source may *also* implement two optional capabilities, each of which lights up
its tool automatically when present:

- `mls.DatasetDescriber` → **describe_dataset**, a live view of the queryable
  schema (columns + the real distinct values of categorical fields) so agents
  filter with exact values instead of guessing.
- `mls.SQLQuerier` → the opt-in **query_sql** escape hatch. This one is
  additionally gated on server configuration (off unless explicitly enabled).

## The default adapter

[`adapters/postgres`](../adapters/postgres) targets the schema
[mlsgrid-sync](https://github.com/piotrsenkow/mlsgrid-sync) produces. It:

- opens a pgx/v5 pool with `default_transaction_read_only=on` (this server never
  writes);
- reads the live schema-contract version from `schema_meta` at startup and
  refuses to run on a **major** mismatch (`ExpectedContractMajor`);
- treats the schema name as configuration (validated + quoted, never hardcoded).

## Writing a private out-of-tree adapter

Because `mls` has no database or SDK dependencies, you can implement `Source`
over a proprietary store in a separate (even private) repository and reuse every
tool. The pattern is plain composition:

```go
package main

import (
    "context"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    "github.com/piotrsenkow/mlsgrid-mcp/mls"
    "github.com/piotrsenkow/mlsgrid-mcp/server"
)

// myAdapter implements mls.Source over your own data store.
type myAdapter struct { /* your handles */ }

func (a *myAdapter) Freshness(ctx context.Context) (mls.Freshness, error) { /* ... */ }
// ...implement the rest of mls.Source, returning mls.ErrNotImplemented for
// anything you don't support yet (and reflecting it in Capabilities).

func main() {
    ctx := context.Background()
    src := &myAdapter{ /* ... */ }
    srv, err := server.New(src, server.WithInfo("my-mls-mcp", "v0.1.0"))
    if err != nil {
        panic(err)
    }
    if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
        panic(err)
    }
}
```

Return `mls.ErrNotImplemented` from any method you haven't built yet — the
server registers only the tools it can serve, and `Capabilities` lets the ones
it does register set honest expectations.
