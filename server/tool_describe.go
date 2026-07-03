package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const describeDescription = `Describe the queryable dataset: its tables, their columns, and the real values of key categorical fields.

Call this BEFORE writing query_sql, or whenever you need exact column names or valid filter values — the schema uses snake_case columns and case-sensitive values that are easy to guess wrong (a wrong guess silently returns zero rows). It returns each table's columns (name + SQL type + nullability) plus, for low-cardinality columns (status, type, event kind, …), the distinct values actually present, most common first, with counts. A "(null)" entry shows how often a column is empty — watch for fields that are mostly null.

Important: there are two status columns — standard_status (RESO-canonical; what search_listings and market_stats filter on) and mls_status (the MLS's own, more granular) — and they can spell the same concept differently (e.g. "Canceled" vs "Cancelled"). In raw SQL, use exactly the values this returns. Takes no arguments.`

// describeInput is empty; describe_dataset is a nullary tool.
type describeInput struct{}

// describeOutput is the wire shape of describe_dataset. Its schema is locked by
// the tools/list golden test.
type describeOutput struct {
	Tables   []tableOut `json:"tables" jsonschema:"queryable tables and their columns"`
	Enums    []enumOut  `json:"enums" jsonschema:"observed distinct values of low-cardinality columns, most frequent first"`
	DataAsOf string     `json:"data_as_of,omitempty" jsonschema:"how current the underlying data is (RFC3339 UTC)"`
}

type tableOut struct {
	Name    string      `json:"name" jsonschema:"table name (query it by this name; unqualified names resolve to the data schema)"`
	Columns []columnOut `json:"columns" jsonschema:"columns in schema order"`
}

type columnOut struct {
	Name     string `json:"name"`
	Type     string `json:"type" jsonschema:"SQL data type"`
	Nullable bool   `json:"nullable"`
}

type enumOut struct {
	Table  string         `json:"table"`
	Column string         `json:"column"`
	Values []enumValueOut `json:"values" jsonschema:"observed values (exact case), most frequent first; \"(null)\" is the empty bucket"`
}

type enumValueOut struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// registerDescribe wires describe_dataset. It is only called when the source
// implements mls.DatasetDescriber.
func registerDescribe(srv *mcp.Server, d mls.DatasetDescriber) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "describe_dataset",
		Description: describeDescription,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ describeInput) (*mcp.CallToolResult, describeOutput, error) {
		desc, err := d.DescribeDataset(ctx)
		if err != nil {
			return nil, describeOutput{}, err
		}
		return nil, toDescribeOutput(desc), nil
	})
}

func toDescribeOutput(d *mls.DatasetDescription) describeOutput {
	out := describeOutput{DataAsOf: formatTime(d.DataAsOf)}
	out.Tables = make([]tableOut, 0, len(d.Tables))
	for _, t := range d.Tables {
		to := tableOut{Name: t.Name}
		to.Columns = make([]columnOut, 0, len(t.Columns))
		for _, c := range t.Columns {
			to.Columns = append(to.Columns, columnOut{Name: c.Name, Type: c.Type, Nullable: c.Nullable})
		}
		out.Tables = append(out.Tables, to)
	}
	out.Enums = make([]enumOut, 0, len(d.Enums))
	for _, e := range d.Enums {
		eo := enumOut{Table: e.Table, Column: e.Column}
		eo.Values = make([]enumValueOut, 0, len(e.Values))
		for _, v := range e.Values {
			eo.Values = append(eo.Values, enumValueOut{Value: v.Value, Count: v.Count})
		}
		out.Enums = append(out.Enums, eo)
	}
	return out
}
