package cli

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/piotrsenkow/mlsgrid-mcp/adapters/postgres"
	"github.com/piotrsenkow/mlsgrid-mcp/mls"
	"github.com/piotrsenkow/mlsgrid-mcp/server"
)

// openSource builds the default Postgres adapter from loaded config.
func openSource(cmd *cobra.Command) (mls.Source, error) {
	return postgres.New(cmd.Context(), cfg.Database.URL, postgres.Options{
		Schema:     cfg.Database.Schema,
		SQLMaxRows: cfg.SQL.MaxRows,
		SQLTimeout: cfg.SQL.Timeout,
	})
}

// runServe is the root command: serve the MCP protocol over stdio.
func runServe(cmd *cobra.Command, _ []string) error {
	src, err := openSource(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	sqlEnabled := resolveSQL(src)
	srv, err := server.New(src, server.WithInfo(cfg.Server.Name, version), server.WithSQL(sqlEnabled))
	if err != nil {
		return err
	}
	slog.Info("serving mlsgrid-mcp over stdio", "schema", cfg.Database.Schema, "query_sql", sqlEnabled)
	// Run blocks until the client disconnects or the context is canceled.
	if err := srv.Run(cmd.Context(), &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("serving over stdio: %w", err)
	}
	return nil
}

// sqlSafer is the optional preflight a source can implement to veto exposing
// query_sql over an unsafe (e.g. superuser) connection.
type sqlSafer interface{ SQLSafe() error }

// resolveSQL decides whether to expose the query_sql escape hatch. It must be
// enabled in config, the source must implement mls.SQLQuerier, and — as a safety
// backstop — the connection must pass the source's SQLSafe preflight. Any failed
// precondition disables query_sql while leaving the curated tools serving.
func resolveSQL(src mls.Source) bool {
	if !cfg.SQL.Enabled {
		return false
	}
	if _, ok := src.(mls.SQLQuerier); !ok {
		slog.Warn("sql.enabled is set but this data source does not support query_sql; not exposing it")
		return false
	}
	if safe, ok := src.(sqlSafer); ok {
		if err := safe.SQLSafe(); err != nil {
			slog.Error("sql.enabled is set but the connection is unsafe for query_sql; not exposing it", "reason", err)
			return false
		}
	}
	slog.Warn("query_sql escape hatch ENABLED — MCP clients can run read-only SQL against the database")
	return true
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Verify database connectivity, contract version, and data freshness",
	Long: `check opens the configured database, asserts the schema-contract version, and
prints a data-freshness summary — the same information the get_data_freshness
tool returns, in human-readable form. Use it to confirm the server will start
and that the sync pipeline is producing current data.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		src, err := openSource(cmd)
		if err != nil {
			return err
		}
		defer func() { _ = src.Close() }()

		ctx := cmd.Context()
		caps, err := src.Capabilities(ctx)
		if err != nil {
			return err
		}
		f, err := src.Freshness(ctx)
		if err != nil {
			return err
		}

		// check is a one-shot diagnostic, not the serving path, so writing the
		// report to stdout is correct here.
		sqlEffective, sqlNote := sqlStatus(src, caps)
		fmt.Printf("schema:   %s (contract %s)\n", cfg.Database.Schema, f.SchemaContractVersion)
		fmt.Printf("feeds:    %s\n", joinOrDash(caps.OriginatingSystems))
		fmt.Printf("features: geo=%t price_history=%t open_houses=%t sql=%t%s\n", caps.Geo, caps.PriceHistory, caps.OpenHouses, sqlEffective, sqlNote)
		fmt.Printf("listings: %d total\n", f.TotalListings)

		for _, c := range f.Cursors {
			fmt.Printf("\n%s/%s\n", c.Resource, c.OriginatingSystem)
			fmt.Printf("  stored:     %d\n", c.StoredRows)
			fmt.Printf("  watermark:  %s\n", fmtTimePtr(c.Watermark))
			fmt.Printf("  backfill:   %s\n", backfillLabel(c.BackfillComplete))
			fmt.Printf("  reconciled: %s\n", fmtTimePtr(c.LastReconcile))
		}

		if len(f.ListingStatusCounts) > 0 {
			fmt.Printf("\nstatus\n")
			for _, sc := range f.ListingStatusCounts {
				fmt.Printf("  %-22s %d\n", sc.Status, sc.Count)
			}
		}
		if len(f.MediaCounts) > 0 {
			fmt.Printf("\nmedia\n")
			for _, sc := range f.MediaCounts {
				fmt.Printf("  %-22s %d\n", sc.Status, sc.Count)
			}
		}
		fmt.Printf("\ndata as of %s\n", fmtTime(f.DataAsOf))
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the mlsgrid-mcp version",
	Args:  cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println(version)
	},
}

// sqlStatus reports whether query_sql would actually be exposed given config,
// source support, and the connection preflight, plus a short parenthetical when
// it is configured on but withheld.
func sqlStatus(src mls.Source, caps mls.Capabilities) (bool, string) {
	if !cfg.SQL.Enabled {
		return false, ""
	}
	if !caps.SQL {
		return false, " (config-enabled; source has no SQL support)"
	}
	if safe, ok := src.(sqlSafer); ok {
		if err := safe.SQLSafe(); err != nil {
			return false, " (config-enabled; withheld: " + err.Error() + ")"
		}
	}
	return true, ""
}

func joinOrDash(s []string) string {
	if len(s) == 0 {
		return "—"
	}
	out := s[0]
	for _, v := range s[1:] {
		out += ", " + v
	}
	return out
}

func backfillLabel(complete bool) string {
	if complete {
		return "complete"
	}
	return "in progress (incremental sync blocked)"
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.UTC().Format(time.RFC3339)
}

func fmtTimePtr(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return fmtTime(*t)
}
