package postgres

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	in := searchCursor{
		ModTS: time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC),
		Key:   "MRD1002",
	}
	out, err := decodeCursor(in.encode())
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if !out.ModTS.Equal(in.ModTS) {
		t.Errorf("ModTS = %v, want %v", out.ModTS, in.ModTS)
	}
	if out.Key != in.Key {
		t.Errorf("Key = %q, want %q", out.Key, in.Key)
	}
}

func TestDecodeCursorRejectsGarbage(t *testing.T) {
	cases := map[string]string{
		"not base64":  "!!!not-base64!!!",
		"not json":    base64.RawURLEncoding.EncodeToString([]byte("not json")),
		"empty key":   base64.RawURLEncoding.EncodeToString([]byte(`{"t":"2026-06-05T09:00:00Z","k":""}`)),
		"missing key": base64.RawURLEncoding.EncodeToString([]byte(`{"t":"2026-06-05T09:00:00Z"}`)),
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeCursor(tok); err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}
}
