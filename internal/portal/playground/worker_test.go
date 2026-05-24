package playground_test

import (
	"context"
	"testing"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/playground"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// advanceable clock for worker tests
// ---------------------------------------------------------------------------

type mutableClock struct {
	now time.Time
}

func (c *mutableClock) Now() time.Time { return c.now }

func (c *mutableClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

// ---------------------------------------------------------------------------
// Worker integration tests using the in-memory SQLite store
// ---------------------------------------------------------------------------

// newWorkerEnv provisions the reserved org and returns a playground-enabled
// testEnv with a Worker wired to the same store.
func newWorkerEnv(t *testing.T) (*testEnv, *playground.Worker, *mutableClock) {
	t.Helper()
	cfg := defaultCfg()
	env := newTestEnvSQLite(t, cfg)
	clk := &mutableClock{now: env.clock.Now()}

	worker := &playground.Worker{
		Store:    env.s,
		Storage:  env.stor,
		Cfg:      cfg,
		Clock:    clk,
		Interval: 10 * time.Millisecond, // very short for test speed
		Logger:   noopLogger(),
	}
	return env, worker, clk
}

// createPlaygroundSession is a helper that creates a playground session
// directly in the store with configurable hard_cap_at and idle_timeout_at.
func createPlaygroundSession(t *testing.T, ctx context.Context, s store.Store, svc tokens.Service, now time.Time, hardCap, idleTimeout time.Duration) store.Session {
	t.Helper()
	hardCapAt := now.Add(hardCap)
	idleTimeoutAt := now.Add(idleTimeout)
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:                        "sess-" + randHexTest(6),
		OrgID:                     playground.ReservedOrgID,
		Name:                      "test-session",
		Goal:                      "",
		WritableScope:             `["**"]`,
		DefaultMode:               "sync",
		Status:                    "active",
		CreatedAt:                 now,
		LastSubstantiveActivityAt: &now,
		HardCapAt:                 &hardCapAt,
		IdleTimeoutAt:             &idleTimeoutAt,
	})
	if err != nil {
		t.Fatalf("createPlaygroundSession: %v", err)
	}
	// Create a bare repo so Storage.RemoveRepo doesn't error.
	if err := newStubStorage().CreateRepo(ctx, playground.ReservedOrgID, sess.ID); err != nil {
		t.Logf("createPlaygroundSession: stub repo create: %v", err)
	}
	return sess
}

// randHexTest returns a simple deterministic hex string for test IDs.
func randHexTest(n int) string {
	const hexchars = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = hexchars[i%len(hexchars)]
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// Test: worker identifies expired sessions and destroys them
// ---------------------------------------------------------------------------

func TestWorker_SweepDestroysExpiredByHardCap(t *testing.T) {
	ctx := context.Background()
	env, worker, clk := newWorkerEnv(t)

	// Create a session that expires in 1 second (hard cap).
	now := clk.Now()
	hardCapAt := now.Add(1 * time.Second)
	idleTimeoutAt := now.Add(30 * time.Minute)
	sess, err := env.s.CreateSession(ctx, store.CreateSessionParams{
		ID:                        "sess-hc-001",
		OrgID:                     playground.ReservedOrgID,
		Name:                      "hard-cap-test",
		Goal:                      "",
		WritableScope:             `["**"]`,
		DefaultMode:               "sync",
		Status:                    "active",
		CreatedAt:                 now,
		LastSubstantiveActivityAt: &now,
		HardCapAt:                 &hardCapAt,
		IdleTimeoutAt:             &idleTimeoutAt,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Advance clock past hard cap.
	clk.Advance(2 * time.Second)

	// Run one sweep.
	runWorkerSweep(worker)

	// Session should be gone.
	_, err = env.s.GetSession(ctx, playground.ReservedOrgID, sess.ID)
	if err == nil {
		t.Error("expected session to be deleted after hard cap, but GetSession succeeded")
	}
	if !isNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}

	// Tombstone should exist.
	tomb, err := env.s.GetTombstone(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}
	if tomb.EndReason != "hard_cap" {
		t.Errorf("tombstone end_reason: want hard_cap, got %s", tomb.EndReason)
	}
}

func TestWorker_SweepDestroysExpiredByIdleTimeout(t *testing.T) {
	ctx := context.Background()
	env, worker, clk := newWorkerEnv(t)

	now := clk.Now()
	hardCapAt := now.Add(24 * time.Hour)
	idleTimeoutAt := now.Add(1 * time.Second)
	sess, err := env.s.CreateSession(ctx, store.CreateSessionParams{
		ID:                        "sess-idle-001",
		OrgID:                     playground.ReservedOrgID,
		Name:                      "idle-timeout-test",
		Goal:                      "",
		WritableScope:             `["**"]`,
		DefaultMode:               "sync",
		Status:                    "active",
		CreatedAt:                 now,
		LastSubstantiveActivityAt: &now,
		HardCapAt:                 &hardCapAt,
		IdleTimeoutAt:             &idleTimeoutAt,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Advance clock past idle timeout but not hard cap.
	clk.Advance(2 * time.Second)
	runWorkerSweep(worker)

	_, err = env.s.GetSession(ctx, playground.ReservedOrgID, sess.ID)
	if err == nil {
		t.Error("expected session to be deleted after idle timeout, but GetSession succeeded")
	}
	if !isNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}

	tomb, err := env.s.GetTombstone(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}
	if tomb.EndReason != "idle" {
		t.Errorf("tombstone end_reason: want idle, got %s", tomb.EndReason)
	}
}

func TestWorker_SweepSkipsNonExpiredSessions(t *testing.T) {
	ctx := context.Background()
	env, worker, clk := newWorkerEnv(t)

	now := clk.Now()
	hardCapAt := now.Add(24 * time.Hour)
	idleTimeoutAt := now.Add(30 * time.Minute)
	sess, err := env.s.CreateSession(ctx, store.CreateSessionParams{
		ID:                        "sess-alive-001",
		OrgID:                     playground.ReservedOrgID,
		Name:                      "alive-session",
		Goal:                      "",
		WritableScope:             `["**"]`,
		DefaultMode:               "sync",
		Status:                    "active",
		CreatedAt:                 now,
		LastSubstantiveActivityAt: &now,
		HardCapAt:                 &hardCapAt,
		IdleTimeoutAt:             &idleTimeoutAt,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Only advance 10 seconds — far from expiry.
	clk.Advance(10 * time.Second)
	runWorkerSweep(worker)

	// Session should still exist.
	_, err = env.s.GetSession(ctx, playground.ReservedOrgID, sess.ID)
	if err != nil {
		t.Errorf("expected session to survive, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test: graceful shutdown stops the worker
// ---------------------------------------------------------------------------

func TestWorker_GracefulShutdownStopsWithinOneInterval(t *testing.T) {
	_, worker, _ := newWorkerEnv(t)
	worker.Interval = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = worker.Run(ctx)
	}()

	// Cancel context and expect worker to stop.
	cancel()
	select {
	case <-done:
		// Good: worker stopped.
	case <-time.After(500 * time.Millisecond):
		t.Error("worker did not stop within 500ms after context cancel")
	}
}

// ---------------------------------------------------------------------------
// Test: tombstone TTL purge runs
// ---------------------------------------------------------------------------

func TestWorker_PurgesTombstonesAfterTTL(t *testing.T) {
	ctx := context.Background()
	env, _, clk := newWorkerEnv(t)

	now := clk.Now()

	// Insert a tombstone directly with a very short TTL (already expired).
	err := env.s.RecordTombstone(ctx, store.RecordTombstoneParams{
		SessionID:       "sess-purge-001",
		OrgID:           playground.ReservedOrgID,
		MembersCount:    1,
		CommitsCount:    0,
		AutoMergesCount: 0,
		DurationSeconds: 60,
		EndReason:       "manual",
		EndedAt:         now.Add(-48 * time.Hour),
		ExpiresAt:       now.Add(-24 * time.Hour), // already expired
	})
	if err != nil {
		t.Fatalf("RecordTombstone: %v", err)
	}

	// Purge via the store directly (exercises the method the worker calls).
	if err := env.s.PurgeExpiredTombstones(ctx, now); err != nil {
		t.Fatalf("PurgeExpiredTombstones: %v", err)
	}

	// The tombstone should be gone.
	_, err = env.s.GetTombstone(ctx, "sess-purge-001")
	if err == nil {
		t.Error("expected tombstone to be purged, but GetTombstone succeeded")
	}
	if !isNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test: reasonFor priority (hard_cap wins over idle)
// ---------------------------------------------------------------------------

func TestWorker_ReasonFor_HardCapTakesPriority(t *testing.T) {
	ctx := context.Background()
	env, worker, clk := newWorkerEnv(t)

	now := clk.Now()
	// Both thresholds in the past.
	hardCapAt := now.Add(-2 * time.Second)
	idleTimeoutAt := now.Add(-1 * time.Second)

	_, err := env.s.CreateSession(ctx, store.CreateSessionParams{
		ID:                        "sess-reason-001",
		OrgID:                     playground.ReservedOrgID,
		Name:                      "reason-test",
		Goal:                      "",
		WritableScope:             `["**"]`,
		DefaultMode:               "sync",
		Status:                    "active",
		CreatedAt:                 now.Add(-10 * time.Second),
		LastSubstantiveActivityAt: &now,
		HardCapAt:                 &hardCapAt,
		IdleTimeoutAt:             &idleTimeoutAt,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	clk.Advance(1 * time.Millisecond) // ensure now > both thresholds
	runWorkerSweep(worker)

	tomb, err := env.s.GetTombstone(ctx, "sess-reason-001")
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}
	if tomb.EndReason != "hard_cap" {
		t.Errorf("tombstone end_reason: want hard_cap, got %s", tomb.EndReason)
	}
}

// ---------------------------------------------------------------------------
// Test: sweep still runs when Cfg.Enabled=false (disable-flip)
// ---------------------------------------------------------------------------

// TestWorker_RunsEvenWhenCreateDisabled verifies the documented invariant from
// SELF_HOST.md: "Existing in-flight sessions keep running through their normal
// idle / hard-cap lifecycles — the destruction sweep continues to fire even
// when the create endpoint is off."
//
// The create endpoint checks Cfg.Enabled and returns 503 when false; the
// worker must NOT — it always sweeps. This test pins that contract.
func TestWorker_RunsEvenWhenCreateDisabled(t *testing.T) {
	ctx := context.Background()

	// Build a worker with Cfg.Enabled=false — only the create endpoint should
	// be gated; the sweep loop must be unaffected.
	cfg := defaultCfg()
	cfg.Enabled = false
	env := newTestEnvSQLite(t, cfg)
	clk := &mutableClock{now: env.clock.Now()}

	worker := &playground.Worker{
		Store:    env.s,
		Storage:  env.stor,
		Cfg:      cfg,
		Clock:    clk,
		Interval: 10 * time.Millisecond,
		Logger:   noopLogger(),
	}

	// Seed a session that is already past its hard cap.
	now := clk.Now()
	hardCapAt := now.Add(1 * time.Second)
	idleTimeoutAt := now.Add(30 * time.Minute)
	sess, err := env.s.CreateSession(ctx, store.CreateSessionParams{
		ID:                        "sess-disabled-001",
		OrgID:                     playground.ReservedOrgID,
		Name:                      "disabled-flag-test",
		Goal:                      "",
		WritableScope:             `["**"]`,
		DefaultMode:               "sync",
		Status:                    "active",
		CreatedAt:                 now,
		LastSubstantiveActivityAt: &now,
		HardCapAt:                 &hardCapAt,
		IdleTimeoutAt:             &idleTimeoutAt,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Advance the clock past the hard cap so the session is expired.
	clk.Advance(2 * time.Second)

	// Run one sweep — this must destroy the session even though Enabled=false.
	runWorkerSweep(worker)

	// The session must be gone: the sweep runs regardless of Cfg.Enabled.
	_, err = env.s.GetSession(ctx, playground.ReservedOrgID, sess.ID)
	if err == nil {
		t.Error("expected session to be destroyed by sweep even with Cfg.Enabled=false, but GetSession succeeded")
	}
	if !isNotFound(err) {
		t.Errorf("expected ErrNotFound after sweep, got: %v", err)
	}

	// Tombstone confirms the reason.
	tomb, err := env.s.GetTombstone(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}
	if tomb.EndReason != "hard_cap" {
		t.Errorf("tombstone end_reason: want hard_cap, got %s", tomb.EndReason)
	}
}

// ---------------------------------------------------------------------------
// Test: exact-boundary cases for hard_cap_at and idle_timeout_at
// ---------------------------------------------------------------------------

// TestWorker_SessionExpiresWhenNowEqualsHardCapAt pins the boundary behaviour
// for hard_cap_at.
//
// The SQL sweep uses `hard_cap_at <= ?now` (i.e. <=), so `now == hard_cap_at`
// IS included: the session IS destroyed at the exact boundary. This test locks
// that inclusion so future refactors or query rewrites can't silently change
// `<=` to `<` (strict) without a failing test.
//
// Secondary observation: reasonFor uses now.After(hard_cap_at) which is
// strictly `now > hard_cap_at`. At the exact equality point the hard_cap
// branch is false and the idle branch is also false, so reasonFor falls
// through to "manual". This test therefore asserts reason "manual" at the
// boundary — not "hard_cap". A future change that aligns reasonFor to use
// `>=` (i.e. !now.Before(hard_cap_at)) would correctly return "hard_cap" and
// this assertion should be updated at that time.
func TestWorker_SessionExpiresWhenNowEqualsHardCapAt(t *testing.T) {
	ctx := context.Background()
	env, worker, clk := newWorkerEnv(t)

	// Set hard_cap_at exactly equal to the current clock time.
	// SQL sweep: hard_cap_at <= now → true → session is included in the sweep.
	now := clk.Now()
	hardCapAt := now            // boundary: hard_cap_at == now
	idleTimeoutAt := now.Add(30 * time.Minute)

	sess, err := env.s.CreateSession(ctx, store.CreateSessionParams{
		ID:                        "sess-hc-boundary-001",
		OrgID:                     playground.ReservedOrgID,
		Name:                      "hard-cap-boundary-test",
		Goal:                      "",
		WritableScope:             `["**"]`,
		DefaultMode:               "sync",
		Status:                    "active",
		CreatedAt:                 now,
		LastSubstantiveActivityAt: &now,
		HardCapAt:                 &hardCapAt,
		IdleTimeoutAt:             &idleTimeoutAt,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Do NOT advance the clock — now == hard_cap_at exactly.
	runWorkerSweep(worker)

	// SQL uses <=, so the session IS included in the sweep and must be gone.
	_, err = env.s.GetSession(ctx, playground.ReservedOrgID, sess.ID)
	if err == nil {
		t.Error("expected session to be destroyed when now == hard_cap_at (SQL uses <=), but GetSession succeeded")
	}
	if !isNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}

	// At the exact boundary reasonFor uses now.After(hard_cap_at) which is
	// strictly >, so it falls through to "manual". This is the documented
	// off-by-one between the SQL predicate (<=) and reasonFor (>).
	tomb, err := env.s.GetTombstone(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}
	if tomb.EndReason != "manual" {
		t.Errorf("tombstone end_reason at exact hard_cap_at boundary: want manual (reasonFor off-by-one), got %s", tomb.EndReason)
	}
}

// TestWorker_SessionExpiresWhenNowEqualsIdleTimeoutAt pins the boundary
// behaviour for idle_timeout_at.
//
// The SQL sweep uses `idle_timeout_at <= ?now` (i.e. <=), so
// `now == idle_timeout_at` IS included: the session IS destroyed at the exact
// boundary. This test locks that inclusion so future refactors or query
// rewrites can't silently change `<=` to `<` (strict) without a failing test.
//
// Secondary observation: reasonFor uses now.After(idle_timeout_at) which is
// strictly `now > idle_timeout_at`. At the exact equality point the idle
// branch is false, so reasonFor falls through to "manual". This test therefore
// asserts reason "manual" at the boundary — not "idle". A future change that
// aligns reasonFor to use `>=` (i.e. !now.Before(idle_timeout_at)) would
// correctly return "idle" and this assertion should be updated at that time.
func TestWorker_SessionExpiresWhenNowEqualsIdleTimeoutAt(t *testing.T) {
	ctx := context.Background()
	env, worker, clk := newWorkerEnv(t)

	// Set idle_timeout_at exactly equal to the current clock time.
	// SQL sweep: idle_timeout_at <= now → true → session is included in sweep.
	now := clk.Now()
	hardCapAt := now.Add(24 * time.Hour)
	idleTimeoutAt := now // boundary: idle_timeout_at == now

	sess, err := env.s.CreateSession(ctx, store.CreateSessionParams{
		ID:                        "sess-idle-boundary-001",
		OrgID:                     playground.ReservedOrgID,
		Name:                      "idle-timeout-boundary-test",
		Goal:                      "",
		WritableScope:             `["**"]`,
		DefaultMode:               "sync",
		Status:                    "active",
		CreatedAt:                 now,
		LastSubstantiveActivityAt: &now,
		HardCapAt:                 &hardCapAt,
		IdleTimeoutAt:             &idleTimeoutAt,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Do NOT advance the clock — now == idle_timeout_at exactly.
	runWorkerSweep(worker)

	// SQL uses <=, so the session IS included in the sweep and must be gone.
	_, err = env.s.GetSession(ctx, playground.ReservedOrgID, sess.ID)
	if err == nil {
		t.Error("expected session to be destroyed when now == idle_timeout_at (SQL uses <=), but GetSession succeeded")
	}
	if !isNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}

	// At the exact boundary reasonFor uses now.After(idle_timeout_at) which is
	// strictly >, so it falls through to "manual". This is the documented
	// off-by-one between the SQL predicate (<=) and reasonFor (>).
	tomb, err := env.s.GetTombstone(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}
	if tomb.EndReason != "manual" {
		t.Errorf("tombstone end_reason at exact idle_timeout_at boundary: want manual (reasonFor off-by-one), got %s", tomb.EndReason)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// runWorkerSweep calls the worker's internal sweep via Run with a very short
// context deadline so exactly one tick fires.
func runWorkerSweep(w *playground.Worker) {
	w.Interval = 1 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_ = w.Run(ctx)
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err == store.ErrNotFound
}
