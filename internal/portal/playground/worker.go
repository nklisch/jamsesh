package playground

import (
	"context"
	"log/slog"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/lease"
	"jamsesh/internal/portal/storage"
)

// Worker is the background subsystem that sweeps active playground sessions on
// every tick and destroys expired ones. A session is expired when:
//
//   - now > hard_cap_at   (wall-clock cap exceeded)
//   - now > idle_timeout_at (no substantive activity for IdleTimeout duration)
//
// Worker is safe to embed in a WaitGroup-based graceful-shutdown pattern:
//
//	worker := &playground.Worker{...}
//	wg.Add(1)
//	go func() {
//	    defer wg.Done()
//	    if err := worker.Run(workerCtx); err != nil && !errors.Is(err, context.Canceled) {
//	        logger.Error("playground worker exited", "err", err)
//	    }
//	}()
// workerStore is the minimal store interface consumed by Worker (and transitively
// by Destruction, which Worker constructs from the same store).
type workerStore interface {
	destructionStore
	store.PlaygroundSessionStore
}

type Worker struct {
	Store    workerStore
	Storage  storage.Service
	Cfg      Config
	Clock    Clock
	Interval time.Duration // default 60s when zero
	Logger   *slog.Logger
	Leases   lease.Manager // optional; nil → NoopManager (single-instance)

	// destruction is wired after construction so tests can inject a stub.
	// If nil, Run() initialises a real Destruction using the same Store/Storage.
	destruction *Destruction
}

// Run loops until ctx is cancelled. Each tick calls sweep(ctx). Graceful
// shutdown: a ctx cancellation stops the ticker loop; any in-flight sweep()
// call runs to completion before Run returns.
func (w *Worker) Run(ctx context.Context) error {
	if w.Interval == 0 {
		w.Interval = 60 * time.Second
	}
	if w.Logger == nil {
		w.Logger = slog.Default()
	}
	if w.destruction == nil {
		w.destruction = &Destruction{
			Store:        w.Store,
			Storage:      w.Storage,
			Clock:        w.Clock,
			Logger:       w.Logger,
			TombstoneTTL: 30 * 24 * time.Hour, // 30-day default
			Leases:       w.Leases,             // nil → NoopManager inside Destruction
		}
	}

	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	// purgeEvery controls how many sweep ticks pass between tombstone TTL
	// purges. At 60s interval, purgeEvery=60 means purge runs ~once per hour.
	const purgeEvery = 60
	tick := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			tick++
			w.sweep(ctx)
			if tick%purgeEvery == 0 {
				w.purgeTombstones(ctx)
			}
		}
	}
}

// sweep identifies expired playground sessions and runs Destruction.Destroy for
// each one. Errors within a single session's destruction are logged but do NOT
// abort the loop — the remaining sessions are always processed, and the next
// tick will retry any partially-destroyed sessions (each step is idempotent).
func (w *Worker) sweep(ctx context.Context) {
	now := w.Clock.Now().UTC()
	expired, err := w.Store.ListExpiredPlaygroundSessions(ctx, store.ListExpiredPlaygroundSessionsParams{
		OrgID: ReservedOrgID,
		Now:   now,
	})
	if err != nil {
		w.Logger.Error("playground sweep: list expired sessions failed", "err", err)
		return
	}
	if len(expired) == 0 {
		return
	}
	w.Logger.Info("playground sweep: destroying expired sessions", "count", len(expired))
	for _, sess := range expired {
		reason := w.reasonFor(sess, now)
		if err := w.destruction.Destroy(ctx, sess, reason); err != nil {
			// Log and continue: next tick will retry this session.
			// Errors from individual steps are already logged inside Destroy.
			w.Logger.Error("playground sweep: destroy failed",
				"session_id", sess.ID, "reason", reason, "err", err)
		}
	}
}

// purgeTombstones deletes tombstone rows whose expires_at has elapsed. Called
// periodically inside the sweep loop so no separate goroutine is required.
func (w *Worker) purgeTombstones(ctx context.Context) {
	now := w.Clock.Now().UTC()
	if err := w.Store.PurgeExpiredTombstones(ctx, now); err != nil {
		w.Logger.Error("playground sweep: tombstone purge failed", "err", err)
	}
}

// reasonFor determines the end_reason string for a session based on which
// threshold has elapsed. Hard cap takes priority when both are past.
//
// Uses !now.Before (i.e. now >= threshold) rather than now.After (now >
// threshold) so the tombstone reason is correct at the EXACT boundary
// instant. The SQL sweep predicate is `hard_cap_at <= ?` / `idle_timeout_at
// <= ?`; matching that predicate's inclusivity here keeps the tombstone
// reason consistent at the boundary.
// (bug-playground-worker-reasonFor-off-by-one-at-exact-boundary)
func (w *Worker) reasonFor(sess store.Session, now time.Time) string {
	if sess.HardCapAt != nil && !now.Before(*sess.HardCapAt) {
		return "hard_cap"
	}
	if sess.IdleTimeoutAt != nil && !now.Before(*sess.IdleTimeoutAt) {
		return "idle"
	}
	// Shouldn't happen — the sweep query only returns sessions where at least
	// one threshold has elapsed. Treat as manual to avoid silently skipping.
	return "manual"
}

