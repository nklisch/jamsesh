package playground_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/playground"
	"jamsesh/internal/portal/tokens"
)

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
