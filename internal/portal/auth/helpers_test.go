package auth_test

import (
	"context"
	"testing"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
)

// openStore opens a fresh in-memory SQLite store for a single test.
// The store is automatically closed when the test completes.
func openStore(t *testing.T) store.Store {
	t.Helper()
	s, err := db.Open(context.Background(), "sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
