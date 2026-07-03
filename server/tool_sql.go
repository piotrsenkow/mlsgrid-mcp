package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const querySQLDescription = `Run a single read-only SQL SELECT against the MLS database, for questions the curated tools can't express.

PREFER THE CURATED TOOLS. search_listings, get_listing, get_comps, market_stats, price_history, and get_open_houses return documented, well-shaped results and should answer almost everything. Reach for query_sql only for a one-off aggregation, join, or filter the other tools don't cover.

Rules enforced by the server: exactly one statement; it must be a SELECT (or WITH … SELECT); writes, DDL, multiple statements, and server-side file/IO functions are rejected. The query runs in a read-only transaction under a short statement timeout, and results are row-capped — check "truncated" and raise max_rows or add your own LIMIT/aggregation if it is set.

Table and column names follow the mlsgrid-sync schema contract, and unqualified names resolve to the data schema. Call describe_dataset first to get exact column names and valid, correctly-cased filter values — the schema is snake_case and values are case-sensitive, so guessing (e.g. 'active' instead of 'Active') silently returns nothing. This tool is only present when the operator has explicitly enabled it against a least-privilege read-only role.`

// querySQLInput is the query_sql request. Field names/tags are a public
// contract locked by the tools/list golden test.
type querySQLInput struct {
	Query   string `json:"query" jsonschema:"a single read-only SQL SELECT (or WITH … SELECT) statement"`
	MaxRows int    `json:"max_rows,omitempty" jsonschema:"maximum rows to return; the server applies a default and enforces a hard ceiling"`
}

// querySQLOutput is the tabular wire shape of query_sql.
type querySQLOutput struct {
	Columns   []string `json:"columns" jsonschema:"result column names, in order"`
	Rows      [][]any  `json:"rows" jsonschema:"result rows; each row is an array of values aligned to columns"`
	RowCount  int      `json:"row_count" jsonschema:"number of rows returned"`
	Truncated bool     `json:"truncated" jsonschema:"true when more rows matched than were returned — raise max_rows or narrow the query"`
	DataAsOf  string   `json:"data_as_of,omitempty" jsonschema:"how current the underlying data is (RFC3339 UTC)"`
}

// registerQuerySQL wires the opt-in query_sql escape hatch. It is only called
// when the operator has enabled SQL and the source implements mls.SQLQuerier.
func registerQuerySQL(srv *mcp.Server, q mls.SQLQuerier) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "query_sql",
		Description: querySQLDescription,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in querySQLInput) (*mcp.CallToolResult, querySQLOutput, error) {
		rs, err := q.QueryReadOnly(ctx, in.Query, in.MaxRows)
		if err != nil {
			return nil, querySQLOutput{}, err
		}
		return nil, toQuerySQLOutput(rs), nil
	})
}

func toQuerySQLOutput(rs *mls.ResultSet) querySQLOutput {
	out := querySQLOutput{
		Columns:   rs.Columns,
		Rows:      rs.Rows,
		RowCount:  len(rs.Rows),
		Truncated: rs.Truncated,
		DataAsOf:  formatTime(rs.DataAsOf),
	}
	// Prefer empty arrays over null for a stable, iterable wire shape.
	if out.Columns == nil {
		out.Columns = []string{}
	}
	if out.Rows == nil {
		out.Rows = [][]any{}
	}
	return out
}
