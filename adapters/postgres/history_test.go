package postgres

import (
	"math"
	"testing"
	"time"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

func TestParseMoney(t *testing.T) {
	ok := map[string]float64{
		"520000":     520000,
		"$1,200,000": 1200000,
		"  300000  ": 300000,
		"3.5":        3.5,
	}
	for in, want := range ok {
		if got, valid := parseMoney(in); !valid || got != want {
			t.Errorf("parseMoney(%q) = %v,%v want %v,true", in, got, valid, want)
		}
	}
	for _, in := range []string{"", "  ", "abc", "N/A"} {
		if _, valid := parseMoney(in); valid {
			t.Errorf("parseMoney(%q) = valid, want invalid", in)
		}
	}
}

func TestTotalReductionPct(t *testing.T) {
	pc := func(old, new string) mls.PriceEvent {
		return mls.PriceEvent{EventType: "price_change", OldValue: old, NewValue: new}
	}
	cases := []struct {
		name   string
		events []mls.PriceEvent
		want   float64
	}{
		{"single drop", []mls.PriceEvent{pc("520000", "500000")}, 3.8461538},
		{"two drops", []mls.PriceEvent{pc("500000", "450000"), pc("450000", "400000")}, 20},
		{"increase is negative", []mls.PriceEvent{pc("400000", "450000")}, -12.5},
		{"no price_change", []mls.PriceEvent{{EventType: "status_change", OldValue: "Active", NewValue: "Closed"}}, 0},
		{"empty", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := totalReductionPct(tc.events)
			if math.Abs(got-tc.want) > 0.001 {
				t.Errorf("totalReductionPct = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDaysSinceLastChange(t *testing.T) {
	if d := daysSinceLastChange(nil); d != 0 {
		t.Errorf("empty = %d, want 0", d)
	}
	// A future-dated event clamps to 0 rather than going negative.
	future := []mls.PriceEvent{{ObservedAt: now().Add(48 * time.Hour)}}
	if d := daysSinceLastChange(future); d != 0 {
		t.Errorf("future = %d, want 0", d)
	}
	// ~100 days ago → 99 or 100 after truncation.
	past := []mls.PriceEvent{{ObservedAt: now().Add(-100 * 24 * time.Hour)}}
	if d := daysSinceLastChange(past); d < 99 || d > 100 {
		t.Errorf("past = %d, want ~100", d)
	}
}
