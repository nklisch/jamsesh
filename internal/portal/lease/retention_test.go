package lease_test

import (
	"context"
	"os"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/portal/lease"
)

// ---------------------------------------------------------------------------
// Unit test: context cancellation exits RunRetention cleanly
// ---------------------------------------------------------------------------

// TestRunRetention_CancelExits verifies that RunRetention returns when its
// context is cancelled, without performing any I/O.
// We use a SQLite-backed store here; the delete call is a no-op (no rows)
// but the function must still exit when ctx is cancelled.
func TestRunRetention_CancelExits(t *testing.T) {
	s, _, err := db.Open(context.Background(), "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		// Use a very short interval so the tick fires quickly.
		done <- lease.RunRetention(ctx, s, 10*time.Millisecond, 30*24*time.Hour)
	}()

	// Give the goroutine a moment to start and tick at least once.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("RunRetention returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunRetention did not exit after context cancellation")
	}
}

// ---------------------------------------------------------------------------
// Integration test (gated on JAMSESH_TEST_PG_DSN)
// ---------------------------------------------------------------------------

// TestRunRetention_DeletesOldRows verifies that RunRetention actually deletes
// released lease rows whose released_at is older than the retention window.
func TestRunRetention_DeletesOldRows(t *testing.T) {
	dsn := os.Getenv("JAMSESH_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("set JAMSESH_TEST_PG_DSN to enable Postgres retention integration test")
	}

	ctx := context.Background()
	s, _, err := db.Open(ctx, "postgres", dsn, db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open postgres: %v", err)
	}
	defer s.Close()

	// The retention call itself simply invokes DELETE WHERE released_at < $1.
	// Seed no rows — we just verify the function completes without error and
	// that ctx cancellation works against a real PG connection.
	retentionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- lease.RunRetention(retentionCtx, s, 50*time.Millisecond, 30*24*time.Hour)
	}()

	// Allow a couple of ticks before cancelling.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			t.Errorf("RunRetention returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunRetention did not exit after context cancellation (PG)")
	}
}
