package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const (
	defaultOpenHouseLimit = 25
	maxOpenHouseLimit     = 100
	// defaultOpenHouseSpan is how far ahead of From the window reaches when the
	// caller leaves To unset — a week of upcoming open houses.
	defaultOpenHouseSpan = 7 * 24 * time.Hour
)

// OpenHouses returns scheduled open houses whose date falls in the query window,
// optionally scoped to an area, newest date first. The open_house resource
// carries no address of its own, so each row is left-joined to its property for
// locality fields (an open house whose listing is absent still returns its
// date/time/remarks, with empty address). The result is stamped with the newest
// open-house sync time so an agent can judge how current the schedule is.
func (a *Adapter) OpenHouses(ctx context.Context, q mls.OpenHouseQuery) (mls.OpenHouseResult, error) {
	from, to := openHouseWindow(q.From, q.To)
	limit := q.Limit
	if limit <= 0 {
		limit = defaultOpenHouseLimit
	}
	if limit > maxOpenHouseLimit {
		limit = maxOpenHouseLimit
	}

	var args argList
	conds := []string{
		"oh.open_house_date >= " + args.add(from.Format("2006-01-02")) + "::date",
		"oh.open_house_date <= " + args.add(to.Format("2006-01-02")) + "::date",
	}
	// Area predicates reference the joined property (aliased p); the open_house
	// row itself has no locality columns.
	if v := strings.TrimSpace(q.Area.City); v != "" {
		conds = append(conds, "lower(p.city) = lower("+args.add(v)+")")
	}
	if v := strings.TrimSpace(q.Area.PostalCode); v != "" {
		conds = append(conds, "p.postal_code = "+args.add(v))
	}
	if v := strings.TrimSpace(q.Area.County); v != "" {
		conds = append(conds, "lower(p.county_or_parish) = lower("+args.add(v)+")")
	}
	if v := strings.TrimSpace(q.Area.State); v != "" {
		conds = append(conds, "lower(p.state_or_province) = lower("+args.add(v)+")")
	}

	sql := fmt.Sprintf(`SELECT
		coalesce(oh.listing_key, ''), coalesce(oh.listing_id, ''),
		coalesce(p.street_number, ''), coalesce(p.street_dir_prefix, ''), coalesce(p.street_name, ''),
		coalesce(p.street_suffix, ''), coalesce(p.unit_number, ''),
		coalesce(p.city, ''), coalesce(p.postal_code, ''), coalesce(p.county_or_parish, ''), coalesce(p.state_or_province, ''),
		oh.open_house_date, oh.start_time, oh.end_time, coalesce(oh.remarks, '')
		FROM %s oh
		LEFT JOIN %s p ON p.listing_key = oh.listing_key
		WHERE %s
		ORDER BY oh.open_house_date, oh.start_time NULLS LAST, oh.open_house_key
		LIMIT %s`,
		a.rel("open_house"), a.rel("property"), strings.Join(conds, " AND "), args.add(limit))

	rows, err := a.pool.Query(ctx, sql, args.args...)
	if err != nil {
		return mls.OpenHouseResult{}, fmt.Errorf("open houses: query: %w", err)
	}
	defer rows.Close()

	out := make([]mls.OpenHouse, 0, limit)
	for rows.Next() {
		var oh mls.OpenHouse
		var streetNumber, streetDir, streetName, streetSuffix, unit string
		var city, postal, county, state string
		var date time.Time
		var start, end *time.Time
		if err := rows.Scan(
			&oh.ListingKey, &oh.MLSNumber,
			&streetNumber, &streetDir, &streetName, &streetSuffix, &unit,
			&city, &postal, &county, &state,
			&date, &start, &end, &oh.Remarks,
		); err != nil {
			return mls.OpenHouseResult{}, fmt.Errorf("open houses: scan: %w", err)
		}
		oh.Date = date.UTC()
		oh.StartTime = orZeroTime(derefTime(start))
		oh.EndTime = orZeroTime(derefTime(end))
		oh.Address = mls.Address{
			StreetNumber: streetNumber,
			StreetName:   joinNonEmpty(" ", streetDir, streetName, streetSuffix),
			UnitNumber:   unit,
			City:         city,
			State:        state,
			PostalCode:   postal,
			County:       county,
		}
		out = append(out, oh)
	}
	if err := rows.Err(); err != nil {
		return mls.OpenHouseResult{}, fmt.Errorf("open houses: rows: %w", err)
	}

	res := mls.OpenHouseResult{OpenHouses: out}
	if newest, ok, err := a.maxTimestamp(ctx, "open_house", "modification_timestamp"); err == nil {
		res.DataAsOf = orZeroTime(newest, ok)
	}
	return res, nil
}

// openHouseWindow applies the defaults: an unset From starts now, an unset To
// reaches a week past From.
func openHouseWindow(from, to time.Time) (time.Time, time.Time) {
	if from.IsZero() {
		from = now()
	}
	if to.IsZero() {
		to = from.Add(defaultOpenHouseSpan)
	}
	return from, to
}

// derefTime returns (*t, t != nil) so a nullable timestamp scans through
// orZeroTime like the aggregate helpers elsewhere in the adapter.
func derefTime(t *time.Time) (time.Time, bool) {
	if t == nil {
		return time.Time{}, false
	}
	return *t, true
}
