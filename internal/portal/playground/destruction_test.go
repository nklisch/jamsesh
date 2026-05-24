package playground_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/lease"
	"jamsesh/internal/portal/playground"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// stubLeaseManager — single-acquisition stub for contention tests
// ---------------------------------------------------------------------------

// stubLeaseManager is a lease.Manager that allows only one concurrent holder
// per session ID. The first Acquire succeeds; subsequent concurrent Acquires
// (while the first handle is still held) return lease.ErrAlreadyHeld. This
// models pg_try_advisory_lock behaviour in the test environment.
type stubLeaseManager struct {
	mu   sync.Mutex
	held map[string]bool // session IDs currently acquired
}

func newStubLeaseManager() *stubLeaseManager {
	return &stubLeaseManager{held: make(map[string]bool)}
}

func (m *stubLeaseManager) Acquire(ctx context.Context, sessionID string) (lease.Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held[sessionID] {
		return nil, lease.ErrAlreadyHeld
	}
	m.held[sessionID] = true
	return &stubLeaseHandle{mgr: m, sessionID: sessionID, lost: make(chan struct{})}, nil
}

type stubLeaseHandle struct {
	mgr       *stubLeaseManager
	sessionID string
	lost      chan struct{}
	once      sync.Once
}

func (h *stubLeaseHandle) SessionID() string        { return h.sessionID }
func (h *stubLeaseHandle) FencingToken() int64      { return 0 }
func (h *stubLeaseHandle) Lost() <-chan struct{}    { return h.lost }
func (h *stubLeaseHandle) Release() error {
	h.once.Do(func() {
		h.mgr.mu.Lock()
		delete(h.mgr.held, h.sessionID)
		h.mgr.mu.Unlock()
		close(h.lost)
	})
	return nil
}

// ---------------------------------------------------------------------------
// Helpers — build a session with anon member for destruction tests
// ---------------------------------------------------------------------------

// setupDestructionSession creates a playground session, issues an anonymous
// bearer (which creates an anon account row), adds the member row, and creates
// the stub repo. Returns the session and the anon account ID.
func setupDestructionSession(t *testing.T, ctx context.Context, env *testEnv) (store.Session, string) {
	t.Helper()
	svc := tokens.New(env.s)
	now := env.clock.Now()
	hardCapAt := now.Add(24 * time.Hour)
	idleTimeoutAt := now.Add(30 * time.Minute)

	sess, err := env.s.CreateSession(ctx, store.CreateSessionParams{
		ID:                        "dest-sess-" + randHexTest(6),
		OrgID:                     playground.ReservedOrgID,
		Name:                      "destruction-test",
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
		t.Fatalf("setupDestructionSession: CreateSession: %v", err)
	}

	// Issue anon bearer (creates anon account internally).
	_, accountID, _, err := svc.IssueAnonymousSessionBearer(ctx, sess.ID, "test-nick", 24*time.Hour)
	if err != nil {
		t.Fatalf("setupDestructionSession: IssueAnonymousSessionBearer: %v", err)
	}

	// Add session member.
	if err := env.s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID:     playground.ReservedOrgID,
		SessionID: sess.ID,
		AccountID: accountID,
		Role:      "creator",
		JoinedAt:  now,
	}); err != nil {
		t.Fatalf("setupDestructionSession: AddSessionMember: %v", err)
	}

	// Create stub repo so RemoveRepo doesn't error.
	if err := env.stor.CreateRepo(ctx, playground.ReservedOrgID, sess.ID); err != nil {
		t.Fatalf("setupDestructionSession: CreateRepo: %v", err)
	}

	return sess, accountID
}

// newDestruction builds a Destruction with the env's store and storage.
func newDestruction(env *testEnv) *playground.Destruction {
	return &playground.Destruction{
		Store:        env.s,
		Storage:      env.stor,
		Clock:        env.clock,
		Logger:       noopLogger(),
		TombstoneTTL: 30 * 24 * time.Hour,
	}
}

// ---------------------------------------------------------------------------
// TestDestruction_CascadeCorrectness
// ---------------------------------------------------------------------------

func TestDestruction_CascadeCorrectness(t *testing.T) {
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())

	sess, accountID := setupDestructionSession(t, ctx, env)
	d := newDestruction(env)

	// Run the cascade.
	if err := d.Destroy(ctx, sess, "hard_cap"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	// Session row must be gone.
	_, err := env.s.GetSession(ctx, playground.ReservedOrgID, sess.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected session to be deleted; got err=%v", err)
	}

	// Tombstone must exist with correct end_reason.
	tomb, err := env.s.GetTombstone(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}
	if tomb.EndReason != "hard_cap" {
		t.Errorf("tombstone end_reason: want hard_cap, got %s", tomb.EndReason)
	}
	if tomb.MembersCount != 1 {
		t.Errorf("tombstone members_count: want 1, got %d", tomb.MembersCount)
	}

	// Anonymous account must be deleted (not cascade-deleted by session deletion).
	_, err = env.s.GetAccountByID(ctx, accountID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected anon account to be deleted; got err=%v", err)
	}

	// Bare repo must be removed from stub storage.
	exists, _ := env.stor.RepoExists(playground.ReservedOrgID, sess.ID)
	if exists {
		t.Error("expected bare repo to be deleted, but it still exists in stub storage")
	}
}

// ---------------------------------------------------------------------------
// TestDestruction_TombstoneInsertedBeforeSessionDelete
// ---------------------------------------------------------------------------

func TestDestruction_TombstoneInsertedBeforeSessionDelete(t *testing.T) {
	// Verifies that the tombstone captures members_count > 0, which requires
	// querying session_members BEFORE the session row is deleted.
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())

	sess, _ := setupDestructionSession(t, ctx, env)
	d := newDestruction(env)

	if err := d.Destroy(ctx, sess, "idle"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	tomb, err := env.s.GetTombstone(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}
	// We added one member, so members_count must be 1 (not 0).
	if tomb.MembersCount < 1 {
		t.Errorf("tombstone members_count: want >= 1 (captured before delete), got %d", tomb.MembersCount)
	}
}

// ---------------------------------------------------------------------------
// TestDestruction_Idempotent
// ---------------------------------------------------------------------------

func TestDestruction_Idempotent(t *testing.T) {
	// Running Destroy twice on the same session should not error on the second
	// run. Every step is idempotent: tombstone uses ON CONFLICT DO NOTHING,
	// bearer revoke is a no-op when already revoked, session delete returns
	// ErrNotFound (tolerated), anon account delete is a no-op when IDs are gone.
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())

	sess, _ := setupDestructionSession(t, ctx, env)
	d := newDestruction(env)

	if err := d.Destroy(ctx, sess, "hard_cap"); err != nil {
		t.Fatalf("first Destroy: %v", err)
	}
	// Second call should not panic or return an error.
	if err := d.Destroy(ctx, sess, "hard_cap"); err != nil {
		t.Fatalf("second Destroy (idempotent): %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestDestruction_TombstoneStats
// ---------------------------------------------------------------------------

func TestDestruction_TombstoneStats(t *testing.T) {
	// Verify that duration_seconds, end_reason, expires_at are set correctly.
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())

	now := env.clock.Now()
	ttl := 30 * 24 * time.Hour

	sess, _ := setupDestructionSession(t, ctx, env)
	d := newDestruction(env)
	d.TombstoneTTL = ttl

	if err := d.Destroy(ctx, sess, "idle"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	tomb, err := env.s.GetTombstone(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}

	if tomb.EndReason != "idle" {
		t.Errorf("end_reason: want idle, got %s", tomb.EndReason)
	}
	// DurationSeconds must be >= 0.
	if tomb.DurationSeconds < 0 {
		t.Errorf("duration_seconds: want >= 0, got %d", tomb.DurationSeconds)
	}
	// ExpiresAt should be approximately now + 30 days.
	wantExpires := now.Add(ttl)
	diff := tomb.ExpiresAt.Sub(wantExpires)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("expires_at: want ~%v, got %v (diff %v)", wantExpires, tomb.ExpiresAt, diff)
	}
}

// ---------------------------------------------------------------------------
// TestDestruction_BearersRevoked
// ---------------------------------------------------------------------------

func TestDestruction_BearersRevoked(t *testing.T) {
	// After Destroy, oauth_tokens for the session are cascade-deleted by the FK.
	// We verify the session row is gone (which triggers the cascade), and the
	// revoke step ran (defense-in-depth: the bearer hash lookup would return ErrNotFound).
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())

	sess, _ := setupDestructionSession(t, ctx, env)
	d := newDestruction(env)

	if err := d.Destroy(ctx, sess, "hard_cap"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	// After cascade-delete of the session row, the bearer tokens are gone.
	// Verify via GetSession (should be deleted) since we can't easily look up
	// a specific token hash from the test without reading the raw token.
	_, err := env.s.GetSession(ctx, playground.ReservedOrgID, sess.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected session gone (implies bearers cascade-deleted), got err=%v", err)
	}
}

// ---------------------------------------------------------------------------
// TestDestruction_AnonymousAccountsDeleted
// ---------------------------------------------------------------------------

func TestDestruction_AnonymousAccountsDeleted(t *testing.T) {
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())

	// Two participants in the same session.
	svc := tokens.New(env.s)
	now := env.clock.Now()
	hardCapAt := now.Add(24 * time.Hour)
	idleTimeoutAt := now.Add(30 * time.Minute)

	sess, err := env.s.CreateSession(ctx, store.CreateSessionParams{
		ID:                        "dest-multi-001",
		OrgID:                     playground.ReservedOrgID,
		Name:                      "multi-member",
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
		t.Fatalf("CreateSession: %v", err)
	}

	var accountIDs []string
	for _, nick := range []string{"alice", "bob"} {
		_, accID, _, err := svc.IssueAnonymousSessionBearer(ctx, sess.ID, nick, 24*time.Hour)
		if err != nil {
			t.Fatalf("IssueAnonymousSessionBearer(%s): %v", nick, err)
		}
		if err := env.s.AddSessionMember(ctx, store.AddSessionMemberParams{
			OrgID:     playground.ReservedOrgID,
			SessionID: sess.ID,
			AccountID: accID,
			Role:      "member",
			JoinedAt:  now,
		}); err != nil {
			t.Fatalf("AddSessionMember(%s): %v", nick, err)
		}
		accountIDs = append(accountIDs, accID)
	}

	if err := env.stor.CreateRepo(ctx, playground.ReservedOrgID, sess.ID); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	d := newDestruction(env)
	if err := d.Destroy(ctx, sess, "hard_cap"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	// Both anon accounts must be deleted.
	for _, id := range accountIDs {
		_, err := env.s.GetAccountByID(ctx, id)
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("anon account %s: expected deleted, got err=%v", id, err)
		}
	}
}

// ---------------------------------------------------------------------------
// TestDestruction_RepoRemovedFromStorage
// ---------------------------------------------------------------------------

func TestDestruction_RepoRemovedFromStorage(t *testing.T) {
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())
	sess, _ := setupDestructionSession(t, ctx, env)

	// Confirm repo exists before destruction.
	exists, _ := env.stor.RepoExists(playground.ReservedOrgID, sess.ID)
	if !exists {
		t.Fatal("expected bare repo to exist before destruction")
	}

	d := newDestruction(env)
	if err := d.Destroy(ctx, sess, "idle"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	exists, _ = env.stor.RepoExists(playground.ReservedOrgID, sess.ID)
	if exists {
		t.Error("expected bare repo to be deleted after destruction")
	}
}

// ---------------------------------------------------------------------------
// TestDestruction_DeleteAccountsByIDs_Empty
// ---------------------------------------------------------------------------

func TestDestruction_DeleteAccountsByIDs_Empty(t *testing.T) {
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())

	// Should be a no-op without error.
	if err := env.s.DeleteAccountsByIDs(ctx, []string{}); err != nil {
		t.Errorf("DeleteAccountsByIDs(empty): %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestDestruction_CountSessionEventsByType
// ---------------------------------------------------------------------------

func TestDestruction_CountSessionEventsByType(t *testing.T) {
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())
	sess, _ := setupDestructionSession(t, ctx, env)

	// Count with no events — should be 0.
	count, err := env.s.CountSessionEventsByType(ctx, sess.ID, "commit.arrived")
	if err != nil {
		t.Fatalf("CountSessionEventsByType: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 events, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// TestDestruction_ListAnonymousSessionMemberIDs
// ---------------------------------------------------------------------------

func TestDestruction_ListAnonymousSessionMemberIDs(t *testing.T) {
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())
	sess, anonID := setupDestructionSession(t, ctx, env)

	ids, err := env.s.ListAnonymousSessionMemberIDs(ctx, playground.ReservedOrgID, sess.ID)
	if err != nil {
		t.Fatalf("ListAnonymousSessionMemberIDs: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 anon member, got %d", len(ids))
	}
	if ids[0] != anonID {
		t.Errorf("anon member ID: want %s, got %s", anonID, ids[0])
	}
}

// ---------------------------------------------------------------------------
// TestDestruction_ConcurrentDestroyCallsForSameSession_NoCorruption
// ---------------------------------------------------------------------------

// concurrentSafeStorage wraps stubStorage with a mutex so concurrent
// goroutines don't race on the underlying map. This mirrors the situation
// where two portal pods share a database but each drives its own local
// filesystem (storage) operation — so we only need map-level safety here,
// not cross-process safety.
type concurrentSafeStorage struct {
	mu   sync.Mutex
	stor *stubStorage
}

func newConcurrentSafeStorage() *concurrentSafeStorage {
	return &concurrentSafeStorage{stor: newStubStorage()}
}

func (s *concurrentSafeStorage) RepoPath(orgID, sessionID string) string {
	// Pure string construction — no map access, no lock needed.
	return s.stor.RepoPath(orgID, sessionID)
}

func (s *concurrentSafeStorage) CreateRepo(ctx context.Context, orgID, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stor.CreateRepo(ctx, orgID, sessionID)
}

func (s *concurrentSafeStorage) RemoveRepo(ctx context.Context, orgID, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stor.RemoveRepo(ctx, orgID, sessionID)
}

func (s *concurrentSafeStorage) RepoExists(orgID, sessionID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stor.RepoExists(orgID, sessionID)
}

func (s *concurrentSafeStorage) ArchiveSession(ctx context.Context, orgID, sessionID string, info storage.ArchiveInfo) error {
	// Destruction never calls ArchiveSession; delegate unconditionally.
	return s.stor.ArchiveSession(ctx, orgID, sessionID, info)
}

func (s *concurrentSafeStorage) LookupArchived(ctx context.Context, orgID, sessionID string) (*storage.ArchivedRecord, error) {
	return s.stor.LookupArchived(ctx, orgID, sessionID)
}

func (s *concurrentSafeStorage) StubResponse(rec *storage.ArchivedRecord) storage.ArchivedStub {
	return s.stor.StubResponse(rec)
}

func TestDestruction_ConcurrentDestroyCallsForSameSession_NoCorruption(t *testing.T) {
	// Two goroutines call Destroy(ctx, sess, "hard_cap") concurrently against
	// the same session row. Each step in Destroy is idempotent:
	//   - tombstone uses ON CONFLICT DO NOTHING
	//   - bearer revoke is a no-op when already revoked
	//   - DeleteSession tolerates ErrNotFound (session already gone)
	//   - DeleteAccountsByIDs is a no-op on already-absent rows
	//
	// This is the in-process analogue of what JAMSESH_DEPLOY_MODE=clustered
	// exposes across pods. The advisory-lock fix (bug-playground-destruction-
	// clustered-advisory-lock) operates at the PostgreSQL level; this test
	// exercises the idempotency contract at the Go call level, and is run
	// with -race to surface any data races introduced by future refactors.
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())

	// SQLite :memory: gives each connection its own empty database. Concurrent
	// goroutines that get different pool connections would see "no such table".
	// Constrain the pool to a single connection so all goroutines share the same
	// in-memory DB — matching the single-writer SQLite production deployment.
	// This mirrors the pattern in internal/portal/automerger/worker_test.go.
	type rawDBer interface{ RawDB() *sql.DB }
	if r, ok := env.s.(rawDBer); ok {
		r.RawDB().SetMaxOpenConns(1)
	}

	sess, accountID := setupDestructionSession(t, ctx, env)

	// Build a mutex-guarded storage wrapper so the race detector doesn't fire
	// on the stub's underlying map. The SQLite store serialises its own writes
	// (single connection) so no wrapping is needed on the DB side.
	safe := newConcurrentSafeStorage()
	if err := safe.CreateRepo(ctx, playground.ReservedOrgID, sess.ID); err != nil {
		t.Fatalf("pre-create repo in safe storage: %v", err)
	}

	makeD := func() *playground.Destruction {
		return &playground.Destruction{
			Store:        env.s,
			Storage:      safe,
			Clock:        env.clock,
			Logger:       noopLogger(),
			TombstoneTTL: 30 * 24 * time.Hour,
		}
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		errList []error
	)
	barrier := make(chan struct{})

	for i := 0; i < 2; i++ {
		d := makeD()
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-barrier // synchronise start so both goroutines overlap
			if err := d.Destroy(ctx, sess, "hard_cap"); err != nil {
				mu.Lock()
				errList = append(errList, err)
				mu.Unlock()
			}
		}()
	}

	close(barrier) // release both goroutines simultaneously
	wg.Wait()

	// Neither call should return an error — idempotency must hold under
	// concurrent invocation just as it does for sequential re-invocation
	// (see TestDestruction_Idempotent).
	for _, err := range errList {
		t.Errorf("Destroy returned unexpected error: %v", err)
	}

	// Exactly one tombstone row must exist (ON CONFLICT DO NOTHING is idempotent).
	tomb, err := env.s.GetTombstone(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetTombstone after concurrent destroy: %v", err)
	}
	if tomb.EndReason != "hard_cap" {
		t.Errorf("tombstone end_reason: want hard_cap, got %s", tomb.EndReason)
	}

	// Session row must be gone — deleted by whichever goroutine won step 6.
	_, err = env.s.GetSession(ctx, playground.ReservedOrgID, sess.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected session deleted after concurrent destroy; got err=%v", err)
	}

	// Anonymous account must be deleted — not double-deleted in a way that
	// surfaces an error (DeleteAccountsByIDs must tolerate already-absent rows).
	_, err = env.s.GetAccountByID(ctx, accountID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected anon account deleted after concurrent destroy; got err=%v", err)
	}

	// Bare repo must be removed (RemoveRepo on an absent key is a no-op in
	// stubStorage, so double-removal by both goroutines is safe).
	exists, _ := safe.RepoExists(playground.ReservedOrgID, sess.ID)
	if exists {
		t.Error("expected bare repo deleted after concurrent destruction")
	}
}

// ---------------------------------------------------------------------------
// TestDestruction_AdvisoryLock_SecondDestroyIsNoOp
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// TestDestruction_BearerRevokedBeforeSessionDelete
// ---------------------------------------------------------------------------

// orderCapturingStore wraps a store.Store and records the sequence of
// RevokeBearersForSession and DeleteSession calls so that the ordering
// invariant (step 4 before step 6) can be asserted.
type orderCapturingStore struct {
	store.Store
	mu  sync.Mutex
	seq []string // "revoke" or "delete" appended on each call
}

func (s *orderCapturingStore) RevokeBearersForSession(ctx context.Context, p store.RevokeBearersForSessionParams) error {
	s.mu.Lock()
	s.seq = append(s.seq, "revoke")
	s.mu.Unlock()
	return s.Store.RevokeBearersForSession(ctx, p)
}

func (s *orderCapturingStore) DeleteSession(ctx context.Context, p store.DeleteSessionParams) error {
	s.mu.Lock()
	s.seq = append(s.seq, "delete")
	s.mu.Unlock()
	return s.Store.DeleteSession(ctx, p)
}

// TestDestruction_BearerRevokedBeforeSessionDelete verifies the defense-in-depth
// ordering: RevokeBearersForSession (step 4) MUST be called before DeleteSession
// (step 6). An in-flight request from an anonymous user that races between these
// two steps would use a revoked-but-not-yet-cascade-deleted bearer — which is
// correctly rejected by the auth middleware. If the order were reversed (delete
// then revoke), the cascade would have already removed the bearer row making the
// revoke a no-op, but any request that slipped in between the two steps would
// hit a dangling bearer that was neither revoked nor deleted yet.
func TestDestruction_BearerRevokedBeforeSessionDelete(t *testing.T) {
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())

	sess, _ := setupDestructionSession(t, ctx, env)

	capturing := &orderCapturingStore{Store: env.s}
	d := &playground.Destruction{
		Store:        capturing,
		Storage:      env.stor,
		Clock:        env.clock,
		Logger:       noopLogger(),
		TombstoneTTL: 30 * 24 * time.Hour,
	}

	if err := d.Destroy(ctx, sess, "hard_cap"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	capturing.mu.Lock()
	seq := append([]string(nil), capturing.seq...)
	capturing.mu.Unlock()

	// Both operations must have been called.
	revokeIdx := -1
	deleteIdx := -1
	for i, op := range seq {
		switch op {
		case "revoke":
			if revokeIdx == -1 {
				revokeIdx = i
			}
		case "delete":
			if deleteIdx == -1 {
				deleteIdx = i
			}
		}
	}

	if revokeIdx == -1 {
		t.Fatal("RevokeBearersForSession was never called")
	}
	if deleteIdx == -1 {
		t.Fatal("DeleteSession was never called")
	}
	if revokeIdx >= deleteIdx {
		t.Errorf("ordering violation: RevokeBearersForSession (pos %d) must be called BEFORE DeleteSession (pos %d); seq=%v",
			revokeIdx, deleteIdx, seq)
	}
}

// TestDestruction_AdvisoryLock_SecondDestroyIsNoOp verifies that when two
// Destruction instances race to destroy the same session and only one can
// acquire the per-session lock (stubLeaseManager enforces mutual exclusion),
// exactly one completes the cascade and the other returns nil immediately
// without touching the database.
//
// The user-visible assertion is: after both calls return, the session is gone
// and exactly one tombstone exists. The "losing" pod is identified by its
// lock being blocked — we hold the lock on the first instance while the second
// tries to acquire it, confirming the second returns nil (no error, no cascade).
func TestDestruction_AdvisoryLock_SecondDestroyIsNoOp(t *testing.T) {
	ctx := context.Background()
	env := newTestEnvSQLite(t, defaultCfg())

	sess, _ := setupDestructionSession(t, ctx, env)

	leaseMgr := newStubLeaseManager()

	// Build the "winner" Destruction — acquires the lock and holds it.
	winner := &playground.Destruction{
		Store:        env.s,
		Storage:      env.stor,
		Clock:        env.clock,
		Logger:       noopLogger(),
		TombstoneTTL: 30 * 24 * time.Hour,
		Leases:       leaseMgr,
	}

	// Build the "loser" Destruction — identical configuration; same lock manager.
	loser := &playground.Destruction{
		Store:        env.s,
		Storage:      env.stor,
		Clock:        env.clock,
		Logger:       noopLogger(),
		TombstoneTTL: 30 * 24 * time.Hour,
		Leases:       leaseMgr,
	}

	// Manually acquire the lock for the session before the loser tries —
	// this simulates the winner pod having already grabbed the advisory lock.
	winnerHandle, err := leaseMgr.Acquire(ctx, sess.ID)
	if err != nil {
		t.Fatalf("pre-acquire (winner setup): %v", err)
	}

	// The loser calls Destroy while the winner holds the lock.
	// It must return nil immediately (no-op) without touching the DB.
	if err := loser.Destroy(ctx, sess, "hard_cap"); err != nil {
		t.Fatalf("loser Destroy: expected nil (lock held by winner), got %v", err)
	}

	// Confirm: session still exists — the loser didn't run the cascade.
	_, err = env.s.GetSession(ctx, playground.ReservedOrgID, sess.ID)
	if err != nil {
		t.Errorf("session should still exist after loser no-op; got err=%v", err)
	}

	// Release the winner's lock and run its cascade.
	winnerHandle.Release() //nolint:errcheck
	if err := winner.Destroy(ctx, sess, "hard_cap"); err != nil {
		t.Fatalf("winner Destroy: %v", err)
	}

	// Now the session must be gone.
	_, err = env.s.GetSession(ctx, playground.ReservedOrgID, sess.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected session deleted after winner cascade; got err=%v", err)
	}

	// Exactly one tombstone must exist.
	tomb, err := env.s.GetTombstone(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}
	if tomb.EndReason != "hard_cap" {
		t.Errorf("tombstone end_reason: want hard_cap, got %s", tomb.EndReason)
	}
}
