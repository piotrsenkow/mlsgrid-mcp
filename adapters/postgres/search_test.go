package postgres

import (
	"strings"
	"testing"
	"time"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

// buildSearchWhere touches nothing on the Adapter, so a zero value is fine for
// these pure SQL-assembly tests (no database required).
func TestBuildSearchWhereEmpty(t *testing.T) {
	var a Adapter
	var args argList
	where, err := a.buildSearchWhere(&args, mls.SearchQuery{}, true)
	if err != nil {
		t.Fatalf("buildSearchWhere: %v", err)
	}
	if where != "" {
		t.Errorf("empty query where = %q, want empty", where)
	}
	if len(args.args) != 0 {
		t.Errorf("empty query args = %v, want none", args.args)
	}
}

func TestBuildSearchWhereFilters(t *testing.T) {
	var a Adapter
	var args argList
	q := mls.SearchQuery{
		Area:            mls.Area{City: "Chicago", State: "IL"},
		Statuses:        []string{"Active", "Pending"},
		PropertyTypes:   []string{"Condominium"},
		MinPrice:        200000,
		MaxPrice:        500000,
		MinBeds:         2,
		MinBathsFull:    1,
		MinLivingArea:   1000,
		MaxLivingArea:   3000,
		MinYearBuilt:    1990,
		MaxDaysOnMarket: 60,
		Keywords:        "lake view",
	}
	where, err := a.buildSearchWhere(&args, q, true)
	if err != nil {
		t.Fatalf("buildSearchWhere: %v", err)
	}
	if !strings.HasPrefix(where, " WHERE ") {
		t.Fatalf("where = %q, want leading WHERE", where)
	}
	// 13 predicates → 13 args (statuses and property_types are each one array
	// arg; property_types reuses its single placeholder twice).
	wantConds := 13
	if got := strings.Count(where, " AND ") + 1; got != wantConds {
		t.Errorf("conds = %d, want %d\nwhere: %s", got, wantConds, where)
	}
	if len(args.args) != wantConds {
		t.Errorf("args = %d, want %d", len(args.args), wantConds)
	}
	// City/county/state are matched case-insensitively.
	if !strings.Contains(where, "lower(city) = lower(") {
		t.Errorf("city predicate missing lower(): %s", where)
	}
	// property_types matches PropertyType OR PropertySubType off one placeholder,
	// case-insensitively.
	if !strings.Contains(where, "lower(property_type) = ANY(") || !strings.Contains(where, "lower(property_sub_type) = ANY(") {
		t.Errorf("property_types should match type and subtype case-insensitively: %s", where)
	}
	// Keyword arg is a wrapped, escaped LIKE pattern.
	last := args.args[len(args.args)-1]
	if last != "%lake view%" {
		t.Errorf("keyword arg = %v, want %q", last, "%lake view%")
	}
}

func TestBuildSearchWhereCursorPredicate(t *testing.T) {
	var a Adapter
	var args argList
	cur := searchCursor{ModTS: time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC), Key: "MRD1002"}
	where, err := a.buildSearchWhere(&args, mls.SearchQuery{Cursor: cur.encode()}, true)
	if err != nil {
		t.Fatalf("buildSearchWhere: %v", err)
	}
	if !strings.Contains(where, "(modification_timestamp, listing_key) < (") {
		t.Errorf("cursor row-value predicate missing: %s", where)
	}
	if len(args.args) != 2 {
		t.Fatalf("cursor args = %d, want 2", len(args.args))
	}
	if args.args[1] != "MRD1002" {
		t.Errorf("cursor key arg = %v, want MRD1002", args.args[1])
	}
}

func TestBuildSearchWhereRejectsBadCursor(t *testing.T) {
	var a Adapter
	var args argList
	if _, err := a.buildSearchWhere(&args, mls.SearchQuery{Cursor: "!!!bad"}, true); err == nil {
		t.Error("expected error for malformed cursor")
	}
}

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"plain":      "plain",
		"50%":        `50\%`,
		"a_b":        `a\_b`,
		`back\slash`: `back\\slash`,
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Errorf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNonEmpty(t *testing.T) {
	got := nonEmpty([]string{"a", "", "  ", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("nonEmpty = %v, want [a b]", got)
	}
}
