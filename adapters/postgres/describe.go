package postgres

import (
	"context"
	"fmt"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// enumColumns are the low-cardinality columns describe_dataset enumerates. The
// column names are part of the pinned schema contract (stable within a major
// version); their *values* are read live, so the description never drifts from
// the data. High-cardinality columns (cities, keys, prices) are excluded.
var enumColumns = []struct{ table, column string }{
	{"property", "standard_status"},
	{"property", "mls_status"},
	{"property", "property_type"},
	{"property", "property_sub_type"},
	{"property", "association_fee_frequency"},
	{"listing_event", "event_type"},
	{"media", "storage_status"},
}

// maxEnumValues caps how many distinct values a single column reports, so a
// column that turns out higher-cardinality than expected can't dump a huge list.
const maxEnumValues = 60

// describeSkipTables are internal bookkeeping tables not useful to query.
var describeSkipTables = []string{"schema_meta", "schema_migrations", "sync_state", "rate_budget"}

// DescribeDataset returns a live description of the queryable schema: every
// table's columns (name, type, nullability) plus the observed distinct values of
// the enumerated columns. It powers describe_dataset, which agents call to learn
// exact column names and valid, correctly-cased filter values.
func (a *Adapter) DescribeDataset(ctx context.Context) (*mls.DatasetDescription, error) {
	tables, err := a.describeTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("describe dataset: columns: %w", err)
	}

	enums := make([]mls.EnumDescription, 0, len(enumColumns))
	for _, ec := range enumColumns {
		vals, err := a.enumValues(ctx, ec.table, ec.column)
		if err != nil {
			// A column absent in some future schema shouldn't fail the whole
			// description; just skip it.
			continue
		}
		if len(vals) > 0 {
			enums = append(enums, mls.EnumDescription{Table: ec.table, Column: ec.column, Values: vals})
		}
	}

	desc := &mls.DatasetDescription{Tables: tables, Enums: enums}
	if newest, ok, err := a.maxTimestamp(ctx, "property", "modification_timestamp"); err == nil {
		desc.DataAsOf = orZeroTime(newest, ok)
	}
	return desc, nil
}

// describeTables reads the schema's base tables and their columns from the
// catalog, skipping the internal bookkeeping tables.
func (a *Adapter) describeTables(ctx context.Context) ([]mls.TableDescription, error) {
	const q = `
		SELECT table_name, column_name, data_type, (is_nullable = 'YES')
		FROM information_schema.columns
		WHERE table_schema = $1 AND NOT (table_name = ANY($2))
		ORDER BY table_name, ordinal_position`
	rows, err := a.pool.Query(ctx, q, a.schema, describeSkipTables)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []mls.TableDescription
	idx := map[string]int{}
	for rows.Next() {
		var table, column, typ string
		var nullable bool
		if err := rows.Scan(&table, &column, &typ, &nullable); err != nil {
			return nil, err
		}
		i, ok := idx[table]
		if !ok {
			i = len(tables)
			idx[table] = i
			tables = append(tables, mls.TableDescription{Name: table})
		}
		tables[i].Columns = append(tables[i].Columns, mls.ColumnDescription{Name: column, Type: typ, Nullable: nullable})
	}
	return tables, rows.Err()
}

// enumValues returns a column's distinct values, most frequent first, with NULL
// surfaced as "(null)" so a mostly-empty column is obvious. Capped at
// maxEnumValues.
func (a *Adapter) enumValues(ctx context.Context, table, column string) ([]mls.EnumValue, error) {
	q := fmt.Sprintf(
		`SELECT coalesce(%s::text, '(null)') AS v, count(*) AS n FROM %s GROUP BY 1 ORDER BY n DESC, v LIMIT %d`,
		pgxQuoteIdent(column), a.rel(table), maxEnumValues)
	rows, err := a.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []mls.EnumValue
	for rows.Next() {
		var v string
		var n int64
		if err := rows.Scan(&v, &n); err != nil {
			return nil, err
		}
		out = append(out, mls.EnumValue{Value: v, Count: n})
	}
	return out, rows.Err()
}
