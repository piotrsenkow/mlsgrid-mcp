package server

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const statsDescription = `Aggregate an area's market over a trailing window: pricing, pace, and supply.

Requires an area (city / postal_code / county / state — it will not aggregate the whole feed). Optionally narrow by property_types and set period_days (default 90). Money is in whole US dollars.

Returns two groups of numbers. Current for-sale snapshot: median_list_price, active_inventory, and months_of_supply (active inventory ÷ the monthly closed-sale rate — under ~6 favors sellers, over ~6 favors buyers). Sales closed in the period: median/avg close price, median_ppsf, median days on market (both median_days_on_market and median_cumulative_days_on_market, which counts relists and exposes DOM-reset gaming), and sale-to-list reported two ways — sale_to_list_ratio (vs the final list price) and sale_to_original_ratio (vs the original ask, showing total markdown). Set compare_to_prior to also get the immediately preceding window as prior (closed-sale metrics only; inventory is a current snapshot and is not reconstructed for the past).

Personas: agents gauge market temperature for a pricing conversation; investors read months_of_supply and sale-to-list for timing and negotiation room; buyers/sellers learn whether it is a buyer's or seller's market. Honest limit: these are medians over whatever closed in the window, so a thin area or short period yields small samples — widen period_days when counts are low.`

// statsInput scopes a market_stats aggregation. Field names/tags are a public
// contract locked by the tools/list golden test.
type statsInput struct {
	City           string   `json:"city,omitempty" jsonschema:"area: city (required unless another area field is set); case-insensitive exact match"`
	PostalCode     string   `json:"postal_code,omitempty" jsonschema:"area: postal code"`
	County         string   `json:"county,omitempty" jsonschema:"area: county or parish"`
	State          string   `json:"state,omitempty" jsonschema:"area: two-letter state or province"`
	PropertyTypes  []string `json:"property_types,omitempty" jsonschema:"limit to these PropertyType or PropertySubType values; empty means all types"`
	PeriodDays     int      `json:"period_days,omitempty" jsonschema:"trailing window in days for closed-sale metrics (default 90)"`
	CompareToPrior bool     `json:"compare_to_prior,omitempty" jsonschema:"also aggregate the immediately preceding window and return it as prior"`
}

// priorStatsOut is the compact previous-period comparison: closed-sale metrics
// only (inventory is a current snapshot and is not computed for the past).
type priorStatsOut struct {
	PeriodDays                   int     `json:"period_days,omitempty" jsonschema:"length of this prior window in days"`
	MedianClosePrice             int64   `json:"median_close_price,omitempty" jsonschema:"median close price in whole US dollars"`
	AvgClosePrice                int64   `json:"avg_close_price,omitempty" jsonschema:"average close price in whole US dollars"`
	MedianPPSF                   int64   `json:"median_ppsf,omitempty" jsonschema:"median close price per square foot in whole US dollars"`
	MedianDaysOnMarket           int     `json:"median_days_on_market,omitempty" jsonschema:"median days on market of closed sales"`
	MedianCumulativeDaysOnMarket int     `json:"median_cumulative_days_on_market,omitempty" jsonschema:"median cumulative days on market (across relists)"`
	SaleToListRatio              float64 `json:"sale_to_list_ratio,omitempty" jsonschema:"median close ÷ final list price"`
	SaleToOriginalRatio          float64 `json:"sale_to_original_ratio,omitempty" jsonschema:"median close ÷ original list price"`
	ClosedInPeriod               int64   `json:"closed_in_period,omitempty" jsonschema:"number of sales closed in this prior window"`
}

// marketStatsOut is the wire shape of market_stats.
type marketStatsOut struct {
	PeriodDays                   int            `json:"period_days" jsonschema:"trailing window aggregated over, in days"`
	MedianListPrice              int64          `json:"median_list_price,omitempty" jsonschema:"median list price of active inventory, whole US dollars"`
	ActiveInventory              int64          `json:"active_inventory,omitempty" jsonschema:"count of active listings in the area/type right now"`
	MonthsOfSupply               float64        `json:"months_of_supply,omitempty" jsonschema:"active inventory ÷ monthly closed-sale rate; under ~6 favors sellers"`
	MedianClosePrice             int64          `json:"median_close_price,omitempty" jsonschema:"median close price of sales in the period, whole US dollars"`
	AvgClosePrice                int64          `json:"avg_close_price,omitempty" jsonschema:"average close price of sales in the period, whole US dollars"`
	MedianPPSF                   int64          `json:"median_ppsf,omitempty" jsonschema:"median close price per square foot, whole US dollars"`
	MedianDaysOnMarket           int            `json:"median_days_on_market,omitempty" jsonschema:"median days on market of closed sales"`
	MedianCumulativeDaysOnMarket int            `json:"median_cumulative_days_on_market,omitempty" jsonschema:"median cumulative days on market (across relists); a gap vs median_days_on_market suggests DOM resets"`
	SaleToListRatio              float64        `json:"sale_to_list_ratio,omitempty" jsonschema:"median close ÷ final list price (negotiation off the last ask)"`
	SaleToOriginalRatio          float64        `json:"sale_to_original_ratio,omitempty" jsonschema:"median close ÷ original list price (total markdown from first ask)"`
	ClosedInPeriod               int64          `json:"closed_in_period,omitempty" jsonschema:"number of sales that closed in the period"`
	Prior                        *priorStatsOut `json:"prior,omitempty" jsonschema:"the immediately preceding window when compare_to_prior is set"`
	DataAsOf                     string         `json:"data_as_of,omitempty" jsonschema:"how current the underlying data is (RFC3339 UTC)"`
}

func registerStats(srv *mcp.Server, source mls.Source) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "market_stats",
		Description: statsDescription,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in statsInput) (*mcp.CallToolResult, marketStatsOut, error) {
		q, err := toStatsQuery(in)
		if err != nil {
			return nil, marketStatsOut{}, err
		}
		m, err := source.MarketStats(ctx, q)
		if err != nil {
			return nil, marketStatsOut{}, err
		}
		return nil, toStatsOutput(m), nil
	})
}

func toStatsQuery(in statsInput) (mls.StatsQuery, error) {
	area := mls.Area{City: in.City, PostalCode: in.PostalCode, County: in.County, State: in.State}
	if strings.TrimSpace(area.City) == "" && strings.TrimSpace(area.PostalCode) == "" &&
		strings.TrimSpace(area.County) == "" && strings.TrimSpace(area.State) == "" {
		return mls.StatsQuery{}, errors.New("provide an area (city, postal_code, county, or state) — market_stats does not aggregate the whole feed")
	}
	q := mls.StatsQuery{Area: area, PropertyTypes: in.PropertyTypes, CompareToPrior: in.CompareToPrior}
	if in.PeriodDays > 0 {
		q.Period = time.Duration(in.PeriodDays) * 24 * time.Hour
	}
	return q, nil
}

func toStatsOutput(m *mls.MarketStats) marketStatsOut {
	out := marketStatsOut{
		PeriodDays:                   m.PeriodDays,
		MedianListPrice:              m.MedianListPrice,
		ActiveInventory:              m.ActiveInventory,
		MonthsOfSupply:               m.MonthsOfSupply,
		MedianClosePrice:             m.MedianClosePrice,
		AvgClosePrice:                m.AvgClosePrice,
		MedianPPSF:                   m.MedianPPSF,
		MedianDaysOnMarket:           m.MedianDaysOnMarket,
		MedianCumulativeDaysOnMarket: m.MedianCumulativeDaysOnMarket,
		SaleToListRatio:              m.SaleToListRatio,
		SaleToOriginalRatio:          m.SaleToOriginalRatio,
		ClosedInPeriod:               m.ClosedInPeriod,
		DataAsOf:                     formatTime(m.DataAsOf),
	}
	if m.Prior != nil {
		p := m.Prior
		out.Prior = &priorStatsOut{
			PeriodDays:                   p.PeriodDays,
			MedianClosePrice:             p.MedianClosePrice,
			AvgClosePrice:                p.AvgClosePrice,
			MedianPPSF:                   p.MedianPPSF,
			MedianDaysOnMarket:           p.MedianDaysOnMarket,
			MedianCumulativeDaysOnMarket: p.MedianCumulativeDaysOnMarket,
			SaleToListRatio:              p.SaleToListRatio,
			SaleToOriginalRatio:          p.SaleToOriginalRatio,
			ClosedInPeriod:               p.ClosedInPeriod,
		}
	}
	return out
}
