// Package postgres is the default mls.Source adapter. It reads the database
// schema that mlsgrid-sync produces (schema-contract.md), never writing to it.
//
// The adapter targets contract major version 1. It reads the live contract
// version from the store's schema_meta table at startup and refuses to run on
// a major-version mismatch, per the contract's consumer rule — a mismatched
// schema is a configuration error, not something to paper over at query time.
package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// ExpectedContractMajor is the schema-contract major version this adapter
// understands. A store reporting a different major is refused at startup.
const ExpectedContractMajor = 1

// DefaultSchema is the Postgres schema mlsgrid-sync uses by default.
const DefaultSchema = "mlsgrid"

// Options configures the adapter.
type Options struct {
	// Schema is the Postgres schema holding the mlsgrid-sync tables. Defaults
	// to DefaultSchema. It must be an existing identifier; it is validated and
	// quoted, never interpolated raw.
	Schema string
}

// Adapter implements mls.Source (and, later, mls.SQLQuerier) over a
// mlsgrid-sync Postgres database.
type Adapter struct {
	pool            *pgxpool.Pool
	schema          string
	contractVersion string
}

// compile-time assertion that Adapter satisfies the Source contract.
var _ mls.Source = (*Adapter)(nil)

// New opens a connection pool to dsn, validates the schema contract version,
// and returns a ready adapter. The caller owns the adapter's lifetime and must
// call Close.
func New(ctx context.Context, dsn string, opts Options) (*Adapter, error) {
	schema := opts.Schema
	if schema == "" {
		schema = DefaultSchema
	}
	if !validIdentifier(schema) {
		return nil, fmt.Errorf("postgres: invalid schema name %q", schema)
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}
	// The adapter is read-only by contract; enforce it at the session level so
	// a stray write (or a future SQL escape hatch) cannot mutate the feed.
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}

	a := &Adapter{pool: pool, schema: schema}
	if err := a.assertContract(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return a, nil
}

// Close releases the connection pool.
func (a *Adapter) Close() error {
	a.pool.Close()
	return nil
}

// rel returns a safely-quoted "schema"."table" reference.
func (a *Adapter) rel(table string) string {
	return pgx.Identifier{a.schema, table}.Sanitize()
}

// assertContract reads schema_meta.contract_version and rejects a major-version
// mismatch. It also caches the live version for Capabilities/Freshness.
func (a *Adapter) assertContract(ctx context.Context) error {
	var version string
	q := fmt.Sprintf(`SELECT value FROM %s WHERE key = 'contract_version'`, a.rel("schema_meta"))
	if err := a.pool.QueryRow(ctx, q).Scan(&version); err != nil {
		return fmt.Errorf("postgres: reading contract version from %s.schema_meta (is this a mlsgrid-sync database?): %w", a.schema, err)
	}
	major, err := majorVersion(version)
	if err != nil {
		return fmt.Errorf("postgres: unparseable contract version %q: %w", version, err)
	}
	if major != ExpectedContractMajor {
		return fmt.Errorf("postgres: schema contract major mismatch: store reports %q, this adapter supports v%d.x", version, ExpectedContractMajor)
	}
	a.contractVersion = version
	return nil
}

// validIdentifier accepts a conservative subset of Postgres identifiers so a
// schema name from config can be safely quoted.
func validIdentifier(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// majorVersion parses the leading integer of a semantic version string.
func majorVersion(v string) (int, error) {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, '.'); i >= 0 {
		v = v[:i]
	}
	return strconv.Atoi(v)
}

// now returns the current time in UTC. Wrapped so tests can reason about it.
func now() time.Time { return time.Now().UTC() }
