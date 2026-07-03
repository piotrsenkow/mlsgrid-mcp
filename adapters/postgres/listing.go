package postgres

import (
	"context"
	"fmt"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// GetListing returns full detail for the listing named by ref. ListingKey wins
// when set; otherwise the human MLS number is used, optionally scoped to an
// originating system. A bare MLS number that matches multiple feeds returns
// mls.ErrAmbiguousRef; no match returns mls.ErrNotFound.
func (a *Adapter) GetListing(ctx context.Context, ref mls.ListingRef, opts mls.ListingOptions) (*mls.ListingDetail, error) {
	if ref.Empty() {
		return nil, fmt.Errorf("get_listing: %w", mls.ErrNotFound)
	}

	where, args := a.buildListingWhere(ref)
	// Fetch up to two rows: the second one, if present, proves the reference is
	// ambiguous (only possible for a system-less MLS number).
	sql := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s ORDER BY originating_system_name LIMIT 2",
		detailColumns, a.rel("property"), where)

	rows, err := a.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("get_listing: query: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("get_listing: rows: %w", err)
		}
		return nil, mls.ErrNotFound
	}
	detail, err := scanDetail(rows, opts.IncludeRaw)
	if err != nil {
		return nil, fmt.Errorf("get_listing: scan: %w", err)
	}
	if rows.Next() {
		return nil, mls.ErrAmbiguousRef
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get_listing: rows: %w", err)
	}
	return detail, nil
}

// buildListingWhere returns the parameterized WHERE body and args for ref.
// ListingKey is preferred; failing that, listing_id, optionally narrowed by
// originating_system_name.
func (a *Adapter) buildListingWhere(ref mls.ListingRef) (string, []any) {
	if ref.Key != "" {
		return "listing_key = $1", []any{ref.Key}
	}
	if ref.OriginatingSystem != "" {
		return "listing_id = $1 AND originating_system_name = $2",
			[]any{ref.MLSNumber, ref.OriginatingSystem}
	}
	return "listing_id = $1", []any{ref.MLSNumber}
}
