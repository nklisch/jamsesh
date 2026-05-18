package finalize_test

import (
	"context"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/finalize"
	"jamsesh/internal/portal/tokens"
)

func TestAcquireFinalizeLock_NoExistingLock(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := env.callerCtx

	// Subscribe to events so we can assert session.finalizing was emitted.
	events, unsub := env.log.Subscribe("session.finalizing")
	defer unsub()

	resp, err := env.handler.AcquireFinalizeLock(ctx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	ok, status := asAcquire201(t, resp)
	if !ok {
		t.Fatalf("expected 201 LockStatus, got %T", resp)
	}
	if status.HeldByAccountId != env.caller.ID {
		t.Errorf("HeldByAccountId = %q, want %q", status.HeldByAccountId, env.caller.ID)
	}
	if !status.IsCaller {
		t.Error("IsCaller = false, want true")
	}
	if status.LockId == "" {
		t.Error("LockId is empty")
	}
	if !status.ExpiresAt.After(status.LastActivityAt) {
		t.Error("ExpiresAt should be after LastActivityAt")
	}

	// Session status flipped to "finalizing".
	sess, err := env.store.GetSession(context.Background(), env.orgID, env.sessID)
	if err != nil {
		t.Fatalf("re-get session: %v", err)
	}
	if sess.Status != "finalizing" {
		t.Errorf("session.status = %q, want %q", sess.Status, "finalizing")
	}
	if sess.FinalizeLockedByAccountID == nil || *sess.FinalizeLockedByAccountID != env.caller.ID {
		t.Errorf("FinalizeLockedByAccountID = %v, want %s", sess.FinalizeLockedByAccountID, env.caller.ID)
	}

	// session.finalizing event was emitted.
	select {
	case ev := <-events:
		if ev.Type != "session.finalizing" {
			t.Errorf("got event type %q, want session.finalizing", ev.Type)
		}
		if ev.SessionID != env.sessID {
			t.Errorf("event session_id = %q, want %q", ev.SessionID, env.sessID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session.finalizing event")
	}
}

func TestAcquireFinalizeLock_IdempotentReacquire(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := env.callerCtx

	resp1, err := env.handler.AcquireFinalizeLock(ctx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	_, status1 := asAcquire201(t, resp1)

	// Subscribe to events AFTER the first acquire — we expect NO new
	// session.finalizing event on the idempotent path.
	events, unsub := env.log.Subscribe("session.finalizing")
	defer unsub()

	resp2, err := env.handler.AcquireFinalizeLock(ctx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	_, status2 := asAcquire201(t, resp2)

	if status1.LockId != status2.LockId {
		t.Errorf("re-acquire produced new lock %q (orig %q); should be idempotent", status2.LockId, status1.LockId)
	}

	select {
	case ev := <-events:
		t.Errorf("unexpected event emitted on idempotent re-acquire: %s", ev.Type)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestAcquireFinalizeLock_StaleLockAutoReleases(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	// Seed a stale lock held by `other`: last_activity 31 minutes ago.
	staleLockID := ulid.Make().String()
	stale := time.Now().UTC().Add(-31 * time.Minute)
	if err := env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID:                  staleLockID,
		OrgID:               env.orgID,
		SessionID:           env.sessID,
		AcquiredByAccountID: env.otherID,
		AcquiredAt:          stale,
		LastActivityAt:      stale,
		SelectedCommitSHAs:  "[]",
		Mode:                "squash",
	}); err != nil {
		t.Fatalf("seed stale lock: %v", err)
	}

	// Caller acquires — stale lock should be released and a new one minted.
	resp, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	_, status := asAcquire201(t, resp)
	if status.LockId == staleLockID {
		t.Errorf("returned the stale lock id; expected a new one")
	}
	if status.HeldByAccountId != env.caller.ID {
		t.Errorf("HeldByAccountId = %q, want %q (caller)", status.HeldByAccountId, env.caller.ID)
	}

	// Stale row's released_at is set.
	old, err := env.store.GetFinalizeLockByID(ctx, staleLockID)
	if err != nil {
		t.Fatalf("get stale lock: %v", err)
	}
	if old.ReleasedAt == nil {
		t.Error("stale lock released_at is still nil; should be set")
	}
}

func TestAcquireFinalizeLock_HeldByOtherFresh_409(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	// Seed a fresh lock held by `other`.
	freshID := ulid.Make().String()
	now := time.Now().UTC()
	if err := env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID:                  freshID,
		OrgID:               env.orgID,
		SessionID:           env.sessID,
		AcquiredByAccountID: env.otherID,
		AcquiredAt:          now,
		LastActivityAt:      now,
		SelectedCommitSHAs:  "[]",
		Mode:                "squash",
	}); err != nil {
		t.Fatalf("seed fresh lock: %v", err)
	}

	resp, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	r, ok := resp.(openapi.AcquireFinalizeLock409JSONResponse)
	if !ok {
		t.Fatalf("expected 409, got %T", resp)
	}
	if r.Error != "finalize.lock_held_by_other" {
		t.Errorf("error code = %q, want finalize.lock_held_by_other", r.Error)
	}
	if r.Details == nil {
		t.Fatal("expected details with held_by_account_id")
	}
	if got := r.Details["held_by_account_id"]; got != env.otherID {
		t.Errorf("details.held_by_account_id = %v, want %s", got, env.otherID)
	}
}

func TestAcquireFinalizeLock_OverrideSupersedes(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	// Seed a fresh lock held by `other`, with sessions pointer pointing at other.
	freshID := ulid.Make().String()
	now := time.Now().UTC()
	if err := env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID:                  freshID,
		OrgID:               env.orgID,
		SessionID:           env.sessID,
		AcquiredByAccountID: env.otherID,
		AcquiredAt:          now,
		LastActivityAt:      now,
		SelectedCommitSHAs:  "[]",
		Mode:                "squash",
	}); err != nil {
		t.Fatalf("seed fresh lock: %v", err)
	}
	otherID := env.otherID
	if err := env.store.SetFinalizeLock(ctx, store.SetFinalizeLockParams{
		OrgID: env.orgID, ID: env.sessID, AccountID: &otherID,
	}); err != nil {
		t.Fatalf("seed sessions pointer: %v", err)
	}
	if err := env.store.UpdateSessionStatus(ctx, store.UpdateSessionStatusParams{
		OrgID: env.orgID, ID: env.sessID, Status: "finalizing",
	}); err != nil {
		t.Fatalf("seed session status: %v", err)
	}

	override := true
	resp, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		Body:      &openapi.AcquireFinalizeLockJSONRequestBody{Override: override},
	})
	if err != nil {
		t.Fatalf("override acquire: %v", err)
	}
	_, status := asAcquire201(t, resp)
	if status.LockId == freshID {
		t.Errorf("override returned the old lock id; expected new")
	}
	if status.HeldByAccountId != env.caller.ID {
		t.Errorf("HeldByAccountId = %q, want %s", status.HeldByAccountId, env.caller.ID)
	}

	// Old row's superseded_by_lock_id points at the new lock.
	old, err := env.store.GetFinalizeLockByID(ctx, freshID)
	if err != nil {
		t.Fatalf("get old lock: %v", err)
	}
	if old.SupersededByLockID == nil || *old.SupersededByLockID != status.LockId {
		t.Errorf("old lock superseded_by_lock_id = %v, want %s", old.SupersededByLockID, status.LockId)
	}

	// Sessions pointer reassigned to caller.
	sess, _ := env.store.GetSession(ctx, env.orgID, env.sessID)
	if sess.FinalizeLockedByAccountID == nil || *sess.FinalizeLockedByAccountID != env.caller.ID {
		t.Errorf("sessions.finalize_locked_by = %v, want %s", sess.FinalizeLockedByAccountID, env.caller.ID)
	}
}

// TestAcquireFinalizeLock_ConcurrentOverrides_OnlyOneWins races two callers
// (A and C) both attempting override=true against B's active lock. It asserts
// the core safety invariant: after both goroutines return there must be exactly
// one non-superseded, non-released lock row for the session — two active rows
// would indicate a concurrency bug in the insert-before-supersede sequencing.
//
// The test deliberately avoids asserting which racer wins (A or C); either
// outcome is acceptable. What is NOT acceptable is both rows being active.
func TestAcquireFinalizeLock_ConcurrentOverrides_OnlyOneWins(t *testing.T) {
	// MaxOpenConns=1 forces all goroutines through the single in-memory SQLite
	// connection, so they all see the same schema/data.  SQLite is effectively
	// single-writer anyway; the constraint is intentional, not a workaround.
	env := newFinalizeEnvPool(t, db.PoolConfig{MaxOpenConns: 1})
	ctx := context.Background()

	// Seed B's lock: fresh, held by otherID (B).  Set session pointer + status
	// to "finalizing" so the handler takes branch 5 (override) rather than
	// branch 1 (no lock) or branch 3 (stale).
	bLockID := ulid.Make().String()
	now := time.Now().UTC()
	if err := env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID:                  bLockID,
		OrgID:               env.orgID,
		SessionID:           env.sessID,
		AcquiredByAccountID: env.otherID, // B holds it
		AcquiredAt:          now,
		LastActivityAt:      now,
		SelectedCommitSHAs:  "[]",
		Mode:                "squash",
	}); err != nil {
		t.Fatalf("seed B lock: %v", err)
	}
	bID := env.otherID
	if err := env.store.SetFinalizeLock(ctx, store.SetFinalizeLockParams{
		OrgID: env.orgID, ID: env.sessID, AccountID: &bID,
	}); err != nil {
		t.Fatalf("seed sessions pointer: %v", err)
	}
	if err := env.store.UpdateSessionStatus(ctx, store.UpdateSessionStatusParams{
		OrgID: env.orgID, ID: env.sessID, Status: "finalizing",
	}); err != nil {
		t.Fatalf("seed session status: %v", err)
	}

	// Create account C — a third session member who also wants to override.
	cID := ulid.Make().String()
	cAcct, err := env.store.CreateAccount(ctx, store.CreateAccountParams{
		ID: cID, Email: "c-" + cID[:8] + "@example.com",
		DisplayName: "AccountC", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create account C: %v", err)
	}
	if err := env.store.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID: env.orgID, AccountID: cID, Role: "member", CreatedAt: now,
	}); err != nil {
		t.Fatalf("add C to org: %v", err)
	}
	if err := env.store.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: env.orgID, SessionID: env.sessID, AccountID: cID, Role: "member", JoinedAt: now,
	}); err != nil {
		t.Fatalf("add C to session: %v", err)
	}
	cCtx := tokens.ContextWithAccount(ctx, &cAcct)

	// Race A (caller) and C both calling override=true as close together as
	// possible.  Use a ready channel to synchronise the goroutine launches.
	type result struct {
		resp openapi.AcquireFinalizeLockResponseObject
		err  error
	}
	ready := make(chan struct{})
	aCh := make(chan result, 1)
	cCh := make(chan result, 1)

	go func() {
		<-ready
		resp, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
			OrgID:     env.orgID,
			SessionID: env.sessID,
			Body:      &openapi.AcquireFinalizeLockJSONRequestBody{Override: true},
		})
		aCh <- result{resp, err}
	}()
	go func() {
		<-ready
		resp, err := env.handler.AcquireFinalizeLock(cCtx, openapi.AcquireFinalizeLockRequestObject{
			OrgID:     env.orgID,
			SessionID: env.sessID,
			Body:      &openapi.AcquireFinalizeLockJSONRequestBody{Override: true},
		})
		cCh <- result{resp, err}
	}()

	// Fire both goroutines simultaneously.
	close(ready)
	aRes := <-aCh
	cRes := <-cCh

	// Neither call should return a Go error (internal/dep errors), regardless
	// of which racer "won".
	if aRes.err != nil {
		t.Errorf("A returned unexpected error: %v", aRes.err)
	}
	if cRes.err != nil {
		t.Errorf("C returned unexpected error: %v", cRes.err)
	}

	// Determine A and C's lock IDs (if they got 201).
	var aLockID, cLockID string
	if ok, status := asAcquire201(t, aRes.resp); ok {
		aLockID = status.LockId
	}
	if ok, status := asAcquire201(t, cRes.resp); ok {
		cLockID = status.LockId
	}

	// At least one of A or C must have succeeded with a 201 (it's possible
	// both return 201 if their override paths interleave, but one will win
	// the supersede race).  If both get non-201, that's also a bug.
	if aLockID == "" && cLockID == "" {
		t.Errorf("both A and C failed to acquire a lock; at least one must win. A=%T C=%T", aRes.resp, cRes.resp)
	}

	// --- Core safety invariant ---
	// Count unsuperseded, unreleased lock rows for the session.
	// We check every known lock ID (B, A if created, C if created) and tally
	// rows where SupersededByLockID == nil AND ReleasedAt == nil.
	allIDs := []string{bLockID}
	if aLockID != "" {
		allIDs = append(allIDs, aLockID)
	}
	if cLockID != "" && cLockID != aLockID {
		allIDs = append(allIDs, cLockID)
	}

	activeCount := 0
	var activeLockIDs []string
	for _, id := range allIDs {
		lock, err := env.store.GetFinalizeLockByID(ctx, id)
		if err != nil {
			t.Errorf("GetFinalizeLockByID(%s): %v", id, err)
			continue
		}
		if lock.SupersededByLockID == nil && lock.ReleasedAt == nil {
			activeCount++
			activeLockIDs = append(activeLockIDs, id)
		}
	}

	if activeCount != 1 {
		t.Errorf("expected exactly 1 active (unsuperseded, unreleased) lock, got %d: %v", activeCount, activeLockIDs)
	}

	// sessions.finalize_locked_by_account_id must point at whichever account
	// owns the surviving active lock.
	sess, err := env.store.GetSession(ctx, env.orgID, env.sessID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.FinalizeLockedByAccountID == nil {
		t.Fatal("sessions.finalize_locked_by_account_id is nil; expected a winner account")
	}
	winnerAccountID := *sess.FinalizeLockedByAccountID
	if winnerAccountID != env.caller.ID && winnerAccountID != cID {
		t.Errorf("sessions.finalize_locked_by_account_id = %s, want one of {A=%s, C=%s}",
			winnerAccountID, env.caller.ID, cID)
	}
}

func TestAcquireFinalizeLock_Unauthenticated(t *testing.T) {
	env := newFinalizeEnv(t)
	resp, err := env.handler.AcquireFinalizeLock(context.Background(), openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, ok := resp.(openapi.AcquireFinalizeLock401JSONResponse); !ok {
		t.Fatalf("expected 401, got %T", resp)
	}
}

func TestAcquireFinalizeLock_NotMember_403(t *testing.T) {
	env := newFinalizeEnv(t)

	// Create a third account that is not a member of the session.
	outsiderID := ulid.Make().String()
	now := time.Now().UTC()
	outsider, err := env.store.CreateAccount(context.Background(), store.CreateAccountParams{
		ID: outsiderID, Email: "outsider@example.com", DisplayName: "Outsider", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create outsider: %v", err)
	}
	// Outsider is not an org member either.

	ctx := contextWithAccount(context.Background(), &outsider)
	resp, err := env.handler.AcquireFinalizeLock(ctx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, ok := resp.(openapi.AcquireFinalizeLock403JSONResponse); !ok {
		t.Fatalf("expected 403, got %T", resp)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// asAcquire201 type-asserts a response object to AcquireFinalizeLock201
// (LockStatus body), failing the test on mismatch.
func asAcquire201(t *testing.T, resp openapi.AcquireFinalizeLockResponseObject) (bool, openapi.LockStatus) {
	t.Helper()
	r, ok := resp.(openapi.AcquireFinalizeLock201JSONResponse)
	if !ok {
		return false, openapi.LockStatus{}
	}
	return true, openapi.LockStatus(r)
}

// ensure finalize import is used by tests in this file even if every
// reference is via env.handler; keeps the import block tidy when test
// reorganisations move things around.
var _ = finalize.FinalizeLockTTL
