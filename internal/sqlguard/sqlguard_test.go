package sqlguard

import (
	"strings"
	"testing"
)

func TestValidateAccepts(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"simple select", "SELECT count(*) FROM property"},
		{"lowercase", "select 1"},
		{"leading whitespace", "   SELECT 1  "},
		{"with cte", "WITH t AS (SELECT 1) SELECT * FROM t"},
		{"recursive cte", "WITH RECURSIVE r AS (SELECT 1) SELECT * FROM r"},
		{"parenthesized select", "(SELECT 1 UNION SELECT 2)"},
		{"trailing semicolon", "SELECT 1;"},
		{"trailing semicolon and space", "SELECT 1 ;  "},
		{"line comment", "SELECT 1 -- a comment\n"},
		{"block comment", "SELECT /* hi */ 1"},
		{"semicolon inside string", "SELECT ';' AS s"},
		{"denied word inside string literal", "SELECT 'DROP TABLE x' AS note"},
		{"denied word as quoted identifier", `SELECT col AS "update" FROM property`},
		{"information_schema discovery", "SELECT table_name FROM information_schema.columns WHERE table_schema = 'mlsgrid'"},
		{"dollar quoted body", "SELECT $$ ; DROP TABLE x $$ AS lit"},
		{"positional param not dollar quote", "SELECT * FROM property WHERE id = $1"},
		{"underscore columns not keywords", "SELECT close_price, list_price FROM property"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Validate(tc.in)
			if err != nil {
				t.Fatalf("Validate(%q) rejected: %v", tc.in, err)
			}
			// The cleaned form is what gets wrapped and executed: it must retain no
			// comment marker and no trailing semicolon (a top-level ';' would end the
			// wrapped subquery). Semicolons *inside* string literals are preserved and
			// harmless, so only the trailing case is asserted here.
			if strings.Contains(got, "--") || strings.Contains(got, "/*") {
				t.Errorf("cleaned statement still contains a comment: %q", got)
			}
			if strings.HasSuffix(strings.TrimSpace(got), ";") {
				t.Errorf("cleaned statement still ends with a semicolon: %q", got)
			}
		})
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"whitespace only", "   \n\t"},
		{"comment only", "-- just a comment"},
		{"block comment only", "/* nothing here */"},
		{"insert", "INSERT INTO property VALUES (1)"},
		{"update", "UPDATE property SET list_price = 0"},
		{"delete", "DELETE FROM property"},
		{"drop", "DROP TABLE property"},
		{"truncate", "TRUNCATE property"},
		{"alter", "ALTER TABLE property ADD COLUMN x int"},
		{"create", "CREATE TABLE t (id int)"},
		{"grant", "GRANT SELECT ON property TO public"},
		{"select into creates a table", "SELECT * INTO evil FROM property"},
		{"data modifying cte", "WITH t AS (DELETE FROM property RETURNING *) SELECT * FROM t"},
		{"data modifying cte insert", "WITH t AS (INSERT INTO property DEFAULT VALUES RETURNING *) SELECT * FROM t"},
		{"multi statement", "SELECT 1; SELECT 2"},
		{"multi statement write", "SELECT 1; DROP TABLE property"},
		{"stacked write after comment", "SELECT 1;/* x */ DROP TABLE property"},
		{"leading verb not select", "TABLE property"},
		{"values statement", "VALUES (1), (2)"},
		{"explain analyze", "EXPLAIN ANALYZE SELECT 1"},
		{"copy", "COPY property TO STDOUT"},
		{"copy to program", "COPY (SELECT 1) TO PROGRAM 'id'"},
		{"set statement", "SET statement_timeout = 0"},
		{"pg_read_file", "SELECT pg_read_file('/etc/passwd')"},
		{"pg_read_file schema qualified", "SELECT pg_catalog.pg_read_file('/etc/passwd')"},
		{"pg_sleep", "SELECT pg_sleep(10)"},
		{"lo_export", "SELECT lo_export(1, '/tmp/x')"},
		{"dblink", "SELECT * FROM dblink('conn', 'SELECT 1') AS t(a int)"},
		{"set_config", "SELECT set_config('x', 'y', false)"},
		{"do block", "DO $$ BEGIN END $$"},
		{"call procedure", "CALL some_proc()"},
		{"lock table", "LOCK TABLE property"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := Validate(tc.in); err == nil {
				t.Errorf("Validate(%q) accepted (cleaned %q), want rejected", tc.in, got)
			}
		})
	}
}

// TestSemicolonInStringIsNotASeparator guards the specific injection where a
// semicolon hidden inside a string literal is used to smuggle a second
// statement past the single-statement check.
func TestSemicolonInStringIsNotASeparator(t *testing.T) {
	got, err := Validate("SELECT 'a; DROP TABLE property' AS note")
	if err != nil {
		t.Fatalf("rejected a benign string containing a semicolon: %v", err)
	}
	if !strings.Contains(got, "DROP TABLE property") {
		t.Errorf("string literal content was altered: %q", got)
	}
}

// TestCommentCannotHideKeyword ensures comment stripping doesn't glue tokens
// together in a way that hides a denied keyword from the deny-list scan.
func TestCommentCannotHideKeyword(t *testing.T) {
	// "UP" + comment + "DATE" must not be read as the single token UPDATE; it is
	// two tokens after the comment becomes whitespace, and neither is denied — but
	// the real attack (a genuine UPDATE) is still caught.
	if _, err := Validate("SELECT 1 /* */ FROM property"); err != nil {
		t.Fatalf("benign comment rejected: %v", err)
	}
	if _, err := Validate("UP/**/DATE property SET x=1"); err == nil {
		// Leading token is UP (not SELECT/WITH) → rejected regardless.
		t.Error("expected rejection: statement does not begin with SELECT/WITH")
	}
}

func TestTooLong(t *testing.T) {
	huge := "SELECT " + strings.Repeat("1,", maxQueryLen)
	if _, err := Validate(huge); err == nil {
		t.Error("expected rejection for an over-length query")
	}
}
