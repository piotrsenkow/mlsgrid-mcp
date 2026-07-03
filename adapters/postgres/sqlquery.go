package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/piotrsenkow/mlsgrid-mcp/internal/sqlguard"
	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const (
	// defaultSQLMaxRows / maxSQLMaxRows bound the rows a single query_sql call
	// returns. The default is conservative; the ceiling is a hard cap even when a
	// caller (or the operator) asks for more.
	defaultSQLMaxRows = 1000
	maxSQLMaxRows     = 10000
	// defaultSQLTimeout bounds each query_sql execution so a runaway query cannot
	// tie up the database it points at (which may be shared).
	defaultSQLTimeout = 5 * time.Second
)

// initSQL resolves the query_sql limits from options and records the connection
// role, which SQLSafe consults before the escape hatch is exposed.
func (a *Adapter) initSQL(ctx context.Context, opts Options) error {
	a.sqlMaxRows = opts.SQLMaxRows
	if a.sqlMaxRows <= 0 {
		a.sqlMaxRows = defaultSQLMaxRows
	}
	if a.sqlMaxRows > maxSQLMaxRows {
		a.sqlMaxRows = maxSQLMaxRows
	}
	a.sqlTimeout = opts.SQLTimeout
	if a.sqlTimeout <= 0 {
		a.sqlTimeout = defaultSQLTimeout
	}
	return a.pool.QueryRow(ctx,
		`SELECT current_user, current_setting('is_superuser')::bool`,
	).Scan(&a.currentUser, &a.isSuperuser)
}

// SQLSafe reports whether it is safe to expose the query_sql escape hatch over
// this connection, returning a descriptive error when it is not. A superuser
// connection can reach server-side reads and program execution that the
// read-only transaction alone does not fence off, so the caller should refuse
// to expose query_sql (while still serving the curated tools) in that case.
func (a *Adapter) SQLSafe() error {
	if a.isSuperuser {
		return fmt.Errorf("connection role %q is a Postgres superuser; query_sql requires a least-privilege read-only role (see the query_sql section of the README)", a.currentUser)
	}
	return nil
}

// QueryReadOnly runs a single validated read-only statement and returns at most
// maxRows rows. Enforcement is layered: sqlguard rejects anything that is not a
// lone SELECT/WITH before it reaches the database; the statement then runs in a
// read-only transaction under a statement timeout, with search_path pinned to
// the contract schema so unqualified table names resolve there. The statement is
// wrapped in an outer LIMIT so even a query without one cannot stream an
// unbounded result.
func (a *Adapter) QueryReadOnly(ctx context.Context, query string, maxRows int) (*mls.ResultSet, error) {
	clean, err := sqlguard.Validate(query)
	if err != nil {
		return nil, fmt.Errorf("query_sql: %w", err)
	}
	if maxRows <= 0 {
		maxRows = a.sqlMaxRows
	}
	if maxRows > maxSQLMaxRows {
		maxRows = maxSQLMaxRows
	}

	// A dedicated connection keeps the SET LOCAL settings and the read-only
	// transaction scoped to this query and off any pooled sibling.
	conn, err := a.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("query_sql: acquire connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("query_sql: begin: %w", err)
	}
	// The transaction only ever reads; always roll back.
	defer func() { _ = tx.Rollback(ctx) }()

	// statement_timeout and search_path take integer/identifier literals, not bind
	// parameters; both values are ours (config + validated schema), never caller
	// input, so interpolation is safe.
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", a.sqlTimeout.Milliseconds())); err != nil {
		return nil, fmt.Errorf("query_sql: set timeout: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+pgxQuoteIdent(a.schema)); err != nil {
		return nil, fmt.Errorf("query_sql: set search_path: %w", err)
	}

	// Fetch one extra row so an exact-fit result is distinguishable from a
	// truncated one.
	wrapped := fmt.Sprintf("SELECT * FROM (%s) AS _mlsgrid_guard LIMIT %d", clean, maxRows+1)
	rows, err := tx.Query(ctx, wrapped)
	if err != nil {
		return nil, fmt.Errorf("query_sql: execute: %w", err)
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	cols := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = f.Name
	}

	out := make([][]any, 0, maxRows)
	truncated := false
	for rows.Next() {
		if len(out) >= maxRows {
			truncated = true
			break
		}
		vals, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("query_sql: read row: %w", err)
		}
		normalized := make([]any, len(vals))
		for i, v := range vals {
			normalized[i] = normalizeValue(v)
		}
		out = append(out, normalized)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query_sql: rows: %w", err)
	}
	rows.Close()

	res := &mls.ResultSet{Columns: cols, Rows: out, Truncated: truncated}
	// Stamp with the same "data as of" signal the other tools use, so an agent
	// can reason about staleness of whatever the ad-hoc query returned.
	if newest, ok, err := a.maxTimestamp(ctx, "property", "modification_timestamp"); err == nil {
		res.DataAsOf = orZeroTime(newest, ok)
	}
	return res, nil
}

// normalizeValue coerces the driver's Go value into something that JSON-encodes
// cleanly for a tabular result. Timestamps become RFC3339 UTC strings, byte
// slices become text, and numerics render through their string form; everything
// else passes through as pgx decoded it.
func normalizeValue(v any) any {
	switch t := v.(type) {
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	case []byte:
		return string(t)
	case pgtype.Numeric:
		if dv, err := t.Value(); err == nil {
			return dv
		}
		return nil
	default:
		return v
	}
}
