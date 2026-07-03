// Package sqlguard is a lexical pre-filter for the opt-in query_sql escape
// hatch. It rejects input that is not a single, read-only SELECT before that
// input ever reaches the database.
//
// It is deliberately NOT a SQL parser. The authoritative enforcement lives in
// the database — a least-privilege read-only role, a read-only transaction, and
// a statement timeout (see adapters/postgres). This guard is the first of those
// defense-in-depth layers: it catches the obvious and the malicious cheaply, so
// the database only ever sees input that already looks like one harmless query.
//
// Because it errs toward safety, the guard can reject a legitimate query that
// uses a reserved-ish word (e.g. "set", "copy", "into") as an unquoted
// identifier. Double-quote such identifiers to pass — a small price for a guard
// that never has to reason about SQL semantics.
package sqlguard

import (
	"fmt"
	"strings"
)

// maxQueryLen bounds input length so a pathological string can't make the
// scanner do unbounded work. Real analytic queries are far shorter.
const maxQueryLen = 20000

// Validate reports whether q is a single read-only statement safe to execute,
// returning the cleaned statement (comments stripped, trailing semicolon
// removed) ready to be wrapped and run. A non-nil error means the query was
// rejected and must not be executed.
func Validate(q string) (string, error) {
	if len(q) > maxQueryLen {
		return "", fmt.Errorf("query too long (%d bytes; limit %d)", len(q), maxQueryLen)
	}

	stmts := scan(q)
	nonEmpty := stmts[:0]
	for _, s := range stmts {
		if s.text != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}
	switch len(nonEmpty) {
	case 0:
		return "", fmt.Errorf("empty query")
	case 1:
		// ok
	default:
		return "", fmt.Errorf("only a single statement is allowed (found %d; separate calls, not %q)", len(nonEmpty), ";")
	}

	s := nonEmpty[0]
	if len(s.tokens) == 0 {
		return "", fmt.Errorf("no SQL keyword found")
	}
	if lead := s.tokens[0]; lead != "SELECT" && lead != "WITH" {
		return "", fmt.Errorf("only SELECT or WITH queries are allowed (statement begins with %s)", lead)
	}
	for _, tok := range s.tokens {
		if denied[tok] {
			return "", fmt.Errorf("disallowed keyword %s (query_sql is read-only; writes, DDL, and IO are rejected)", tok)
		}
	}
	return s.text, nil
}

// statement is one top-level statement recovered from the input: its cleaned
// text (comments removed) plus the upper-cased word tokens found outside of
// string literals, quoted identifiers, and dollar-quoted bodies.
type statement struct {
	text   string
	tokens []string
}

// scan walks q once, stripping comments, splitting on top-level semicolons, and
// collecting keyword tokens. It understands the lexical contexts that could
// otherwise hide a comment, a statement separator, or a keyword: single-quoted
// strings, double-quoted identifiers, and dollar-quoted bodies. Everything
// inside those contexts is copied through verbatim but never tokenized.
func scan(q string) []statement {
	var out []statement
	var cur strings.Builder
	var tokens []string
	var word strings.Builder

	flushWord := func() {
		if word.Len() > 0 {
			tokens = append(tokens, strings.ToUpper(word.String()))
			word.Reset()
		}
	}
	endStatement := func() {
		flushWord()
		out = append(out, statement{text: strings.TrimSpace(cur.String()), tokens: tokens})
		cur.Reset()
		tokens = nil
	}

	n := len(q)
	for i := 0; i < n; {
		c := q[i]
		switch {
		case c == '-' && i+1 < n && q[i+1] == '-':
			flushWord()
			j := i + 2
			for j < n && q[j] != '\n' {
				j++
			}
			cur.WriteByte(' ')
			i = j
		case c == '/' && i+1 < n && q[i+1] == '*':
			flushWord()
			depth, j := 1, i+2
			for j < n && depth > 0 {
				if q[j] == '/' && j+1 < n && q[j+1] == '*' {
					depth++
					j += 2
					continue
				}
				if q[j] == '*' && j+1 < n && q[j+1] == '/' {
					depth--
					j += 2
					continue
				}
				j++
			}
			cur.WriteByte(' ')
			i = j
		case c == '\'':
			flushWord()
			i = copyQuoted(q, i, '\'', &cur)
		case c == '"':
			flushWord()
			i = copyQuoted(q, i, '"', &cur)
		case c == '$':
			if tag, ok := dollarTag(q, i); ok {
				flushWord()
				i = copyDollar(q, i, tag, &cur)
			} else {
				cur.WriteByte(c)
				i++
			}
		case c == ';':
			endStatement()
			i++
		case isWordByte(c):
			word.WriteByte(c)
			cur.WriteByte(c)
			i++
		default:
			flushWord()
			cur.WriteByte(c)
			i++
		}
	}
	endStatement()
	return out
}

// copyQuoted copies a quoted run beginning at q[i] (the opening quote) through
// its closing quote, honoring the doubled-quote escape (a repeated quote is a
// literal, not a close), and returns the index just past the close. An
// unterminated quote consumes to end of input.
func copyQuoted(q string, i int, quote byte, cur *strings.Builder) int {
	cur.WriteByte(quote)
	j := i + 1
	for j < len(q) {
		if q[j] == quote {
			if j+1 < len(q) && q[j+1] == quote {
				cur.WriteByte(quote)
				cur.WriteByte(quote)
				j += 2
				continue
			}
			cur.WriteByte(quote)
			return j + 1
		}
		cur.WriteByte(q[j])
		j++
	}
	return j
}

// copyDollar copies a dollar-quoted body $tag$...$tag$ beginning at q[i] and
// returns the index just past the closing tag. An unterminated body consumes to
// end of input.
func copyDollar(q string, i int, tag string, cur *strings.Builder) int {
	cur.WriteString(tag)
	j := i + len(tag)
	if idx := strings.Index(q[j:], tag); idx >= 0 {
		cur.WriteString(q[j : j+idx])
		cur.WriteString(tag)
		return j + idx + len(tag)
	}
	cur.WriteString(q[j:])
	return len(q)
}

// dollarTag returns the dollar-quote delimiter starting at q[i] (which must be
// '$') if one begins there — "$$" or "$tag$" where tag is an identifier. It
// returns false for a positional parameter like $1 or a bare '$'.
func dollarTag(q string, i int) (string, bool) {
	for j := i + 1; j < len(q); j++ {
		c := q[j]
		if c == '$' {
			return q[i : j+1], true
		}
		isTagByte := c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (j > i+1 && c >= '0' && c <= '9')
		if !isTagByte {
			return "", false
		}
	}
	return "", false
}

// isWordByte reports whether c can be part of an unquoted SQL identifier/keyword.
func isWordByte(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

// denied is the set of upper-cased tokens that must never appear in a query_sql
// statement. It blocks data modification (reachable via data-modifying CTEs and
// SELECT ... INTO), DDL, session/transaction control, and server-side IO or
// sleep functions. Reserved-word column names or aliases collide with these;
// double-quote them to pass.
var denied = wordSet(
	// Data modification.
	"INSERT", "UPDATE", "DELETE", "MERGE", "UPSERT", "INTO", "TRUNCATE", "COPY",
	// DDL.
	"DROP", "CREATE", "ALTER", "GRANT", "REVOKE", "REINDEX", "REFRESH",
	"CLUSTER", "VACUUM", "COMMENT", "SECURITY",
	// Session / transaction / program control.
	"SET", "RESET", "LOCK", "CALL", "DO", "PREPARE", "EXECUTE", "DEALLOCATE",
	"DISCARD", "LISTEN", "NOTIFY", "UNLISTEN", "SAVEPOINT",
	// Server-side IO, large objects, sleeps, and admin functions.
	"PG_READ_FILE", "PG_READ_BINARY_FILE", "PG_READ_SERVER_FILES", "PG_LS_DIR",
	"PG_STAT_FILE", "PG_LOGDIR_LS", "PG_WRITE_FILE",
	"LO_IMPORT", "LO_EXPORT", "LO_GET", "LO_PUT", "LO_FROM_BYTEA",
	"DBLINK", "DBLINK_EXEC",
	"PG_SLEEP", "PG_SLEEP_FOR", "PG_SLEEP_UNTIL",
	"PG_TERMINATE_BACKEND", "PG_CANCEL_BACKEND", "PG_RELOAD_CONF", "SET_CONFIG",
	"PG_ADVISORY_LOCK", "PG_ADVISORY_XACT_LOCK",
)

func wordSet(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}
