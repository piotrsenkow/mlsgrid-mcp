// Package mls defines the capability-shaped interface that mlsgrid-mcp's tools
// are built on, together with the query and result types they exchange. It has
// zero dependencies on any database driver or on the MCP SDK.
//
// The design centers on one seam: an adapter (see adapters/postgres) implements
// [Source] over some concrete data store, and the server package turns any
// Source into MCP tools. Because the interface is capability-shaped rather than
// table-shaped, a private data source can implement Source out-of-tree and
// reuse the server unchanged — plain Go composition, no plugins.
//
// The types here target mlsgrid-sync's schema contract v1 conceptually, but
// deliberately do not mirror its columns: they describe what an MLS query
// answers, not how a table is laid out. Money is expressed in whole dollars
// (MLS feeds carry no cents on prices), and every result that a tool returns
// carries a data-as-of timestamp so agents can reason about staleness.
//
// This package is pre-1.0; method sets are stable but individual query/result
// fields may still grow as tools land (see docs/ROADMAP.md).
package mls

import (
	"context"
	"errors"
	"time"
)

// ErrNotImplemented is returned by a Source for a capability it does not
// support. Tools consult [Source.Capabilities] to avoid calling unsupported
// methods, but returning this error is the safe fallback and callers should
// surface it as "unavailable" rather than a failure.
var ErrNotImplemented = errors.New("mls: capability not implemented by this source")

// ErrNotFound is returned when a lookup by key or MLS number matches nothing.
var ErrNotFound = errors.New("mls: not found")

// ErrAmbiguousRef is returned when a lookup by MLS number (without an
// originating system) matches listings in more than one feed. The caller should
// retry with ListingRef.OriginatingSystem set.
var ErrAmbiguousRef = errors.New("mls: reference matches multiple originating systems; specify one")

// Source is a read-only view over one or more MLS feeds. Implementations must
// be safe for concurrent use: the MCP server may invoke tools from multiple
// client requests at once.
//
// Every method takes a context and must honor its cancellation. Methods a
// source cannot serve should return [ErrNotImplemented] rather than panicking,
// and should be reflected in [Capabilities] so tools can route around them.
type Source interface {
	// Capabilities reports which optional features this source supports, plus
	// static facts (schema contract version, covered originating systems) that
	// let tools degrade gracefully — for example, omitting distance when the
	// source has no coordinates, or noting that price history only exists from
	// a given date forward.
	Capabilities(ctx context.Context) (Capabilities, error)

	// Freshness reports how current the underlying data is: per-resource sync
	// cursors, listing counts by status, media coverage, and the schema
	// contract version. It powers the get_data_freshness tool and doubles as a
	// pipeline-liveness check — a trust signal an agent can read before relying
	// on the other tools.
	Freshness(ctx context.Context) (Freshness, error)

	// SearchListings returns a page of listing summaries matching a query.
	// Pagination is opaque-cursor based via Page.NextCursor.
	SearchListings(ctx context.Context, q SearchQuery) (Page[ListingSummary], error)

	// GetListing returns full detail for a single listing referenced by its
	// ListingKey or human-facing MLS number. Returns [ErrNotFound] if nothing
	// matches.
	GetListing(ctx context.Context, ref ListingRef, opts ListingOptions) (*ListingDetail, error)

	// FindComparables returns comparable listings for a subject property,
	// scored by similarity and (when the source has coordinates) distance.
	FindComparables(ctx context.Context, q CompsQuery) (*CompsResult, error)

	// MarketStats aggregates market metrics over an area and time period.
	MarketStats(ctx context.Context, q StatsQuery) (*MarketStats, error)

	// PriceHistory returns the observed change timeline for a listing, built
	// from mlsgrid-sync's append-only change capture. It is best-effort from
	// first sync forward; see Capabilities.HistorySince.
	PriceHistory(ctx context.Context, ref ListingRef) (*PriceHistory, error)

	// OpenHouses returns scheduled open houses for an area and date range,
	// wrapped with a data-as-of stamp.
	OpenHouses(ctx context.Context, q OpenHouseQuery) (OpenHouseResult, error)

	// Close releases resources held by the source (connection pools, etc.).
	Close() error
}

// SQLQuerier is an optional capability a Source may also implement to back the
// opt-in query_sql tool. It is deliberately separate from Source: exposing raw
// SQL is off by default and gated behind both server configuration and a
// source that supports it. Enforcement is primarily the database's job (a
// read-only role, read-only transactions); implementations must still refuse
// anything that is not a single read-only statement.
type SQLQuerier interface {
	// QueryReadOnly executes a single read-only statement and returns at most
	// maxRows rows. It must reject multi-statement input and any write.
	QueryReadOnly(ctx context.Context, query string, maxRows int) (*ResultSet, error)
}

// ListingRef identifies a listing either by its cross-MLS ListingKey (e.g.
// "MRD12345678") or by the human-facing MLS number scoped to an originating
// system. Exactly one form should be set; Key takes precedence when both are.
type ListingRef struct {
	// Key is MLS Grid's cross-MLS unique ListingKey.
	Key string
	// MLSNumber is the listing's local MLS number (ListingId).
	MLSNumber string
	// OriginatingSystem scopes MLSNumber when set (MLS numbers are unique only
	// within a system). Ignored when Key is used.
	OriginatingSystem string
}

// Empty reports whether the reference identifies nothing.
func (r ListingRef) Empty() bool { return r.Key == "" && r.MLSNumber == "" }

// Page is a single page of results with an opaque cursor for the next page.
type Page[T any] struct {
	// Items are the results on this page.
	Items []T
	// NextCursor, when non-empty, fetches the following page; empty means the
	// last page. Its encoding is an implementation detail of the source.
	NextCursor string
	// Total is the total number of matches when the source can compute it
	// cheaply, or -1 when unknown.
	Total int64
	// DataAsOf is when the underlying data was last known current.
	DataAsOf time.Time
}

// ResultSet is a generic tabular result for the opt-in SQL escape hatch.
type ResultSet struct {
	Columns   []string
	Rows      [][]any
	Truncated bool
	DataAsOf  time.Time
}
