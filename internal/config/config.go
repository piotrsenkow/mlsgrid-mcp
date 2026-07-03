// Package config loads mlsgrid-mcp configuration from an optional YAML file
// with MLSGRID_MCP_-prefixed environment overrides. mlsgrid-mcp is a read-only
// consumer of a mlsgrid-sync database, so configuration is small: where the
// database is and which schema holds its tables. No secrets belong in the file
// — supply the connection string via MLSGRID_MCP_DATABASE_URL.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the resolved mlsgrid-mcp configuration.
type Config struct {
	Database Database `mapstructure:"database"`
	Server   Server   `mapstructure:"server"`
	SQL      SQL      `mapstructure:"sql"`
}

type Database struct {
	// URL is a Postgres connection string. Prefer setting it via
	// MLSGRID_MCP_DATABASE_URL over the config file. For production, point it
	// at a read-only role (see docs/adapters.md).
	URL string `mapstructure:"url"`
	// Schema is the Postgres schema holding the mlsgrid-sync tables.
	Schema string `mapstructure:"schema"`
}

type Server struct {
	// Name is advertised to MCP clients in the handshake.
	Name string `mapstructure:"name"`
}

// SQL configures the opt-in query_sql escape hatch. It is off by default and,
// even when enabled, is only exposed over a connection that is not a superuser
// (see the query_sql section of the README). Point the server at a
// least-privilege read-only role before turning it on.
type SQL struct {
	// Enabled exposes the query_sql tool. Off by default.
	Enabled bool `mapstructure:"enabled"`
	// MaxRows is the default row cap per query (0 uses the adapter default). A
	// hard ceiling is enforced regardless.
	MaxRows int `mapstructure:"max_rows"`
	// Timeout bounds each query's execution (statement_timeout). 0 uses the
	// adapter default.
	Timeout time.Duration `mapstructure:"timeout"`
}

func setDefaults(v *viper.Viper) {
	// database.url defaults empty but must be registered: viper's AutomaticEnv
	// only surfaces env overrides for keys it already knows, so without this
	// MLSGRID_MCP_DATABASE_URL is ignored when the file has no database section.
	v.SetDefault("database.url", "")
	v.SetDefault("database.schema", "mlsgrid")
	v.SetDefault("server.name", "mlsgrid-mcp")
	// The escape hatch is off unless the operator opts in.
	v.SetDefault("sql.enabled", false)
	v.SetDefault("sql.max_rows", 1000)
	v.SetDefault("sql.timeout", "5s")
}

// Load reads configuration from path. If path is empty it searches
// ./mlsgrid-mcp.yaml, then $XDG_CONFIG_HOME/mlsgrid-mcp/config.yaml. A missing
// file is not an error — env vars plus defaults may be enough. Environment
// variables prefixed MLSGRID_MCP_ override file values (e.g.
// MLSGRID_MCP_DATABASE_URL overrides database.url).
func Load(path string) (*Config, error) {
	v := viper.New()
	setDefaults(v)
	v.SetEnvPrefix("MLSGRID_MCP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, err
		}
	} else {
		v.SetConfigName("mlsgrid-mcp")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		if xdg := configHome(); xdg != "" {
			v.SetConfigName("config")
			v.AddConfigPath(filepath.Join(xdg, "mlsgrid-mcp"))
		}
		if err := v.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if !errors.As(err, &notFound) {
				return nil, err
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Database.URL) == "" {
		return fmt.Errorf("database.url is required — set MLSGRID_MCP_DATABASE_URL or database.url in the config file")
	}
	return nil
}

func configHome() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return x
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".config")
	}
	return ""
}
