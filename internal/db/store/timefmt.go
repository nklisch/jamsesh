package store

import (
	"fmt"
	"time"
)

// tsLayout is the UTC ISO-8601 format with Z suffix used in SQLite TEXT
// timestamp columns. This format sorts correctly as a plain string, which is
// important for ORDER BY on TEXT columns in SQLite.
const tsLayout = "2006-01-02T15:04:05Z"

// formatTS converts a time.Time to the canonical SQLite timestamp string.
// The result is always UTC with a Z suffix.
func formatTS(t time.Time) string {
	return t.UTC().Format(tsLayout)
}

// parseTS parses a SQLite timestamp string back to time.Time.
// The returned time is in UTC.
func parseTS(s string) (time.Time, error) {
	t, err := time.Parse(tsLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: parse timestamp %q: %w", s, err)
	}
	return t.UTC(), nil
}
