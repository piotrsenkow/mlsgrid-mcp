package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// PriceHistory returns a listing's observed change timeline from mlsgrid-sync's
// append-only listing_event capture, plus derived summary metrics. It is
// best-effort from first sync forward (see Capabilities.HistorySince): no events
// exist for changes that predate the backfill.
func (a *Adapter) PriceHistory(ctx context.Context, ref mls.ListingRef) (*mls.PriceHistory, error) {
	key, modTS, err := a.resolveListingKey(ctx, ref)
	if err != nil {
		return nil, err
	}

	q := fmt.Sprintf(`
		SELECT event_type, coalesce(old_value, ''), coalesce(new_value, ''), observed_at
		FROM %s WHERE listing_key = $1
		ORDER BY observed_at, id`, a.rel("listing_event"))
	rows, err := a.pool.Query(ctx, q, key)
	if err != nil {
		return nil, fmt.Errorf("price_history: query: %w", err)
	}
	defer rows.Close()

	var events []mls.PriceEvent
	for rows.Next() {
		var e mls.PriceEvent
		if err := rows.Scan(&e.EventType, &e.OldValue, &e.NewValue, &e.ObservedAt); err != nil {
			return nil, fmt.Errorf("price_history: scan: %w", err)
		}
		e.ObservedAt = e.ObservedAt.UTC()
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("price_history: rows: %w", err)
	}

	return &mls.PriceHistory{
		ListingKey:          key,
		Events:              events,
		TotalReductionPct:   totalReductionPct(events),
		DaysSinceLastChange: daysSinceLastChange(events),
		DataAsOf:            modTS,
	}, nil
}

// resolveListingKey resolves a ListingRef to exactly one listing_key and its
// modification timestamp, applying the same precedence and ambiguity rules as
// GetListing: ListingKey wins; a bare MLS number matching multiple feeds is
// mls.ErrAmbiguousRef; no match is mls.ErrNotFound.
func (a *Adapter) resolveListingKey(ctx context.Context, ref mls.ListingRef) (string, time.Time, error) {
	if ref.Empty() {
		return "", time.Time{}, mls.ErrNotFound
	}
	where, args := a.buildListingWhere(ref)
	q := fmt.Sprintf(
		"SELECT listing_key, modification_timestamp FROM %s WHERE %s ORDER BY originating_system_name LIMIT 2",
		a.rel("property"), where)
	rows, err := a.pool.Query(ctx, q, args...)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("resolve listing: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", time.Time{}, err
		}
		return "", time.Time{}, mls.ErrNotFound
	}
	var key string
	var modTS time.Time
	if err := rows.Scan(&key, &modTS); err != nil {
		return "", time.Time{}, err
	}
	if rows.Next() {
		return "", time.Time{}, mls.ErrAmbiguousRef
	}
	return key, modTS.UTC(), rows.Err()
}

// totalReductionPct is the net list-price change across captured price_change
// events, as a percent of the earliest observed price. Positive means the price
// came down (a reduction); negative means it rose. Zero when no price_change
// events were captured or the baseline is unusable.
func totalReductionPct(events []mls.PriceEvent) float64 {
	var baseline, latest float64
	haveBaseline := false
	for _, e := range events {
		if e.EventType != "price_change" {
			continue
		}
		if !haveBaseline {
			if v, ok := parseMoney(e.OldValue); ok {
				baseline = v
				haveBaseline = true
			}
		}
		if v, ok := parseMoney(e.NewValue); ok {
			latest = v
		}
	}
	if !haveBaseline || baseline == 0 || latest == 0 {
		return 0
	}
	return (baseline - latest) / baseline * 100
}

// daysSinceLastChange is whole days from the most recent event to now, or 0 when
// there are no events.
func daysSinceLastChange(events []mls.PriceEvent) int {
	if len(events) == 0 {
		return 0
	}
	last := events[len(events)-1].ObservedAt
	d := now().Sub(last).Hours() / 24
	if d < 0 {
		return 0
	}
	return int(d)
}

// parseMoney parses a price stored as event text, tolerating a leading currency
// symbol, thousands separators, and surrounding space. ok is false when the
// value does not parse to a number.
func parseMoney(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("$", "", ",", "").Replace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
