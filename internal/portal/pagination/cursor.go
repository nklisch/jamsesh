// Package pagination provides cursor-based pagination helpers for the portal API.
//
// Cursor format: base64url(json{filter_hash, last_created_at_unix_ns, last_id})
//
// The filter_hash is a SHA-256 hex digest of the sorted "k=v" pairs of the
// current query parameters. On each request the cursor's filter_hash is
// re-computed from the request's query params and compared; a mismatch returns
// ErrFilterMismatch so callers can return 400 pagination.cursor_filter_mismatch.
package pagination

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrFilterMismatch is returned by Decode when the cursor's filter_hash does
// not match a freshly computed hash of the current query params.
var ErrFilterMismatch = errors.New("pagination: cursor filter hash mismatch")

// Cursor is the decoded form of an opaque page cursor.
type Cursor struct {
	// FilterHash is the SHA-256 hex digest of the filter params that produced this cursor.
	FilterHash string `json:"filter_hash"`
	// LastCreatedAtNs is the created_at of the last item on the previous page,
	// stored as UTC Unix nanoseconds. Used as the exclusive upper bound for the
	// next query (created_at < LastCreatedAt, DESC order).
	LastCreatedAtNs int64 `json:"last_created_at_ns"`
	// LastID is the ID of the last item on the previous page. Used as a
	// tiebreaker when multiple rows have the same created_at.
	LastID string `json:"last_id"`
}

// LastCreatedAt returns LastCreatedAtNs as a time.Time in UTC.
func (c Cursor) LastCreatedAt() time.Time {
	return time.Unix(0, c.LastCreatedAtNs).UTC()
}

// Encode serialises the cursor to a base64url-encoded JSON string.
func Encode(c Cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Decode parses and validates an opaque cursor string. If currentFilter is
// non-nil, the cursor's filter_hash must match FilterHash(currentFilter).
// Pass nil to skip the filter check (e.g. the first page has no cursor).
func Decode(s string, currentFilter map[string]string) (Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, fmt.Errorf("pagination: decode cursor: %w", err)
	}
	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return Cursor{}, fmt.Errorf("pagination: unmarshal cursor: %w", err)
	}
	if currentFilter != nil {
		expected := FilterHash(currentFilter)
		if c.FilterHash != expected {
			return Cursor{}, ErrFilterMismatch
		}
	}
	return c, nil
}

// NewCursor constructs a cursor pointing just past the given item. The
// filter argument is used to bind the cursor to the current filter set.
func NewCursor(createdAt time.Time, id string, filter map[string]string) Cursor {
	return Cursor{
		FilterHash:      FilterHash(filter),
		LastCreatedAtNs: createdAt.UTC().UnixNano(),
		LastID:          id,
	}
}

// FilterHash returns a stable SHA-256 hex digest of the provided key=value
// query parameter map. Keys are sorted before hashing so the result is
// deterministic regardless of insertion order.
func FilterHash(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}

	h := sha256.Sum256([]byte(strings.Join(parts, "&")))
	return fmt.Sprintf("%x", h)
}
