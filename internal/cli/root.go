// Package cli defines the mlsgrid-mcp command tree. Commands are thin: they
// load config, open the data source, and either serve the MCP protocol over
// stdio or run a one-shot diagnostic. Human-readable output and logs go to
// stderr and stdout only outside of `serve` — while serving, stdout is
// reserved for the JSON-RPC transport and must not be written to directly.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/piotrsenkow/mlsgrid-mcp/internal/config"
)

// version is set at build time via -ldflags (see Makefile / .goreleaser.yaml).
var version = "dev"

var (
	cfgFile string
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "mlsgrid-mcp",
	Short: "MCP server exposing a mlsgrid-sync database to AI agents",
	Long: `mlsgrid-mcp is a Model Context Protocol server that answers real-estate
questions over a database populated by mlsgrid-sync (RESO / MLS Grid data).

Run with no arguments to serve over stdio — this is how MCP clients such as
Claude Desktop and Claude Code launch it. Use "check" to verify connectivity
and data freshness from a terminal.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "version" || cmd.Name() == "help" {
			return nil
		}
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		return nil
	},
	// Default action: serve over stdio.
	RunE: runServe,
}

// Execute runs the root command. SIGINT/SIGTERM cancel the command context so
// the stdio server shuts down cleanly when its client disconnects.
func Execute() error {
	// Logs go to stderr; stdout belongs to the MCP stdio transport.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return rootCmd.ExecuteContext(ctx)
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "path to config file (default: ./mlsgrid-mcp.yaml, then $XDG_CONFIG_HOME/mlsgrid-mcp/config.yaml)")
	rootCmd.AddCommand(checkCmd, versionCmd)
}
