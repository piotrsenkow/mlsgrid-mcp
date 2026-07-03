package postgres

import (
	"testing"
	"time"
)

func TestOpenHouseWindowDefaults(t *testing.T) {
	fixed := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	prev := now
	now = func() time.Time { return fixed }
	defer func() { now = prev }()

	week := 7 * 24 * time.Hour

	// Both unset → [now, now+7d].
	from, to := openHouseWindow(time.Time{}, time.Time{})
	if !from.Equal(fixed) || !to.Equal(fixed.Add(week)) {
		t.Errorf("defaults = [%v, %v], want [%v, %v]", from, to, fixed, fixed.Add(week))
	}

	// From set, To unset → To = From + 7d.
	f := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	from, to = openHouseWindow(f, time.Time{})
	if !from.Equal(f) || !to.Equal(f.Add(week)) {
		t.Errorf("open-ended to = [%v, %v]", from, to)
	}

	// Both set → passed through untouched.
	b := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	from, to = openHouseWindow(f, b)
	if !from.Equal(f) || !to.Equal(b) {
		t.Errorf("passthrough = [%v, %v], want [%v, %v]", from, to, f, b)
	}
}

func TestDerefTime(t *testing.T) {
	if _, ok := derefTime(nil); ok {
		t.Error("derefTime(nil) reported ok=true")
	}
	ts := time.Date(2026, 7, 4, 16, 0, 0, 0, time.UTC)
	if got, ok := derefTime(&ts); !ok || !got.Equal(ts) {
		t.Errorf("derefTime(&ts) = %v, %v; want %v, true", got, ok, ts)
	}
}
