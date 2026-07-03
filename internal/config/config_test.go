package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadEnvOnly(t *testing.T) {
	// No config file: env + defaults must be enough. This exercises the
	// AutomaticEnv-needs-a-registered-default fix for database.url.
	t.Setenv("MLSGRID_MCP_DATABASE_URL", "postgres://u:p@localhost:5432/mls")
	// Point discovery at an empty dir so no stray ./mlsgrid-mcp.yaml is found.
	t.Chdir(t.TempDir())

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.URL != "postgres://u:p@localhost:5432/mls" {
		t.Errorf("database.url = %q, want the env value", cfg.Database.URL)
	}
	if cfg.Database.Schema != "mlsgrid" {
		t.Errorf("database.schema = %q, want default mlsgrid", cfg.Database.Schema)
	}
	if cfg.Server.Name != "mlsgrid-mcp" {
		t.Errorf("server.name = %q, want default mlsgrid-mcp", cfg.Server.Name)
	}
	// The SQL escape hatch is off by default with conservative limits.
	if cfg.SQL.Enabled {
		t.Error("sql.enabled = true, want default false")
	}
	if cfg.SQL.MaxRows != 1000 {
		t.Errorf("sql.max_rows = %d, want default 1000", cfg.SQL.MaxRows)
	}
	if cfg.SQL.Timeout != 5*time.Second {
		t.Errorf("sql.timeout = %v, want default 5s", cfg.SQL.Timeout)
	}
}

func TestLoadSQLEnvOverride(t *testing.T) {
	t.Setenv("MLSGRID_MCP_DATABASE_URL", "postgres://u:p@localhost:5432/mls")
	t.Setenv("MLSGRID_MCP_SQL_ENABLED", "true")
	t.Setenv("MLSGRID_MCP_SQL_MAX_ROWS", "250")
	t.Setenv("MLSGRID_MCP_SQL_TIMEOUT", "10s") // exercises viper's duration decode hook
	t.Chdir(t.TempDir())

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SQL.Enabled {
		t.Error("sql.enabled = false, want true from env")
	}
	if cfg.SQL.MaxRows != 250 {
		t.Errorf("sql.max_rows = %d, want 250 from env", cfg.SQL.MaxRows)
	}
	if cfg.SQL.Timeout != 10*time.Second {
		t.Errorf("sql.timeout = %v, want 10s from env", cfg.SQL.Timeout)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("MLSGRID_MCP_DATABASE_URL", "")
	t.Chdir(t.TempDir())
	if _, err := Load(""); err == nil {
		t.Fatal("expected error when database.url is unset")
	}
}

func TestLoadFileAndEnvOverride(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "mlsgrid-mcp.yaml")
	content := "database:\n  url: postgres://file/db\n  schema: custom\nserver:\n  name: from-file\n"
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	// Env override wins over the file value.
	t.Setenv("MLSGRID_MCP_DATABASE_URL", "postgres://env/db")

	cfg, err := Load(file)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.URL != "postgres://env/db" {
		t.Errorf("env should override file: got %q", cfg.Database.URL)
	}
	if cfg.Database.Schema != "custom" {
		t.Errorf("schema = %q, want custom from file", cfg.Database.Schema)
	}
	if cfg.Server.Name != "from-file" {
		t.Errorf("server.name = %q, want from-file", cfg.Server.Name)
	}
}
