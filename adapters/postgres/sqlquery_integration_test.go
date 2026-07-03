//go:build integration

package postgres

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/piotrsenkow/mlsgrid-mcp/server"
)

func TestQueryReadOnlyHappyPath(t *testing.T) {
	ctx := context.Background()
	rs, err := testAdapter.QueryReadOnly(ctx, "SELECT count(*) AS n FROM property", 0)
	if err != nil {
		t.Fatalf("QueryReadOnly: %v", err)
	}
	if len(rs.Columns) != 1 || rs.Columns[0] != "n" {
		t.Errorf("columns = %v, want [n]", rs.Columns)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rs.Rows))
	}
	if got, ok := rs.Rows[0][0].(int64); !ok || got != 12 {
		t.Errorf("count = %v (%T), want int64(12)", rs.Rows[0][0], rs.Rows[0][0])
	}
	if rs.Truncated {
		t.Error("Truncated = true, want false for a single-row aggregate")
	}
	if rs.DataAsOf.IsZero() {
		t.Error("DataAsOf is zero, want the newest property timestamp")
	}
}

func TestQueryReadOnlyProjection(t *testing.T) {
	ctx := context.Background()
	// Unqualified table name resolves because search_path is pinned to the schema.
	rs, err := testAdapter.QueryReadOnly(ctx,
		"SELECT listing_key, list_price FROM property WHERE listing_key = 'MRD1003'", 0)
	if err != nil {
		t.Fatalf("QueryReadOnly: %v", err)
	}
	if len(rs.Columns) != 2 || rs.Columns[0] != "listing_key" || rs.Columns[1] != "list_price" {
		t.Errorf("columns = %v", rs.Columns)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rs.Rows))
	}
	if rs.Rows[0][0] != "MRD1003" {
		t.Errorf("listing_key = %v, want MRD1003", rs.Rows[0][0])
	}
	// list_price is a numeric column; the generic SQL path renders numerics as
	// their string form to preserve precision (JSON numbers can't represent
	// arbitrary-precision numerics losslessly). The curated tools scan these into
	// int64 dollars, but query_sql is deliberately faithful to the column type.
	if rs.Rows[0][1] != "500000" {
		t.Errorf("list_price = %v (%T), want string \"500000\"", rs.Rows[0][1], rs.Rows[0][1])
	}
}

func TestQueryReadOnlyTruncation(t *testing.T) {
	ctx := context.Background()
	// 12 rows exist; a cap of 3 must truncate and report it.
	rs, err := testAdapter.QueryReadOnly(ctx, "SELECT listing_key FROM property", 3)
	if err != nil {
		t.Fatalf("QueryReadOnly: %v", err)
	}
	if len(rs.Rows) != 3 {
		t.Errorf("rows = %d, want 3 (capped)", len(rs.Rows))
	}
	if !rs.Truncated {
		t.Error("Truncated = false, want true (12 rows > cap 3)")
	}
}

func TestQueryReadOnlyRejectsUnsafe(t *testing.T) {
	ctx := context.Background()
	unsafe := []string{
		"UPDATE property SET list_price = 0",
		"DELETE FROM property",
		"DROP TABLE property",
		"INSERT INTO property (listing_key) VALUES ('X')",
		"SELECT * INTO evil FROM property",
		"SELECT 1; SELECT 2",
		"SELECT pg_sleep(30)",
	}
	for _, q := range unsafe {
		if _, err := testAdapter.QueryReadOnly(ctx, q, 0); err == nil {
			t.Errorf("QueryReadOnly(%q) succeeded, want rejection", q)
		}
	}
	// The corpus above must not have mutated anything.
	rs, err := testAdapter.QueryReadOnly(ctx, "SELECT count(*) AS n FROM property", 0)
	if err != nil {
		t.Fatalf("recount: %v", err)
	}
	if got := rs.Rows[0][0].(int64); got != 12 {
		t.Errorf("property count = %d after unsafe attempts, want 12 (unchanged)", got)
	}
}

func TestQueryReadOnlyStatementTimeout(t *testing.T) {
	ctx := context.Background()
	// A dedicated adapter with a tiny timeout; a deliberately heavy (but valid,
	// guard-passing) query must be cancelled rather than run to completion.
	fast, err := New(ctx, testDSN, Options{Schema: testSchema, SQLTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = fast.Close() }()

	if _, err := fast.QueryReadOnly(ctx, "SELECT count(*) FROM generate_series(1, 1000000000)", 0); err == nil {
		t.Error("heavy query completed, want a statement-timeout error")
	}
}

func TestSQLSafeRejectsSuperuser(t *testing.T) {
	// The testcontainers default role is a superuser, so the safety preflight must
	// veto exposing query_sql over it...
	if err := testAdapter.SQLSafe(); err == nil {
		t.Error("SQLSafe() = nil for a superuser connection, want an error")
	}
	// ...while QueryReadOnly itself still functions (the veto is at the expose
	// gate, not per query), as the happy-path test confirms.
}

func TestSQLSafeAllowsLeastPrivilegeRole(t *testing.T) {
	ctx := context.Background()
	const role = "mcp_ro_test"
	const pass = "ro_pass"

	admin, err := pgx.Connect(ctx, testDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer func() { _ = admin.Close(ctx) }()

	for _, stmt := range []string{
		fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s'", pgxQuoteIdent(role), pass),
		fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s", pgxQuoteIdent(testSchema), pgxQuoteIdent(role)),
		fmt.Sprintf("GRANT SELECT ON ALL TABLES IN SCHEMA %s TO %s", pgxQuoteIdent(testSchema), pgxQuoteIdent(role)),
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			t.Fatalf("provision role (%q): %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, fmt.Sprintf("DROP OWNED BY %s", pgxQuoteIdent(role)))
		_, _ = admin.Exec(ctx, fmt.Sprintf("DROP ROLE IF EXISTS %s", pgxQuoteIdent(role)))
	})

	roDSN := withUserPassword(t, testDSN, role, pass)
	roAdapter, err := New(ctx, roDSN, Options{Schema: testSchema})
	if err != nil {
		t.Fatalf("New as least-privilege role: %v", err)
	}
	defer func() { _ = roAdapter.Close() }()

	if err := roAdapter.SQLSafe(); err != nil {
		t.Errorf("SQLSafe() = %v for a non-superuser role, want nil", err)
	}
	rs, err := roAdapter.QueryReadOnly(ctx, "SELECT count(*) AS n FROM property", 0)
	if err != nil {
		t.Fatalf("QueryReadOnly as role: %v", err)
	}
	if got := rs.Rows[0][0].(int64); got != 12 {
		t.Errorf("count = %d, want 12", got)
	}
}

func TestEndToEndQuerySQLOverMCP(t *testing.T) {
	ctx := context.Background()
	srv, err := server.New(testAdapter, server.WithInfo("mlsgrid-mcp-it", "test"), server.WithSQL(true))
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	clientT, serverT := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "it-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query_sql",
		Arguments: map[string]any{"query": "SELECT count(*) AS n FROM property"},
	})
	if err != nil {
		t.Fatalf("query_sql CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("query_sql tool error: %+v", res.Content)
	}
	var out struct {
		Columns  []string `json:"columns"`
		Rows     [][]any  `json:"rows"`
		RowCount int      `json:"row_count"`
	}
	decodeStructured(t, res.StructuredContent, &out)
	if len(out.Columns) != 1 || out.Columns[0] != "n" || out.RowCount != 1 {
		t.Fatalf("columns=%v row_count=%d, want [n]/1", out.Columns, out.RowCount)
	}
	if fmt.Sprint(out.Rows[0][0]) != "12" {
		t.Errorf("count = %v, want 12", out.Rows[0][0])
	}
}

// withUserPassword rewrites a Postgres URL DSN's credentials, for connecting as
// a different role than the one TestMain used.
func withUserPassword(t *testing.T, dsn, user, pass string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.User = url.UserPassword(user, pass)
	return u.String()
}
