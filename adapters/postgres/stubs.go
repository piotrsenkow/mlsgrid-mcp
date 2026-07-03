package postgres

import (
	"context"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// The methods below complete the mls.Source interface but are not implemented
// yet. They land in B-M4 (see docs/ROADMAP.md): market_stats / get_open_houses.
// Until then they return mls.ErrNotImplemented so the adapter satisfies the
// interface and the server registers only the tools it can serve.
// SearchListings/GetListing (B-M2) live in search.go and listing.go;
// PriceHistory and FindComparables (B-M3) in history.go and comps.go.

// MarketStats is not yet implemented (B-M4).
func (a *Adapter) MarketStats(ctx context.Context, q mls.StatsQuery) (*mls.MarketStats, error) {
	return nil, mls.ErrNotImplemented
}

// OpenHouses is not yet implemented (B-M4).
func (a *Adapter) OpenHouses(ctx context.Context, q mls.OpenHouseQuery) ([]mls.OpenHouse, error) {
	return nil, mls.ErrNotImplemented
}
