package postgres

import (
	"context"
	"fmt"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// Capabilities reports what this store can answer. It is computed on demand
// from cheap existence/lookup queries plus the cached contract version. Errors
// from the optional probes degrade to "unsupported" rather than failing the
// whole call, so a partially-populated store still yields a useful report.
func (a *Adapter) Capabilities(ctx context.Context) (mls.Capabilities, error) {
	caps := mls.Capabilities{
		SchemaContractVersion: a.contractVersion,
		// SQL reports that this adapter can back query_sql (it implements
		// SQLQuerier). Whether the tool is actually exposed is a separate,
		// operator-controlled decision made in server configuration.
		SQL: true,
	}

	systems, err := a.distinctText(ctx, "property", "originating_system_name")
	if err != nil {
		return caps, fmt.Errorf("capabilities: originating systems: %w", err)
	}
	caps.OriginatingSystems = systems

	statuses, err := a.distinctText(ctx, "property", "standard_status")
	if err != nil {
		return caps, fmt.Errorf("capabilities: statuses: %w", err)
	}
	caps.Statuses = statuses

	if geo, err := a.exists(ctx, fmt.Sprintf(
		`SELECT 1 FROM %s WHERE latitude IS NOT NULL AND longitude IS NOT NULL LIMIT 1`, a.rel("property"))); err == nil {
		caps.Geo = geo
	}

	if oh, err := a.exists(ctx, fmt.Sprintf(
		`SELECT 1 FROM %s LIMIT 1`, a.rel("open_house"))); err == nil {
		caps.OpenHouses = oh
	}

	// Price history exists when any change events have been captured; the
	// earliest observation bounds how far back history is trustworthy.
	if earliest, ok, err := a.minTimestamp(ctx, "listing_event", "observed_at"); err == nil && ok {
		caps.PriceHistory = true
		caps.HistorySince = earliest
	}

	return caps, nil
}

// distinctText returns the distinct non-null values of a text column, ordered.
func (a *Adapter) distinctText(ctx context.Context, table, column string) ([]string, error) {
	q := fmt.Sprintf(
		`SELECT DISTINCT %s FROM %s WHERE %s IS NOT NULL ORDER BY 1`,
		pgxQuoteIdent(column), a.rel(table), pgxQuoteIdent(column))
	rows, err := a.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// exists runs a `SELECT 1 ... LIMIT 1` and reports whether a row came back.
func (a *Adapter) exists(ctx context.Context, query string) (bool, error) {
	var one int
	err := a.pool.QueryRow(ctx, query).Scan(&one)
	if err != nil {
		if isNoRows(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
