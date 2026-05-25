package playground_test

import (
	"context"
	"testing"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/playground"
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
// reasonFor uses !now.Before(hard_cap_at) (i.e. now >= hard_cap_at) so the
// boundary returns "hard_cap" — matching the SQL predicate's inclusivity.
// (bug-playground-worker-reasonFor-off-by-one-at-exact-boundary)
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

	// reasonFor uses !now.Before(hard_cap_at) — matches the SQL <= predicate.
	// At the exact boundary the reason is "hard_cap".
	tomb, err := env.s.GetTombstone(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}
	if tomb.EndReason != "hard_cap" {
		t.Errorf("tombstone end_reason at exact hard_cap_at boundary: want hard_cap, got %s", tomb.EndReason)
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
// reasonFor uses !now.Before(idle_timeout_at) — same inclusivity as the SQL
// predicate — so the reason at the exact boundary is "idle".
// (bug-playground-worker-reasonFor-off-by-one-at-exact-boundary)
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

	// reasonFor uses !now.Before(idle_timeout_at) — matches the SQL <=
	// predicate. At the exact boundary the reason is "idle".
	tomb, err := env.s.GetTombstone(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}
	if tomb.EndReason != "idle" {
		t.Errorf("tombstone end_reason at exact idle_timeout_at boundary: want idle, got %s", tomb.EndReason)
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

// ---------------------------------------------------------------------------
// Story: gate-tests-tombstone-purge-cadence-tick-bound-vs-wallclock
//
// purgeEvery = 60 is a tick-count constant, not a wall-clock interval.
// These tests document that fact so a future refactor cannot silently
// change the semantics (e.g. from tick-count to elapsed-time comparison)
// without breaking a test.
// ---------------------------------------------------------------------------

// purgeCountStore wraps a real store and counts calls to PurgeExpiredTombstones.
//
// Implements workerStore (destructionStore + PlaygroundSessionStore =
// SessionStore + SessionMemberStore + PlaygroundSessionStore + TombstoneStore +
// OAuthTokenStore), delegating all methods except PurgeExpiredTombstones.
type purgeCountStore struct {
	realStore store.Store
	count     int
}

// SessionStore delegation
func (s *purgeCountStore) CreateSession(ctx context.Context, p store.CreateSessionParams) (store.Session, error) {
	return s.realStore.CreateSession(ctx, p)
}
func (s *purgeCountStore) GetSession(ctx context.Context, orgID, id string) (store.Session, error) {
	return s.realStore.GetSession(ctx, orgID, id)
}
func (s *purgeCountStore) GetSessionByID(ctx context.Context, id string) (store.Session, error) {
	return s.realStore.GetSessionByID(ctx, id)
}
func (s *purgeCountStore) ListSessionsForOrg(ctx context.Context, orgID string) ([]store.Session, error) {
	return s.realStore.ListSessionsForOrg(ctx, orgID)
}
func (s *purgeCountStore) ListSessionsForOrgWithCursor(ctx context.Context, p store.ListSessionsForOrgWithCursorParams) ([]store.Session, error) {
	return s.realStore.ListSessionsForOrgWithCursor(ctx, p)
}
func (s *purgeCountStore) UpdateSessionStatus(ctx context.Context, p store.UpdateSessionStatusParams) error {
	return s.realStore.UpdateSessionStatus(ctx, p)
}
func (s *purgeCountStore) UpdateSessionGoalScopeMode(ctx context.Context, p store.UpdateSessionGoalScopeModeParams) error {
	return s.realStore.UpdateSessionGoalScopeMode(ctx, p)
}
func (s *purgeCountStore) SetSessionBaseSHA(ctx context.Context, p store.SetSessionBaseSHAParams) error {
	return s.realStore.SetSessionBaseSHA(ctx, p)
}
func (s *purgeCountStore) SetSessionEndReason(ctx context.Context, p store.SetSessionEndReasonParams) error {
	return s.realStore.SetSessionEndReason(ctx, p)
}
func (s *purgeCountStore) SetFinalizeLock(ctx context.Context, p store.SetFinalizeLockParams) error {
	return s.realStore.SetFinalizeLock(ctx, p)
}
func (s *purgeCountStore) ClearFinalizeLock(ctx context.Context, p store.ClearFinalizeLockParams) error {
	return s.realStore.ClearFinalizeLock(ctx, p)
}
func (s *purgeCountStore) DeleteSession(ctx context.Context, p store.DeleteSessionParams) error {
	return s.realStore.DeleteSession(ctx, p)
}

// SessionMemberStore delegation
func (s *purgeCountStore) AddSessionMember(ctx context.Context, p store.AddSessionMemberParams) error {
	return s.realStore.AddSessionMember(ctx, p)
}
func (s *purgeCountStore) GetSessionMember(ctx context.Context, p store.GetSessionMemberParams) (store.SessionMember, error) {
	return s.realStore.GetSessionMember(ctx, p)
}
func (s *purgeCountStore) ListSessionMembers(ctx context.Context, p store.ListSessionMembersParams) ([]store.SessionMember, error) {
	return s.realStore.ListSessionMembers(ctx, p)
}
func (s *purgeCountStore) RemoveSessionMember(ctx context.Context, p store.RemoveSessionMemberParams) error {
	return s.realStore.RemoveSessionMember(ctx, p)
}
func (s *purgeCountStore) ListSessionMembershipsForAccount(ctx context.Context, accountID string) ([]store.SessionMembership, error) {
	return s.realStore.ListSessionMembershipsForAccount(ctx, accountID)
}
func (s *purgeCountStore) NicknameTakenInSession(ctx context.Context, p store.NicknameTakenInSessionParams) (bool, error) {
	return s.realStore.NicknameTakenInSession(ctx, p)
}
func (s *purgeCountStore) CountSessionMembers(ctx context.Context, p store.CountSessionMembersParams) (int64, error) {
	return s.realStore.CountSessionMembers(ctx, p)
}

// PlaygroundSessionStore delegation (PurgeExpiredTombstones overridden below)
func (s *purgeCountStore) ResetSessionIdleTimer(ctx context.Context, p store.ResetSessionIdleTimerParams) error {
	return s.realStore.ResetSessionIdleTimer(ctx, p)
}
func (s *purgeCountStore) ListExpiredPlaygroundSessions(ctx context.Context, p store.ListExpiredPlaygroundSessionsParams) ([]store.Session, error) {
	return s.realStore.ListExpiredPlaygroundSessions(ctx, p)
}
func (s *purgeCountStore) PurgeExpiredTombstones(ctx context.Context, before time.Time) error {
	s.count++
	return s.realStore.PurgeExpiredTombstones(ctx, before)
}
func (s *purgeCountStore) ListAnonymousSessionMemberIDs(ctx context.Context, orgID, sessID string) ([]string, error) {
	return s.realStore.ListAnonymousSessionMemberIDs(ctx, orgID, sessID)
}
func (s *purgeCountStore) DeleteAccountsByIDs(ctx context.Context, ids []string) error {
	return s.realStore.DeleteAccountsByIDs(ctx, ids)
}
func (s *purgeCountStore) CountSessionEventsByType(ctx context.Context, orgID, eventType string) (int64, error) {
	return s.realStore.CountSessionEventsByType(ctx, orgID, eventType)
}

// TombstoneStore delegation
func (s *purgeCountStore) GetTombstone(ctx context.Context, sessionID string) (store.Tombstone, error) {
	return s.realStore.GetTombstone(ctx, sessionID)
}
func (s *purgeCountStore) RecordTombstone(ctx context.Context, p store.RecordTombstoneParams) error {
	return s.realStore.RecordTombstone(ctx, p)
}

// OAuthTokenStore delegation
func (s *purgeCountStore) CreateOAuthToken(ctx context.Context, p store.CreateOAuthTokenParams) (store.OAuthToken, error) {
	return s.realStore.CreateOAuthToken(ctx, p)
}
func (s *purgeCountStore) CreateAnonymousBearer(ctx context.Context, p store.CreateAnonymousBearerParams) (store.OAuthToken, error) {
	return s.realStore.CreateAnonymousBearer(ctx, p)
}
func (s *purgeCountStore) RevokeBearersForSession(ctx context.Context, p store.RevokeBearersForSessionParams) error {
	return s.realStore.RevokeBearersForSession(ctx, p)
}
func (s *purgeCountStore) GetOAuthTokenByHash(ctx context.Context, h string) (store.OAuthToken, error) {
	return s.realStore.GetOAuthTokenByHash(ctx, h)
}
func (s *purgeCountStore) TouchOAuthTokenLastUsed(ctx context.Context, p store.TouchOAuthTokenLastUsedParams) error {
	return s.realStore.TouchOAuthTokenLastUsed(ctx, p)
}
func (s *purgeCountStore) RevokeOAuthToken(ctx context.Context, p store.RevokeOAuthTokenParams) error {
	return s.realStore.RevokeOAuthToken(ctx, p)
}
func (s *purgeCountStore) RevokeAllOAuthTokensForAccount(ctx context.Context, p store.RevokeAllOAuthTokensForAccountParams) error {
	return s.realStore.RevokeAllOAuthTokensForAccount(ctx, p)
}
func (s *purgeCountStore) ListOAuthTokensForAccount(ctx context.Context, accountID string) ([]store.OAuthToken, error) {
	return s.realStore.ListOAuthTokensForAccount(ctx, accountID)
}

// TestWorker_PurgeCadence_IsTickBound_Not_WallClockBound documents that tombstone
// purge fires every 60 ticks regardless of how much wall-clock time has elapsed.
//
// Tick-bound semantics: purge fires when tick%60==0 — only once every 60 sweep
// cycles. At the default 60-second sweep interval that is ~once per hour; at a
// shortened test interval (1ms) it is still once per 60 ticks.
//
// The two sub-tests pin the contract from both sides:
//   - After <60 ticks the purge must NOT have fired (count == 0).
//   - After >=60 ticks the purge MUST have fired (count >= 1).
func TestWorker_PurgeCadence_IsTickBound_Not_WallClockBound(t *testing.T) {
	const purgeEvery = 60 // must match the private const in worker.go

	t.Run("no purge before 60 ticks regardless of elapsed time", func(t *testing.T) {
		env, _, clk := newWorkerEnv(t)

		counted := &purgeCountStore{realStore: env.s}
		worker := &playground.Worker{
			Store:    counted,
			Storage:  env.stor,
			Cfg:      defaultCfg(),
			Clock:    clk,
			Interval: 1 * time.Millisecond,
			Logger:   noopLogger(),
		}

		// Run for at most (purgeEvery-1) = 59 ticks. Use a tight context that
		// allows well under 60 ticks (30ms at 1ms interval → ~30 ticks max).
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		_ = worker.Run(ctx)

		// At fewer than purgeEvery ticks, PurgeExpiredTombstones must never fire.
		// This proves the cadence is tick-gated, not triggered by elapsed time.
		if counted.count != 0 {
			t.Errorf("purge fired %d time(s) in <60 ticks; expected 0 (tick-bound gate not respected)", counted.count)
		}
	})

	t.Run("purge fires at least once after >=60 ticks", func(t *testing.T) {
		env, _, clk := newWorkerEnv(t)

		counted := &purgeCountStore{realStore: env.s}
		worker := &playground.Worker{
			Store:    counted,
			Storage:  env.stor,
			Cfg:      defaultCfg(),
			Clock:    clk,
			Interval: 1 * time.Millisecond,
			Logger:   noopLogger(),
		}

		// Run for long enough to accumulate >=60 ticks at 1ms/tick.
		// 200ms gives ~200 ticks — generously above the 60-tick threshold and
		// avoids flakiness on slow CI machines.
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_ = worker.Run(ctx)

		if counted.count < 1 {
			t.Errorf("purge never fired after >=60 ticks; expected count >= 1 (tick-bound gate not triggering)")
		}
	})
}

// ---------------------------------------------------------------------------
// Story: gate-tests-tombstone-purge-via-worker-not-store-tautology
//
// TestWorker_PurgesTombstones_OnPurgeEveryTickInterval replaces the existing
// TestWorker_PurgesTombstonesAfterTTL which calls the store layer directly
// (tautological: always proves the store works, never proves the worker fires
// the purge on cadence).
//
// This test drives the full worker.Run() path: it seeds an expired tombstone,
// lets the worker run for >=60 ticks (the purgeEvery threshold), and then
// verifies the tombstone is gone — proving that the worker actually invokes
// PurgeExpiredTombstones on the configured tick cadence.
// ---------------------------------------------------------------------------

func TestWorker_PurgesTombstones_OnPurgeEveryTickInterval(t *testing.T) {
	ctx := context.Background()
	env, _, clk := newWorkerEnv(t)

	now := clk.Now()

	// Seed an expired tombstone: expires_at is already in the past.
	err := env.s.RecordTombstone(ctx, store.RecordTombstoneParams{
		SessionID:       "sess-worker-purge-001",
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

	// Verify the tombstone is present before the worker runs.
	if _, err := env.s.GetTombstone(ctx, "sess-worker-purge-001"); err != nil {
		t.Fatalf("tombstone should exist before worker run: %v", err)
	}

	// Wire a worker against the same store. Run for 200ms at 1ms/tick —
	// this guarantees >60 ticks (the purgeEvery threshold) even on slow CI.
	worker := &playground.Worker{
		Store:    env.s,
		Storage:  env.stor,
		Cfg:      defaultCfg(),
		Clock:    clk,
		Interval: 1 * time.Millisecond,
		Logger:   noopLogger(),
	}

	runCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = worker.Run(runCtx)

	// The worker must have called PurgeExpiredTombstones via its internal
	// purgeTombstones() method. The expired tombstone should be gone.
	_, err = env.s.GetTombstone(ctx, "sess-worker-purge-001")
	if err == nil {
		t.Error("expected tombstone to be purged by worker after >=60 ticks, but GetTombstone succeeded")
	}
	if !isNotFound(err) {
		t.Errorf("expected ErrNotFound after worker purge, got: %v", err)
	}
}
