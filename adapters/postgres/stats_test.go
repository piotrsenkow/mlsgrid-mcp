package postgres

import "testing"

func TestMonthsOfSupply(t *testing.T) {
	cases := []struct {
		active, closed int64
		days           int
		want           float64
	}{
		{4, 5, 90, 2.4}, // 4 ÷ (5 sales / 3 months) = 2.4
		{6, 6, 30, 1},   // 6 ÷ (6 / 1) = 1
		{10, 0, 90, 0},  // nothing closed → 0, never +Inf
		{0, 5, 90, 0},   // no inventory → 0 months of supply
		{4, 5, 0, 0},    // degenerate period → 0
	}
	for _, c := range cases {
		if got := monthsOfSupply(c.active, c.closed, c.days); got != c.want {
			t.Errorf("monthsOfSupply(%d, %d, %d) = %v, want %v", c.active, c.closed, c.days, got, c.want)
		}
	}
}

func TestRoundHelpers(t *testing.T) {
	if got := round4(0.978261); got != 0.9783 {
		t.Errorf("round4(0.978261) = %v, want 0.9783", got)
	}
	if got := round4(0.957446); got != 0.9574 {
		t.Errorf("round4(0.957446) = %v, want 0.9574", got)
	}
	if got := round2(2.4666); got != 2.47 {
		t.Errorf("round2(2.4666) = %v, want 2.47", got)
	}
}
