package finalizecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

// cleanupOutcome is the success-or-failure bit the orchestration layer
// passes to (*cleanupStack).Run. Conditional cleanups (stash pop,
// original-branch restore) branch on this so failure paths leave the
// working tree in a useful state for the user to inspect or resume.
type cleanupOutcome int

const (
	// outcomeSuccess fires every cleanup, including stash-pop and
	// original-branch restore. Used on a clean finalize completion.
	outcomeSuccess cleanupOutcome = iota
	// outcomeAborted fires unconditional cleanups (e.g. jamsesh remote
	// removal) but skips the conditional ones — the user needs the
	// stash and the partial target branch to recover.
	outcomeAborted
)

// cleanupTask is a single entry in the stack. The name is purely
// diagnostic — it surfaces on the warning line if the cleanup errors.
// conditional=true means "only run on outcomeSuccess".
type cleanupTask struct {
	name        string
	fn          func() error
	conditional bool
}

// cleanupStack is a LIFO list of teardown funcs registered during a
// finalize-run flow. The orchestrator calls Run when the main goroutine
// is ready to exit; a background watcher goroutine drains the same
// stack if the root context is cancelled (SIGINT).
//
// Idempotency contract: every registered cleanup must be safe to call
// twice in any order. The HTTPS-remote removal, for example, swallows
// the "no such remote" error so the SIGINT path and the main path can
// both fire it harmlessly.
//
// The stack is goroutine-safe: Push and Run can be called from either
// the main flow or the watcher goroutine without external locking.
type cleanupStack struct {
	mu       sync.Mutex
	tasks    []cleanupTask
	out      io.Writer
	drained  bool      // true once Run has executed
	doneCh   chan struct{}
	watching bool
}

// newCleanupStack returns a stack wired to the given context and writer.
// A background goroutine watches ctx.Done(); on cancellation it drains
// the stack with outcomeAborted. The caller should still invoke Run
// itself on the main exit path — both invocations are safe because the
// stack tracks "drained" and short-circuits the second pass.
//
// out is the writer the stack uses for cleanup-warning messages; pass
// the same os.Stdout the main flow uses so warnings appear inline with
// the verbose git log.
func newCleanupStack(ctx context.Context, out io.Writer) *cleanupStack {
	if out == nil {
		out = io.Discard
	}
	c := &cleanupStack{
		out:    out,
		doneCh: make(chan struct{}),
	}
	if ctx != nil {
		c.watching = true
		go c.watch(ctx)
	}
	return c
}

// watch listens on ctx.Done() and drains the stack with outcomeAborted
// when the context is cancelled (typically a SIGINT propagated by the
// root signal.NotifyContext in cmd/jamsesh/main.go). The goroutine also
// exits if the main flow signals via doneCh that it has run the stack
// itself, so we don't accumulate live goroutines across multiple
// invocations.
func (c *cleanupStack) watch(ctx context.Context) {
	select {
	case <-ctx.Done():
		c.Run(outcomeAborted)
	case <-c.doneCh:
		// Main flow drained the stack normally; nothing for the
		// watcher to do.
	}
}

// Push appends a cleanup task to the stack. conditional=true marks the
// task as success-only — it runs only when Run is called with
// outcomeSuccess. Use this for stash-pop and the original-branch
// restore.
//
// Pushing onto a stack that has already been drained is a no-op: the
// caller likely raced a SIGINT and the stack is closed.
func (c *cleanupStack) Push(name string, conditional bool, fn func() error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.drained {
		return
	}
	c.tasks = append(c.tasks, cleanupTask{name: name, fn: fn, conditional: conditional})
}

// Run drains the stack in LIFO order, returning a joined error of all
// non-nil cleanup failures. Each failure is also logged inline to the
// stack's writer so the user sees what went wrong even if the caller
// discards the error (cleanups are best-effort relative to the primary
// flow error).
//
// Run is idempotent: a second call drains nothing and returns nil. This
// is important because both the main flow and the SIGINT watcher may
// call Run; whichever fires first wins and the loser is a no-op.
//
// On the first invocation Run also closes doneCh so the watcher
// goroutine exits cleanly.
func (c *cleanupStack) Run(outcome cleanupOutcome) error {
	c.mu.Lock()
	if c.drained {
		c.mu.Unlock()
		return nil
	}
	c.drained = true
	tasks := c.tasks
	c.tasks = nil
	// Signal the watcher before we release the lock so it does not
	// double-fire.
	if c.watching {
		// Guard against double-close in the (impossible by construction
		// but defensible) case where Run is somehow re-entered.
		select {
		case <-c.doneCh:
		default:
			close(c.doneCh)
		}
	}
	c.mu.Unlock()

	var errs []error
	// LIFO drain.
	for i := len(tasks) - 1; i >= 0; i-- {
		t := tasks[i]
		if t.conditional && outcome != outcomeSuccess {
			continue
		}
		if err := t.fn(); err != nil {
			fmt.Fprintf(c.out, "Warning: cleanup %q failed: %v\n", t.name, err)
			errs = append(errs, fmt.Errorf("%s: %w", t.name, err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
