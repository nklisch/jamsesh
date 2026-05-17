package lease_test

import (
	"context"
	"testing"
	"time"

	"jamsesh/internal/portal/lease"
)

// TestNoopManagerImplementsManager verifies that NoopManager satisfies the
// Manager interface at compile time.
func TestNoopManagerImplementsManager(t *testing.T) {
	var _ lease.Manager = lease.NoopManager{}
}

// TestNoopAcquireReturnsHandle verifies that Acquire succeeds and that the
// returned Handle satisfies the interface contract.
func TestNoopAcquireReturnsHandle(t *testing.T) {
	ctx := context.Background()
	m := lease.NoopManager{}

	h, err := m.Acquire(ctx, "ses-abc123")
	if err != nil {
		t.Fatalf("Acquire returned unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("Acquire returned nil Handle")
	}
	defer h.Release() //nolint:errcheck
}

// TestNoopHandleSessionID verifies that the Handle echoes back the sessionID
// passed to Acquire.
func TestNoopHandleSessionID(t *testing.T) {
	const want = "ses-hello"
	h, _ := lease.NoopManager{}.Acquire(context.Background(), want)
	defer h.Release() //nolint:errcheck

	if got := h.SessionID(); got != want {
		t.Errorf("SessionID() = %q; want %q", got, want)
	}
}

// TestNoopHandleFencingTokenIsZero verifies the "no fencing required"
// sentinel value.
func TestNoopHandleFencingTokenIsZero(t *testing.T) {
	h, _ := lease.NoopManager{}.Acquire(context.Background(), "ses-1")
	defer h.Release() //nolint:errcheck

	if tok := h.FencingToken(); tok != 0 {
		t.Errorf("FencingToken() = %d; want 0", tok)
	}
}

// TestNoopHandleLostDoesNotFireBeforeRelease verifies that the Lost channel
// stays open while the lease is held so that consumer select loops are not
// accidentally triggered.
func TestNoopHandleLostDoesNotFireBeforeRelease(t *testing.T) {
	h, _ := lease.NoopManager{}.Acquire(context.Background(), "ses-2")
	defer h.Release() //nolint:errcheck

	select {
	case <-h.Lost():
		t.Error("Lost() fired before Release() was called")
	default:
		// correct: channel is open
	}
}

// TestNoopHandleLostFiresAfterRelease verifies that Lost() closes promptly
// once Release() is called, unblocking any consumer goroutine that is
// selecting on it.
func TestNoopHandleLostFiresAfterRelease(t *testing.T) {
	h, _ := lease.NoopManager{}.Acquire(context.Background(), "ses-3")

	if err := h.Release(); err != nil {
		t.Fatalf("Release() returned unexpected error: %v", err)
	}

	select {
	case <-h.Lost():
		// correct: channel closed after Release
	case <-time.After(time.Second):
		t.Error("Lost() did not close within 1s after Release()")
	}
}

// TestNoopHandleReleaseIsIdempotent verifies that calling Release() multiple
// times does not panic or return an error.
func TestNoopHandleReleaseIsIdempotent(t *testing.T) {
	h, _ := lease.NoopManager{}.Acquire(context.Background(), "ses-4")

	for i := range 5 {
		if err := h.Release(); err != nil {
			t.Errorf("Release() call %d returned error: %v", i+1, err)
		}
	}
}

// TestNoopAcquireCancelledContext verifies that Acquire respects a
// pre-cancelled context and returns its error rather than a Handle.
func TestNoopAcquireCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h, err := lease.NoopManager{}.Acquire(ctx, "ses-5")
	if err == nil {
		h.Release() //nolint:errcheck
		t.Error("Acquire with cancelled context returned nil error")
	}
	if h != nil {
		t.Error("Acquire with cancelled context returned non-nil Handle")
	}
}

// TestNoopMultipleAcquiresSameSession verifies that NoopManager permits
// multiple simultaneous acquisitions for the same session (single-instance
// mode; no mutual exclusion needed).
func TestNoopMultipleAcquiresSameSession(t *testing.T) {
	ctx := context.Background()
	m := lease.NoopManager{}

	h1, err := m.Acquire(ctx, "ses-shared")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer h1.Release() //nolint:errcheck

	h2, err := m.Acquire(ctx, "ses-shared")
	if err != nil {
		t.Fatalf("second Acquire for same session: %v", err)
	}
	defer h2.Release() //nolint:errcheck
}

// TestNoopConsumerSelectShape validates the consumer-side select pattern that
// object-storage-sync and hydration-handoff will use. This ensures the
// channel shape works correctly in both single and clustered modes.
func TestNoopConsumerSelectShape(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	h, err := lease.NoopManager{}.Acquire(ctx, "ses-consumer")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Simulate a consumer goroutine that processes work and then calls Release.
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Simulated work unit.
		select {
		case <-h.Lost():
			t.Errorf("lease lost unexpectedly during work")
		case <-ctx.Done():
			t.Errorf("context expired before work completed")
		case <-time.After(10 * time.Millisecond):
			// Work completed normally.
		}
		h.Release() //nolint:errcheck
	}()

	// After Release the Lost channel should be closed.
	<-done
	select {
	case <-h.Lost():
		// correct
	case <-time.After(time.Second):
		t.Error("Lost() did not close after Release() in consumer goroutine")
	}
}
