package postgres

import (
	"context"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// The methods below complete the mls.Source interface but are not implemented
// yet. They land in later milestones (see docs/ROADMAP.md): get_comps /
// price_history in B-M3, market_stats / get_open_houses in B-M4. Until then they
// return mls.ErrNotImplemented so the adapter satisfies the interface and the
// server can register only the tools it can actually serve. SearchListings and
// GetListing (B-M2) live in search.go and listing.go.

// FindComparables is not yet implemented (B-M3).
func (a *Adapter) FindComparables(ctx context.Context, q mls.CompsQuery) (*mls.CompsResult, error) {
	return nil, mls.ErrNotImplemented
}

// MarketStats is not yet implemented (B-M4).
func (a *Adapter) MarketStats(ctx context.Context, q mls.StatsQuery) (*mls.MarketStats, error) {
	return nil, mls.ErrNotImplemented
}

// PriceHistory is not yet implemented (B-M3).
func (a *Adapter) PriceHistory(ctx context.Context, ref mls.ListingRef) (*mls.PriceHistory, error) {
	return nil, mls.ErrNotImplemented
}

// OpenHouses is not yet implemented (B-M4).
func (a *Adapter) OpenHouses(ctx context.Context, q mls.OpenHouseQuery) ([]mls.OpenHouse, error) {
	return nil, mls.ErrNotImplemented
}
