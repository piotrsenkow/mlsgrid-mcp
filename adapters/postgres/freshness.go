package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// Freshness reports replication state, corpus size, status/media breakdowns,
// and the live contract version — the data behind the get_data_freshness tool.
func (a *Adapter) Freshness(ctx context.Context) (mls.Freshness, error) {
	f := mls.Freshness{
		SchemaContractVersion: a.contractVersion,
		GeneratedAt:           now(),
	}

	cursors, err := a.cursors(ctx)
	if err != nil {
		return f, fmt.Errorf("freshness: cursors: %w", err)
	}
	f.Cursors = cursors

	statusCounts, total, err := a.groupCounts(ctx, "property", "standard_status")
	if err != nil {
		return f, fmt.Errorf("freshness: status counts: %w", err)
	}
	f.ListingStatusCounts = statusCounts
	f.TotalListings = total

	mediaCounts, _, err := a.groupCounts(ctx, "media", "storage_status")
	if err != nil {
		return f, fmt.Errorf("freshness: media counts: %w", err)
	}
	f.MediaCounts = mediaCounts

	if newest, ok, err := a.maxTimestamp(ctx, "property", "modification_timestamp"); err == nil && ok {
		f.DataAsOf = newest
	}

	return f, nil
}

// cursors reads sync_state and pairs each row with a stored-row count for its
// resource.
func (a *Adapter) cursors(ctx context.Context) ([]mls.ResourceCursor, error) {
	q := fmt.Sprintf(`
		SELECT resource, originating_system, last_modification_ts,
		       backfill_completed_at IS NOT NULL AS backfilled,
		       last_full_reconcile_at
		FROM %s
		ORDER BY resource, originating_system`, a.rel("sync_state"))
	rows, err := a.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []mls.ResourceCursor
	for rows.Next() {
		var c mls.ResourceCursor
		var watermark, reconcile *time.Time
		if err := rows.Scan(&c.Resource, &c.OriginatingSystem, &watermark, &c.BackfillComplete, &reconcile); err != nil {
			return nil, err
		}
		c.Watermark = watermark
		c.LastReconcile = reconcile
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Attach stored-row counts per resource. Counts are cheap relative to the
	// tool's cadence and keep the report self-contained.
	for i := range out {
		n, err := a.resourceCount(ctx, out[i].Resource, out[i].OriginatingSystem)
		if err != nil {
			return nil, err
		}
		out[i].StoredRows = n
	}
	return out, nil
}

// resourceCount returns the number of stored rows for a synced resource.
func (a *Adapter) resourceCount(ctx context.Context, resource, originatingSystem string) (int64, error) {
	switch resource {
	case "Property":
		var n int64
		q := fmt.Sprintf(`SELECT count(*) FROM %s WHERE originating_system_name = $1`, a.rel("property"))
		return n, a.pool.QueryRow(ctx, q, originatingSystem).Scan(&n)
	case "OpenHouse":
		var n int64
		q := fmt.Sprintf(`SELECT count(*) FROM %s`, a.rel("open_house"))
		return n, a.pool.QueryRow(ctx, q).Scan(&n)
	default:
		return 0, nil
	}
}

// groupCounts returns per-value row counts for a column, most populous first,
// along with the grand total. NULL values are labeled "(unknown)".
func (a *Adapter) groupCounts(ctx context.Context, table, column string) ([]mls.StatusCount, int64, error) {
	q := fmt.Sprintf(
		`SELECT coalesce(%s, '(unknown)') AS label, count(*) AS n
		 FROM %s GROUP BY 1 ORDER BY n DESC, label`,
		pgxQuoteIdent(column), a.rel(table))
	rows, err := a.pool.Query(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []mls.StatusCount
	var total int64
	for rows.Next() {
		var sc mls.StatusCount
		if err := rows.Scan(&sc.Status, &sc.Count); err != nil {
			return nil, 0, err
		}
		total += sc.Count
		out = append(out, sc)
	}
	return out, total, rows.Err()
}
