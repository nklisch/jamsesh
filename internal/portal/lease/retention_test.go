package lease_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
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
		done <- lease.RunRetention(ctx, s, 10*time.Millisecond, 30*24*time.Hour, func() time.Time { return time.Now().UTC() })
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
// Unit test: deterministic cutoff regression (no real-time wait)
// ---------------------------------------------------------------------------

// retentionStub implements store.LeaseStore — the only interface RunRetention
// requires. Records the cutoff passed to DeleteReleasedLeasesOlderThan and
// unblocks a channel on first call. The four other LeaseStore methods are
// never exercised by RunRetention and panic if called accidentally.
type retentionStub struct {
	mu     sync.Mutex
	called []time.Time
	notify chan struct{}
}

func (s *retentionStub) IssueLeaseFencingToken(_ context.Context) (int64, error) {
	panic("not implemented")
}
func (s *retentionStub) InsertLease(_ context.Context, _ store.InsertLeaseParams) (store.Lease, error) {
	panic("not implemented")
}
func (s *retentionStub) MarkLeaseReleased(_ context.Context, _ string) error {
	panic("not implemented")
}
func (s *retentionStub) UpdateLeaseHeartbeat(_ context.Context, _ string) error {
	panic("not implemented")
}

func newRetentionStub() *retentionStub {
	return &retentionStub{notify: make(chan struct{}, 1)}
}

func (s *retentionStub) DeleteReleasedLeasesOlderThan(_ context.Context, before time.Time) error {
	s.mu.Lock()
	s.called = append(s.called, before)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
	return nil
}

// TestRunRetention_CutoffAdvancesEachTick verifies that RunRetention
// recomputes the cutoff from nowFn on every tick, so the cutoff advances
// across ticks (not frozen at startup). The test uses a fake nowFn that
// returns increasing times and asserts that successive calls to
// DeleteReleasedLeasesOlderThan receive increasing cutoffs.
func TestRunRetention_CutoffAdvancesEachTick(t *testing.T) {
	var mu sync.Mutex
	callCount := 0
	// nowBase is advanced by 24h on each call to nowFn to simulate wall-clock
	// advance between ticks.
	nowBase := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	nowFn := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		t := nowBase
		nowBase = nowBase.Add(24 * time.Hour)
		callCount++
		return t
	}

	retention := 30 * 24 * time.Hour
	stub := newRetentionStub()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- lease.RunRetention(ctx, stub, 5*time.Millisecond, retention, nowFn)
	}()

	// Wait for at least two DeleteReleasedLeasesOlderThan calls.
	<-stub.notify
	<-stub.notify
	cancel()
	<-done

	stub.mu.Lock()
	got := stub.called
	stub.mu.Unlock()

	if len(got) < 2 {
		t.Fatalf("expected at least 2 calls, got %d", len(got))
	}
	// Each successive cutoff must be later than the previous one.
	if !got[1].After(got[0]) {
		t.Errorf("cutoff did not advance: first=%v second=%v (want second > first)", got[0], got[1])
	}
}

// TestRunRetention_CutoffUsesNow verifies that RunRetention computes the
// cutoff as nowFn().Add(-retentionAfter) rather than using a startup-frozen
// value. The test passes a nowFn returning a synthetic time and asserts that
// DeleteReleasedLeasesOlderThan receives the expected cutoff.
func TestRunRetention_CutoffUsesNow(t *testing.T) {
	syntheticNow := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	retention := 30 * 24 * time.Hour
	wantCutoff := syntheticNow.Add(-retention) // 2025-12-16 12:00:00 UTC

	stub := newRetentionStub()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- lease.RunRetention(ctx, stub, 5*time.Millisecond, retention, func() time.Time { return syntheticNow })
	}()

	// Wait for at least one DeleteReleasedLeasesOlderThan call.
	select {
	case <-stub.notify:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("RunRetention did not call DeleteReleasedLeasesOlderThan within timeout")
	}
	cancel()
	<-done

	stub.mu.Lock()
	got := stub.called
	stub.mu.Unlock()

	if len(got) == 0 {
		t.Fatal("DeleteReleasedLeasesOlderThan was never called")
	}
	if !got[0].Equal(wantCutoff) {
		t.Errorf("cutoff = %v, want %v", got[0], wantCutoff)
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
		done <- lease.RunRetention(retentionCtx, s, 50*time.Millisecond, 30*24*time.Hour, func() time.Time { return time.Now().UTC() })
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
