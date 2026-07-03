package postgres

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// defaultStatsPeriod is the trailing window market_stats aggregates over when
// the caller does not set one — a quarter, the usual "current market" horizon.
const defaultStatsPeriod = 90 * 24 * time.Hour

// MarketStats aggregates an area's market over a trailing window. Medians and
// quartiles are computed server-side with percentile_cont so a large closed-sale
// set never has to be shipped to the client. Inventory metrics (median list
// price, active count, months of supply) describe the current for-sale snapshot;
// the rest summarize sales that closed within the period. When CompareToPrior is
// set the same closed-sale aggregate is run over the immediately preceding
// window and attached as Prior (inventory is a point-in-time figure and is not
// reconstructed for the past window).
func (a *Adapter) MarketStats(ctx context.Context, q mls.StatsQuery) (*mls.MarketStats, error) {
	period := q.Period
	if period <= 0 {
		period = defaultStatsPeriod
	}
	periodDays := int(math.Round(period.Hours() / 24))
	end := now()
	curStart := end.Add(-period)

	closed, err := a.closedStats(ctx, q.Area, q.PropertyTypes, curStart, end, true)
	if err != nil {
		return nil, fmt.Errorf("market stats: closed sales: %w", err)
	}
	active, err := a.activeStats(ctx, q.Area, q.PropertyTypes)
	if err != nil {
		return nil, fmt.Errorf("market stats: active inventory: %w", err)
	}

	ms := &mls.MarketStats{
		Area:                         q.Area,
		PeriodDays:                   periodDays,
		MedianListPrice:              active.medianList,
		ActiveInventory:              active.count,
		MonthsOfSupply:               monthsOfSupply(active.count, closed.count, periodDays),
		MedianClosePrice:             closed.medianClose,
		AvgClosePrice:                closed.avgClose,
		MedianPPSF:                   closed.medianPPSF,
		MedianDaysOnMarket:           closed.medianDOM,
		MedianCumulativeDaysOnMarket: closed.medianCDOM,
		SaleToListRatio:              round4(closed.saleToList),
		SaleToOriginalRatio:          round4(closed.saleToOrig),
		ClosedInPeriod:               closed.count,
	}

	if q.CompareToPrior {
		priorStart := end.Add(-2 * period)
		// Half-open on the upper bound (< curStart) so a sale on the boundary is
		// counted in exactly one window.
		pc, err := a.closedStats(ctx, q.Area, q.PropertyTypes, priorStart, curStart, false)
		if err != nil {
			return nil, fmt.Errorf("market stats: prior period: %w", err)
		}
		ms.Prior = &mls.MarketStats{
			PeriodDays:                   periodDays,
			MedianClosePrice:             pc.medianClose,
			AvgClosePrice:                pc.avgClose,
			MedianPPSF:                   pc.medianPPSF,
			MedianDaysOnMarket:           pc.medianDOM,
			MedianCumulativeDaysOnMarket: pc.medianCDOM,
			SaleToListRatio:              round4(pc.saleToList),
			SaleToOriginalRatio:          round4(pc.saleToOrig),
			ClosedInPeriod:               pc.count,
		}
	}

	if newest, ok, err := a.maxTimestamp(ctx, "property", "modification_timestamp"); err == nil {
		ms.DataAsOf = orZeroTime(newest, ok)
	}
	return ms, nil
}

// closedAgg holds the closed-sale aggregates for one window.
type closedAgg struct {
	count       int64
	medianClose int64
	avgClose    int64
	medianPPSF  int64
	medianDOM   int
	medianCDOM  int
	saleToList  float64
	saleToOrig  float64
}

// closedStats aggregates sales that closed in [start, end] (end inclusive) or
// [start, end) (end exclusive) for the area/type scope. Medians ignore NULLs
// (percentile_cont does), and per-row ratios guard division with nullif, so a
// zero size or list price simply drops out rather than skewing the result.
func (a *Adapter) closedStats(ctx context.Context, area mls.Area, types []string, start, end time.Time, endInclusive bool) (closedAgg, error) {
	var args argList
	conds := []string{
		"standard_status = 'Closed'",
		"close_price IS NOT NULL AND close_price > 0",
		"close_date >= " + args.add(start.Format("2006-01-02")) + "::date",
	}
	endOp := "<"
	if endInclusive {
		endOp = "<="
	}
	conds = append(conds, "close_date "+endOp+" "+args.add(end.Format("2006-01-02"))+"::date")
	conds = append(conds, areaTypeConds(&args, area, types)...)

	sql := fmt.Sprintf(`SELECT
		count(*),
		coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY close_price::double precision), 0)::bigint,
		coalesce(avg(close_price), 0)::bigint,
		coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY (close_price / nullif(living_area, 0))::double precision), 0)::bigint,
		coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY days_on_market::double precision), 0)::int,
		coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY cumulative_days_on_market::double precision), 0)::int,
		coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY (close_price / nullif(list_price, 0))::double precision), 0),
		coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY (close_price / nullif(original_list_price, 0))::double precision), 0)
		FROM %s WHERE %s`, a.rel("property"), strings.Join(conds, " AND "))

	var c closedAgg
	if err := a.pool.QueryRow(ctx, sql, args.args...).Scan(
		&c.count, &c.medianClose, &c.avgClose, &c.medianPPSF,
		&c.medianDOM, &c.medianCDOM, &c.saleToList, &c.saleToOrig,
	); err != nil {
		return closedAgg{}, err
	}
	return c, nil
}

// activeStats holds current for-sale inventory metrics for the scope.
type activeAgg struct {
	count      int64
	medianList int64
}

// activeStats counts active listings and their median list price — the current
// for-sale snapshot behind inventory and months-of-supply.
func (a *Adapter) activeStats(ctx context.Context, area mls.Area, types []string) (activeAgg, error) {
	var args argList
	conds := append([]string{"standard_status = 'Active'"}, areaTypeConds(&args, area, types)...)
	sql := fmt.Sprintf(`SELECT
		count(*),
		coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY list_price::double precision), 0)::bigint
		FROM %s WHERE %s`, a.rel("property"), strings.Join(conds, " AND "))

	var ag activeAgg
	if err := a.pool.QueryRow(ctx, sql, args.args...).Scan(&ag.count, &ag.medianList); err != nil {
		return activeAgg{}, err
	}
	return ag, nil
}

// areaTypeConds builds the parameterized area + property-type predicates shared
// by the closed and active aggregates. Area matching mirrors search_listings
// (case-insensitive city/county/state, exact postal); a property type matches
// either PropertyType or PropertySubType.
func areaTypeConds(args *argList, area mls.Area, types []string) []string {
	var conds []string
	if v := strings.TrimSpace(area.City); v != "" {
		conds = append(conds, "lower(city) = lower("+args.add(v)+")")
	}
	if v := strings.TrimSpace(area.PostalCode); v != "" {
		conds = append(conds, "postal_code = "+args.add(v))
	}
	if v := strings.TrimSpace(area.County); v != "" {
		conds = append(conds, "lower(county_or_parish) = lower("+args.add(v)+")")
	}
	if v := strings.TrimSpace(area.State); v != "" {
		conds = append(conds, "lower(state_or_province) = lower("+args.add(v)+")")
	}
	if vals := nonEmpty(types); len(vals) > 0 {
		p := args.add(vals)
		conds = append(conds, "(property_type = ANY("+p+") OR property_sub_type = ANY("+p+"))")
	}
	return conds
}

// monthsOfSupply is the standard absorption metric: active inventory divided by
// the monthly closed-sale rate over the period. It is zero when nothing closed
// (an undefined rate) so the figure never blows up on a thin market.
func monthsOfSupply(active, closed int64, periodDays int) float64 {
	if closed <= 0 || periodDays <= 0 {
		return 0
	}
	months := float64(periodDays) / 30.0
	perMonth := float64(closed) / months
	if perMonth <= 0 {
		return 0
	}
	return round2(float64(active) / perMonth)
}

// round2 / round4 keep derived ratios to a sane number of decimals so the wire
// output is stable and readable rather than carrying float noise.
func round2(f float64) float64 { return math.Round(f*100) / 100 }
func round4(f float64) float64 { return math.Round(f*10000) / 10000 }
