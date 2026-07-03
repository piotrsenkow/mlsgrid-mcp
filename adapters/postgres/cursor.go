package postgres

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// searchCursor is the opaque pagination token for SearchListings. It captures
// the sort key of the last row returned so the next page can resume with a
// keyset predicate — no OFFSET, so pages stay stable and index-friendly even as
// the underlying data changes between requests.
//
// The sort is (modification_timestamp DESC, listing_key DESC); listing_key is
// the primary key, giving every row a unique, stable tiebreak.
type searchCursor struct {
	ModTS time.Time `json:"t"`
	Key   string    `json:"k"`
}

// encode renders the cursor as a URL-safe base64 token. The encoding is an
// implementation detail clients must treat as opaque.
func (c searchCursor) encode() string {
	b, _ := json.Marshal(c) // a struct of time+string never fails to marshal
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeCursor parses a token produced by searchCursor.encode. A malformed
// token is an error rather than a silent full-scan reset.
func decodeCursor(tok string) (searchCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return searchCursor{}, fmt.Errorf("invalid cursor encoding: %w", err)
	}
	var c searchCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return searchCursor{}, fmt.Errorf("invalid cursor payload: %w", err)
	}
	if c.Key == "" {
		return searchCursor{}, fmt.Errorf("invalid cursor: empty key")
	}
	return c, nil
}
