// Package server turns an mls.Source into a Model Context Protocol server. It
// is the public composition point: New registers curated tools on an
// mcp.Server from the official Go SDK, and the caller runs that server over a
// transport (stdio in v1). A private data source only has to implement
// mls.Source and pass it here to reuse every tool unchanged.
package server

import (
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// DefaultName is the MCP server name advertised to clients.
const DefaultName = "mlsgrid-mcp"

type options struct {
	name    string
	version string
}

// Option configures the server.
type Option func(*options)

// WithInfo sets the server name and version advertised in the MCP handshake.
func WithInfo(name, version string) Option {
	return func(o *options) {
		if name != "" {
			o.name = name
		}
		if version != "" {
			o.version = version
		}
	}
}

// New builds an MCP server backed by source and registers the curated tools it
// can serve. The returned *mcp.Server is ready to Run over a transport; the
// caller owns both the server and the source's lifetime.
//
// Tools are registered milestone by milestone: get_data_freshness (B-M1),
// search_listings and get_listing (B-M2), price_history + get_comps (B-M3), and
// market_stats + get_open_houses (B-M4). New does not take ownership of source
// and never closes it.
func New(source mls.Source, opts ...Option) (*mcp.Server, error) {
	if source == nil {
		return nil, errors.New("server: source must not be nil")
	}
	o := options{name: DefaultName, version: "dev"}
	for _, fn := range opts {
		fn(&o)
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: o.name, Version: o.version}, nil)
	registerFreshness(srv, source)
	registerSearch(srv, source)
	registerListing(srv, source)
	registerHistory(srv, source)
	registerComps(srv, source)
	registerStats(srv, source)
	registerOpenHouses(srv, source)
	return srv, nil
}
