package finalizecmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestCleanupStack_LIFOOrderOnSuccess verifies that Push'd cleanups
// fire in reverse-registration order on outcomeSuccess — this is the
// shell-like "last in, first out" contract every defer-style cleanup
// stack must honor.
func TestCleanupStack_LIFOOrderOnSuccess(t *testing.T) {
	var buf bytes.Buffer
	c := newCleanupStack(nil, &buf)

	var order []string
	var mu sync.Mutex
	push := func(name string) {
		c.Push(name, false, func() error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, name)
			return nil
		})
	}
	push("a")
	push("b")
	push("c")

	if err := c.Run(outcomeSuccess); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := order, []string{"c", "b", "a"}; !equalStrSlice(got, want) {
		t.Errorf("LIFO order wrong: got %v, want %v", got, want)
	}
}

// TestCleanupStack_ConditionalSkippedOnAbort verifies that
// conditional=true tasks are skipped when Run is invoked with
// outcomeAborted, while unconditional tasks still fire.
func TestCleanupStack_ConditionalSkippedOnAbort(t *testing.T) {
	var buf bytes.Buffer
	c := newCleanupStack(nil, &buf)

	var fired []string
	c.Push("unconditional-a", false, func() error { fired = append(fired, "u-a"); return nil })
	c.Push("conditional-b", true, func() error { fired = append(fired, "c-b"); return nil })
	c.Push("unconditional-c", false, func() error { fired = append(fired, "u-c"); return nil })

	if err := c.Run(outcomeAborted); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// LIFO drain: u-c, c-b (skipped), u-a.
	if got, want := fired, []string{"u-c", "u-a"}; !equalStrSlice(got, want) {
		t.Errorf("conditional skip wrong: got %v, want %v", got, want)
	}
}

// TestCleanupStack_IdempotentRun verifies that calling Run twice is a
// no-op the second time — the stack tracks "drained" state so the
// SIGINT watcher and the main flow can both safely call Run.
func TestCleanupStack_IdempotentRun(t *testing.T) {
	var buf bytes.Buffer
	c := newCleanupStack(nil, &buf)

	count := 0
	c.Push("once", false, func() error {
		count++
		return nil
	})
	if err := c.Run(outcomeSuccess); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := c.Run(outcomeSuccess); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if count != 1 {
		t.Errorf("cleanup fired %d times; want 1 (second Run should be a no-op)", count)
	}
}

// TestCleanupStack_PushAfterDrainIsNoOp verifies that pushing onto a
// drained stack does nothing — important because the SIGINT watcher may
// drain the stack while the main flow is still in mid-Push.
func TestCleanupStack_PushAfterDrainIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	c := newCleanupStack(nil, &buf)
	if err := c.Run(outcomeSuccess); err != nil {
		t.Fatalf("Run: %v", err)
	}
	fired := 0
	c.Push("late", false, func() error { fired++; return nil })
	if err := c.Run(outcomeSuccess); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if fired != 0 {
		t.Errorf("late Push fired %d times; want 0", fired)
	}
}

// TestCleanupStack_CollectsErrors verifies that failing cleanups are
// joined into the returned error (errors.Is finds each one) and that a
// failing cleanup does NOT short-circuit subsequent cleanups.
func TestCleanupStack_CollectsErrors(t *testing.T) {
	var buf bytes.Buffer
	c := newCleanupStack(nil, &buf)

	errA := errors.New("boom-a")
	errC := errors.New("boom-c")
	fired := 0
	c.Push("a", false, func() error { fired++; return errA })
	c.Push("b", false, func() error { fired++; return nil })
	c.Push("c", false, func() error { fired++; return errC })

	err := c.Run(outcomeSuccess)
	if err == nil {
		t.Fatal("expected joined error, got nil")
	}
	if !errors.Is(err, errA) || !errors.Is(err, errC) {
		t.Errorf("joined error missing one of the underlying errs: %v", err)
	}
	if fired != 3 {
		t.Errorf("expected all 3 cleanups to fire, fired=%d", fired)
	}
	// Each failure should also log a warning to the writer.
	if !strings.Contains(buf.String(), "cleanup \"a\" failed") {
		t.Errorf("missing 'a' warning in output: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "cleanup \"c\" failed") {
		t.Errorf("missing 'c' warning in output: %s", buf.String())
	}
}

// TestCleanupStack_CtxCancelDrainsStack verifies the SIGINT path: when
// the context is cancelled, the watcher goroutine drains the stack with
// outcomeAborted. The test uses a sync channel so we can deterministically
// wait for the watcher to finish without sleeping.
func TestCleanupStack_CtxCancelDrainsStack(t *testing.T) {
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	c := newCleanupStack(ctx, &buf)

	done := make(chan string, 2)
	c.Push("unconditional", false, func() error {
		done <- "u"
		return nil
	})
	c.Push("conditional", true, func() error {
		done <- "c"
		return nil
	})

	cancel()

	// Watcher fires Run(outcomeAborted) — wait for the unconditional
	// cleanup to fire.
	select {
	case got := <-done:
		if got != "u" {
			t.Errorf("first fire = %q, want u (conditional should be skipped on abort)", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup never fired after ctx cancel")
	}

	// Conditional must NOT have fired.
	select {
	case got := <-done:
		t.Errorf("conditional fired on abort: got %q", got)
	case <-time.After(100 * time.Millisecond):
		// expected — silence
	}

	// Main flow's Run is now a no-op.
	if err := c.Run(outcomeSuccess); err != nil {
		t.Errorf("post-cancel Run: %v", err)
	}
}

// TestCleanupStack_MainFlowRunStopsWatcher verifies the inverse: when
// the main flow drains the stack, the watcher goroutine exits without
// double-firing the cleanups even after the ctx is cancelled later.
func TestCleanupStack_MainFlowRunStopsWatcher(t *testing.T) {
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newCleanupStack(ctx, &buf)

	count := 0
	c.Push("once", false, func() error { count++; return nil })

	if err := c.Run(outcomeSuccess); err != nil {
		t.Fatalf("Run: %v", err)
	}
	cancel()
	// Give the watcher a moment to exit.
	time.Sleep(50 * time.Millisecond)
	if count != 1 {
		t.Errorf("cleanup fired %d times; want 1", count)
	}
}

// TestCleanupStack_IdempotentCleanupFnSafeOnDoubleDrain verifies the
// caller-side idempotency contract: even if a cleanup func is invoked
// twice (which Run prevents structurally, but suspenders for the belt),
// idempotent cleanups must not error. This mirrors the behavior of
// removeJamseshRemote (which swallows "no such remote").
func TestCleanupStack_IdempotentCleanupFnSafeOnDoubleDrain(t *testing.T) {
	var buf bytes.Buffer
	c := newCleanupStack(nil, &buf)
	calls := 0
	// Idempotent func: first call "succeeds", subsequent calls also
	// succeed without doing real work.
	fn := func() error {
		calls++
		return nil // idempotent — never errors
	}
	c.Push("idempotent", false, fn)
	if err := c.Run(outcomeSuccess); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Second drain is a no-op at the stack level; cleanup is not
	// re-invoked. But if we manually invoke the fn again (simulating a
	// caller racing the watcher), it must not error.
	if err := fn(); err != nil {
		t.Errorf("idempotent fn errored on second manual call: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls=%d, want 2", calls)
	}
}

// equalStrSlice is a tiny helper since the test file does not pull in
// reflect or testify. Returns true iff a and b carry the same elements
// in the same order.
func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
