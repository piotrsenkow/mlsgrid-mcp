package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const (
	// defaultSearchLimit is used when a query does not set Limit.
	defaultSearchLimit = 25
	// maxSearchLimit caps a page regardless of the requested Limit, bounding the
	// work a single tool call can ask the database to do.
	maxSearchLimit = 100
)

// SearchListings returns a page of listing summaries matching q, ordered newest
// first (modification_timestamp DESC, listing_key DESC) with keyset pagination.
func (a *Adapter) SearchListings(ctx context.Context, q mls.SearchQuery) (mls.Page[mls.ListingSummary], error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	var args argList
	where, err := a.buildSearchWhere(&args, q, true)
	if err != nil {
		return mls.Page[mls.ListingSummary]{}, err
	}

	// Fetch one extra row to detect whether a further page exists without a
	// separate count query.
	limitPlaceholder := args.add(limit + 1)
	sql := fmt.Sprintf(
		"SELECT %s FROM %s%s ORDER BY modification_timestamp DESC, listing_key DESC LIMIT %s",
		summaryColumns, a.rel("property"), where, limitPlaceholder)

	rows, err := a.pool.Query(ctx, sql, args.args...)
	if err != nil {
		return mls.Page[mls.ListingSummary]{}, fmt.Errorf("search: query: %w", err)
	}
	defer rows.Close()

	items := make([]mls.ListingSummary, 0, limit)
	for rows.Next() {
		s, err := scanSummary(rows)
		if err != nil {
			return mls.Page[mls.ListingSummary]{}, fmt.Errorf("search: scan: %w", err)
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return mls.Page[mls.ListingSummary]{}, fmt.Errorf("search: rows: %w", err)
	}

	var page mls.Page[mls.ListingSummary]
	if len(items) > limit {
		last := items[limit-1]
		page.NextCursor = searchCursor{ModTS: last.ModificationTS, Key: last.ListingKey}.encode()
		items = items[:limit]
	}
	page.Items = items

	// Total is the count across all pages for the same filters (excluding the
	// cursor). "How many match?" is the most common follow-up to a search, and
	// without it the caller is pushed toward raw SQL just to count. A filtered
	// COUNT over the property table is cheap for the typical scoped query.
	total, err := a.countListings(ctx, q)
	if err != nil {
		return mls.Page[mls.ListingSummary]{}, err
	}
	page.Total = total

	if newest, ok, err := a.maxTimestamp(ctx, "property", "modification_timestamp"); err == nil {
		page.DataAsOf = orZeroTime(newest, ok)
	}
	return page, nil
}

// countListings returns how many listings match the query's filters, ignoring
// pagination (cursor + limit).
func (a *Adapter) countListings(ctx context.Context, q mls.SearchQuery) (int64, error) {
	var args argList
	where, err := a.buildSearchWhere(&args, q, false)
	if err != nil {
		return 0, err
	}
	var total int64
	sql := "SELECT count(*) FROM " + a.rel("property") + where
	if err := a.pool.QueryRow(ctx, sql, args.args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("search: count: %w", err)
	}
	return total, nil
}

// buildSearchWhere appends the parameterized predicates for q to args and
// returns the WHERE clause (with a leading " WHERE ", or "" when unfiltered).
// When includeCursor is false the pagination predicate is omitted, which is what
// the count query wants (it counts all matches, not just those after the cursor).
func (a *Adapter) buildSearchWhere(args *argList, q mls.SearchQuery, includeCursor bool) (string, error) {
	var conds []string
	add := func(c string) { conds = append(conds, c) }

	// Area — at most one of City/PostalCode/County is expected, but applying
	// whichever are set simply ANDs them; State narrows further.
	if v := strings.TrimSpace(q.Area.City); v != "" {
		add("lower(city) = lower(" + args.add(v) + ")")
	}
	if v := strings.TrimSpace(q.Area.PostalCode); v != "" {
		add("postal_code = " + args.add(v))
	}
	if v := strings.TrimSpace(q.Area.County); v != "" {
		add("lower(county_or_parish) = lower(" + args.add(v) + ")")
	}
	if v := strings.TrimSpace(q.Area.State); v != "" {
		add("lower(state_or_province) = lower(" + args.add(v) + ")")
	}

	// Status and type values are matched case-insensitively: the stored values
	// are exact-cased RESO enums (e.g. "Active"), and a caller passing "active"
	// should not silently get zero rows.
	if vals := nonEmpty(q.Statuses); len(vals) > 0 {
		add("lower(standard_status) = ANY(" + args.add(lowerAll(vals)) + ")")
	}
	if vals := nonEmpty(q.PropertyTypes); len(vals) > 0 {
		// A caller's "type" may name either a PropertyType or a PropertySubType;
		// match against both so "Condominium" finds subtyped rows.
		p := args.add(lowerAll(vals))
		add("(lower(property_type) = ANY(" + p + ") OR lower(property_sub_type) = ANY(" + p + "))")
	}

	if q.MinPrice > 0 {
		add("list_price >= " + args.add(q.MinPrice))
	}
	if q.MaxPrice > 0 {
		add("list_price <= " + args.add(q.MaxPrice))
	}
	if q.MinBeds > 0 {
		add("bedrooms_total >= " + args.add(q.MinBeds))
	}
	if q.MinBathsFull > 0 {
		add("bathrooms_full >= " + args.add(q.MinBathsFull))
	}
	if q.MinLivingArea > 0 {
		add("living_area >= " + args.add(q.MinLivingArea))
	}
	if q.MaxLivingArea > 0 {
		add("living_area <= " + args.add(q.MaxLivingArea))
	}
	if q.MinYearBuilt > 0 {
		add("year_built >= " + args.add(q.MinYearBuilt))
	}
	if q.MaxDaysOnMarket > 0 {
		add("days_on_market <= " + args.add(q.MaxDaysOnMarket))
	}

	if kw := strings.TrimSpace(q.Keywords); kw != "" {
		// Best-effort free-text match over remarks + address. There is no
		// full-text index in the contract, so this is an ILIKE substring scan;
		// tool descriptions say as much.
		pat := args.add("%" + escapeLike(kw) + "%")
		add(fmt.Sprintf(`(coalesce(public_remarks,'') || ' ' ||
			coalesce(street_number,'') || ' ' || coalesce(street_name,'') || ' ' ||
			coalesce(city,'') || ' ' || coalesce(postal_code,'')) ILIKE %s`, pat))
	}

	if includeCursor && q.Cursor != "" {
		c, err := decodeCursor(q.Cursor)
		if err != nil {
			return "", err
		}
		// Row-value comparison expresses "strictly after the cursor" under the
		// (modification_timestamp DESC, listing_key DESC) order in one predicate.
		add(fmt.Sprintf("(modification_timestamp, listing_key) < (%s, %s)",
			args.add(c.ModTS), args.add(c.Key)))
	}

	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), nil
}

// argList accumulates positional query arguments and hands back their $N
// placeholders, keeping every user value parameterized.
type argList struct {
	args []any
}

func (a *argList) add(v any) string {
	a.args = append(a.args, v)
	return "$" + strconv.Itoa(len(a.args))
}

// nonEmpty returns the input with blank/whitespace-only entries dropped.
func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// lowerAll lowercases every element, for case-insensitive ANY() matching.
func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}

// escapeLike escapes LIKE/ILIKE metacharacters so a keyword is matched
// literally (backslash is Postgres's default LIKE escape character).
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
