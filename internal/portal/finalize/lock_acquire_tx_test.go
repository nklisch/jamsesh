package finalize_test

// TestAcquireFinalizeLock_TxRollbackOnMidSequenceFailure verifies that if the
// mutation sequence inside the WithTx closure fails midway, the earlier
// mutations are rolled back — no orphaned lock rows remain.
//
// The test uses a wrapping store that injects an error on InsertFinalizeLock
// and verifies that no finalize_lock row was committed.
//
// TestAcquireFinalizeLock_UniqueViolationReturns409Regardless verifies that a
// unique-violation on InsertFinalizeLock returns a 409 regardless of whether
// supersedeOldID was set (covers the fresh-insert race path, not just the
// override path).

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
)

// --- wrapping store for mid-sequence failure injection ---

// failingInsertLockStore is a wrapping store that delegates everything to the
// real store but returns a custom error on InsertFinalizeLock.
type failingInsertLockStore struct {
	realStore store.Store
	failErr   error
}

func (f *failingInsertLockStore) InsertFinalizeLock(_ context.Context, _ store.InsertFinalizeLockParams) error {
	return f.failErr
}
func (f *failingInsertLockStore) GetFinalizeLockByID(ctx context.Context, id string) (store.FinalizeLock, error) {
	return f.realStore.GetFinalizeLockByID(ctx, id)
}
func (f *failingInsertLockStore) GetActiveFinalizeLockForSession(ctx context.Context, sessionID string) (store.FinalizeLock, error) {
	return f.realStore.GetActiveFinalizeLockForSession(ctx, sessionID)
}
func (f *failingInsertLockStore) UpdateFinalizeLockCuration(ctx context.Context, p store.UpdateFinalizeLockCurationParams) error {
	return f.realStore.UpdateFinalizeLockCuration(ctx, p)
}
func (f *failingInsertLockStore) TouchFinalizeLock(ctx context.Context, p store.TouchFinalizeLockParams) error {
	return f.realStore.TouchFinalizeLock(ctx, p)
}
func (f *failingInsertLockStore) ReleaseFinalizeLock(ctx context.Context, p store.ReleaseFinalizeLockParams) error {
	return f.realStore.ReleaseFinalizeLock(ctx, p)
}
func (f *failingInsertLockStore) ReleaseFinalizeLockIfStale(ctx context.Context, p store.ReleaseFinalizeLockIfStaleParams) (int64, error) {
	return f.realStore.ReleaseFinalizeLockIfStale(ctx, p)
}
func (f *failingInsertLockStore) SupersedeFinalizeLock(ctx context.Context, p store.SupersedeFinalizeLockParams) error {
	return f.realStore.SupersedeFinalizeLock(ctx, p)
}
func (f *failingInsertLockStore) CreateSession(ctx context.Context, p store.CreateSessionParams) (store.Session, error) {
	return f.realStore.CreateSession(ctx, p)
}
func (f *failingInsertLockStore) GetSession(ctx context.Context, orgID, id string) (store.Session, error) {
	return f.realStore.GetSession(ctx, orgID, id)
}
func (f *failingInsertLockStore) GetSessionByID(ctx context.Context, id string) (store.Session, error) {
	return f.realStore.GetSessionByID(ctx, id)
}
func (f *failingInsertLockStore) ListSessionsForOrg(ctx context.Context, orgID string) ([]store.Session, error) {
	return f.realStore.ListSessionsForOrg(ctx, orgID)
}
func (f *failingInsertLockStore) ListSessionsForOrgWithCursor(ctx context.Context, p store.ListSessionsForOrgWithCursorParams) ([]store.Session, error) {
	return f.realStore.ListSessionsForOrgWithCursor(ctx, p)
}
func (f *failingInsertLockStore) UpdateSessionStatus(ctx context.Context, p store.UpdateSessionStatusParams) error {
	return f.realStore.UpdateSessionStatus(ctx, p)
}
func (f *failingInsertLockStore) UpdateSessionGoalScopeMode(ctx context.Context, p store.UpdateSessionGoalScopeModeParams) error {
	return f.realStore.UpdateSessionGoalScopeMode(ctx, p)
}
func (f *failingInsertLockStore) SetSessionBaseSHA(ctx context.Context, p store.SetSessionBaseSHAParams) error {
	return f.realStore.SetSessionBaseSHA(ctx, p)
}
func (f *failingInsertLockStore) SetSessionEndReason(ctx context.Context, p store.SetSessionEndReasonParams) error {
	return f.realStore.SetSessionEndReason(ctx, p)
}
func (f *failingInsertLockStore) SetFinalizeLock(ctx context.Context, p store.SetFinalizeLockParams) error {
	return f.realStore.SetFinalizeLock(ctx, p)
}
func (f *failingInsertLockStore) ClearFinalizeLock(ctx context.Context, p store.ClearFinalizeLockParams) error {
	return f.realStore.ClearFinalizeLock(ctx, p)
}
func (f *failingInsertLockStore) DeleteSession(ctx context.Context, p store.DeleteSessionParams) error {
	return f.realStore.DeleteSession(ctx, p)
}
func (f *failingInsertLockStore) AddSessionMember(ctx context.Context, p store.AddSessionMemberParams) error {
	return f.realStore.AddSessionMember(ctx, p)
}
func (f *failingInsertLockStore) GetSessionMember(ctx context.Context, p store.GetSessionMemberParams) (store.SessionMember, error) {
	return f.realStore.GetSessionMember(ctx, p)
}
func (f *failingInsertLockStore) ListSessionMembers(ctx context.Context, p store.ListSessionMembersParams) ([]store.SessionMember, error) {
	return f.realStore.ListSessionMembers(ctx, p)
}
func (f *failingInsertLockStore) RemoveSessionMember(ctx context.Context, p store.RemoveSessionMemberParams) error {
	return f.realStore.RemoveSessionMember(ctx, p)
}
func (f *failingInsertLockStore) ListSessionMembershipsForAccount(ctx context.Context, accountID string) ([]store.SessionMembership, error) {
	return f.realStore.ListSessionMembershipsForAccount(ctx, accountID)
}
func (f *failingInsertLockStore) NicknameTakenInSession(ctx context.Context, p store.NicknameTakenInSessionParams) (bool, error) {
	return f.realStore.NicknameTakenInSession(ctx, p)
}
func (f *failingInsertLockStore) CountSessionMembers(ctx context.Context, p store.CountSessionMembersParams) (int64, error) {
	return f.realStore.CountSessionMembers(ctx, p)
}
func (f *failingInsertLockStore) AddOrgMember(ctx context.Context, p store.AddOrgMemberParams) error {
	return f.realStore.AddOrgMember(ctx, p)
}
func (f *failingInsertLockStore) GetOrgMember(ctx context.Context, p store.GetOrgMemberParams) (store.OrgMember, error) {
	return f.realStore.GetOrgMember(ctx, p)
}
func (f *failingInsertLockStore) ListOrgsForAccount(ctx context.Context, accountID string) ([]store.Org, error) {
	return f.realStore.ListOrgsForAccount(ctx, accountID)
}
func (f *failingInsertLockStore) ListOrgMembers(ctx context.Context, orgID string) ([]store.OrgMemberWithAccount, error) {
	return f.realStore.ListOrgMembers(ctx, orgID)
}
func (f *failingInsertLockStore) RemoveOrgMember(ctx context.Context, p store.RemoveOrgMemberParams) error {
	return f.realStore.RemoveOrgMember(ctx, p)
}
func (f *failingInsertLockStore) CreateAccount(ctx context.Context, p store.CreateAccountParams) (store.Account, error) {
	return f.realStore.CreateAccount(ctx, p)
}
func (f *failingInsertLockStore) CreateAnonymousAccount(ctx context.Context, p store.CreateAnonymousAccountParams) (store.Account, error) {
	return f.realStore.CreateAnonymousAccount(ctx, p)
}
func (f *failingInsertLockStore) GetAccountByID(ctx context.Context, id string) (store.Account, error) {
	return f.realStore.GetAccountByID(ctx, id)
}
func (f *failingInsertLockStore) GetAccountByEmail(ctx context.Context, email string) (store.Account, error) {
	return f.realStore.GetAccountByEmail(ctx, email)
}
func (f *failingInsertLockStore) GetAccountByGitHubUserID(ctx context.Context, id *string) (store.Account, error) {
	return f.realStore.GetAccountByGitHubUserID(ctx, id)
}
func (f *failingInsertLockStore) UpdateAccountDisplayName(ctx context.Context, p store.UpdateAccountDisplayNameParams) error {
	return f.realStore.UpdateAccountDisplayName(ctx, p)
}

// WithTx delegates to the real store's transaction. The real store opens a tx
// and calls fn — fn calls InsertFinalizeLock which this wrapper intercepts.
// The error propagates from fn → tx rollback, so no partial state is committed.
func (f *failingInsertLockStore) WithTx(ctx context.Context, fn func(store.TxStore) error) error {
	// Wrap: call the real WithTx but inject the failure by wrapping the TxStore.
	return f.realStore.WithTx(ctx, func(tx store.TxStore) error {
		return fn(&failingInsertTxStore{TxStore: tx, failErr: f.failErr})
	})
}

// failingInsertTxStore wraps a TxStore and overrides InsertFinalizeLock to fail.
type failingInsertTxStore struct {
	store.TxStore
	failErr error
}

func (f *failingInsertTxStore) InsertFinalizeLock(_ context.Context, _ store.InsertFinalizeLockParams) error {
	return f.failErr
}

// --- test: rollback on mid-sequence failure ---

// TestAcquireFinalizeLock_TxRollbackOnMidSequenceFailure seeds a stale lock
// (branch 3), injects a failure on InsertFinalizeLock (step 2), and asserts:
//  1. AcquireFinalizeLock returns a non-nil Go error (dep-wrapped).
//  2. The stale lock row's released_at is NIL — the earlier ReleaseFinalizeLock
//     inside the tx was rolled back.
//  3. No new lock row was created.
func TestAcquireFinalizeLock_TxRollbackOnMidSequenceFailure(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	// Seed a stale lock held by `other` so the handler takes branch 3.
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

	// Inject a failure on InsertFinalizeLock (step 2 of the sequence: after
	// the stale lock is released but before the new lock is created).
	injectedErr := errors.New("simulated db failure on insert")
	failStore := &failingInsertLockStore{realStore: env.store, failErr: injectedErr}
	depHandler := newFinalizeHandlerWith(t, failStore, env.store)

	_, err := depHandler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err == nil {
		t.Fatal("expected a non-nil error from the failing handler, got nil")
	}

	// The stale lock's released_at must still be nil — the ReleaseFinalizeLock
	// inside the tx was rolled back when InsertFinalizeLock failed.
	staleLock, err2 := env.store.GetFinalizeLockByID(ctx, staleLockID)
	if err2 != nil {
		t.Fatalf("GetFinalizeLockByID: %v", err2)
	}
	if staleLock.ReleasedAt != nil {
		t.Errorf("stale lock.released_at = %v; expected nil (rollback should have undone the release)", staleLock.ReleasedAt)
	}

	// The stale lock should be the only (active) lock — the new lock was never
	// committed. GetActiveFinalizeLockForSession must return the stale lock with
	// released_at == nil (proving the rollback restored it to its prior state).
	activeLock, activeErr := env.store.GetActiveFinalizeLockForSession(ctx, env.sessID)
	if activeErr != nil {
		t.Fatalf("expected the stale lock to still be active after rollback, got error: %v", activeErr)
	}
	if activeLock.ID != staleLockID {
		t.Errorf("active lock ID = %q, want stale lock %q (new lock must not have been committed)", activeLock.ID, staleLockID)
	}
	if activeLock.ReleasedAt != nil {
		t.Errorf("stale lock.released_at = %v on active-lock read; expected nil", activeLock.ReleasedAt)
	}
}

// TestAcquireFinalizeLock_UniqueViolationReturns409Regardless tests that a
// unique-violation on InsertFinalizeLock (fresh-insert race — no prior
// ReleaseFinalizeLock done) still returns a 409 rather than a Go error.
// This covers the path where supersedeOldID is "" (the codex must-fix).
func TestAcquireFinalizeLock_UniqueViolationReturns409Regardless(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	// Seed an active lock directly (as if a concurrent caller just inserted it).
	now := time.Now().UTC()
	freshID := ulid.Make().String()
	if err := env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID:                  freshID,
		OrgID:               env.orgID,
		SessionID:           env.sessID,
		AcquiredByAccountID: env.otherID, // other holds it
		AcquiredAt:          now,
		LastActivityAt:      now,
		SelectedCommitSHAs:  "[]",
		Mode:                "squash",
	}); err != nil {
		t.Fatalf("seed active lock: %v", err)
	}

	// Inject the unique-violation directly so we don't need to race.
	uvStore := &failingInsertLockStore{realStore: env.store, failErr: store.ErrUniqueViolation}
	depHandler := newFinalizeHandlerWith(t, uvStore, env.store)

	// The handler reads the existing lock first (sees otherID holds it → fresh
	// → caller has no lock; pre-flight read returns hadExisting=true, not override).
	// But we inject ErrUniqueViolation on InsertFinalizeLock.
	// Since !override, the handler returns 409 (lock_held_by_other) from the
	// pre-flight path — so let's instead test with no pre-existing lock so the
	// handler goes straight to the tx path and hits the unique violation.
	_ = depHandler

	// Use a fresh env with no pre-existing lock.
	env2 := newFinalizeEnv(t)
	uvStore2 := &failingInsertLockStore{realStore: env2.store, failErr: store.ErrUniqueViolation}
	depHandler2 := newFinalizeHandlerWith(t, uvStore2, env2.store)

	resp, err := depHandler2.AcquireFinalizeLock(env2.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env2.orgID,
		SessionID: env2.sessID,
	})
	if err != nil {
		t.Fatalf("expected 409 response not a Go error, got err: %v", err)
	}
	if _, ok := resp.(openapi.AcquireFinalizeLock409JSONResponse); !ok {
		t.Errorf("expected 409 JSON response, got %T", resp)
	}
}
