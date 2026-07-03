package postgres

import (
	"context"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// The methods below complete the mls.Source interface but are not implemented
// yet. They land in later milestones (see docs/ROADMAP.md): search_listings /
// get_listing in B-M2, get_comps / price_history in B-M3, market_stats /
// get_open_houses in B-M4. Until then they return mls.ErrNotImplemented so the
// adapter satisfies the interface and the server can register only the tools it
// can actually serve.

// SearchListings is not yet implemented (B-M2).
func (a *Adapter) SearchListings(ctx context.Context, q mls.SearchQuery) (mls.Page[mls.ListingSummary], error) {
	return mls.Page[mls.ListingSummary]{}, mls.ErrNotImplemented
}

// GetListing is not yet implemented (B-M2).
func (a *Adapter) GetListing(ctx context.Context, ref mls.ListingRef, opts mls.ListingOptions) (*mls.ListingDetail, error) {
	return nil, mls.ErrNotImplemented
}

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
