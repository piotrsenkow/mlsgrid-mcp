package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// pgxQuoteIdent safely double-quotes a single SQL identifier (column name).
func pgxQuoteIdent(ident string) string {
	return pgx.Identifier{ident}.Sanitize()
}

// isNoRows reports whether err is pgx's no-rows sentinel.
func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// minTimestamp returns the minimum value of a timestamptz column. ok is false
// when the table is empty (min is NULL).
func (a *Adapter) minTimestamp(ctx context.Context, table, column string) (time.Time, bool, error) {
	return a.aggTimestamp(ctx, "min", table, column)
}

// maxTimestamp returns the maximum value of a timestamptz column. ok is false
// when the table is empty (max is NULL).
func (a *Adapter) maxTimestamp(ctx context.Context, table, column string) (time.Time, bool, error) {
	return a.aggTimestamp(ctx, "max", table, column)
}

func (a *Adapter) aggTimestamp(ctx context.Context, agg, table, column string) (time.Time, bool, error) {
	q := fmt.Sprintf(`SELECT %s(%s) FROM %s`, agg, pgxQuoteIdent(column), a.rel(table))
	var t *time.Time
	if err := a.pool.QueryRow(ctx, q).Scan(&t); err != nil {
		return time.Time{}, false, err
	}
	if t == nil {
		return time.Time{}, false, nil
	}
	return t.UTC(), true, nil
}
