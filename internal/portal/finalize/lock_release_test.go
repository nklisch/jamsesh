package finalize_test

import (
	"context"
	"errors"
	"testing"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
)

func TestReleaseFinalizeLock_HappyPath(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	// Acquire to set up sessions pointer + status.
	if _, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	}); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	row, _ := env.store.GetActiveFinalizeLockForSession(ctx, env.sessID)

	resp, err := env.handler.ReleaseFinalizeLock(env.callerCtx, openapi.ReleaseFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		LockID:    row.ID,
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, ok := resp.(openapi.ReleaseFinalizeLock204Response); !ok {
		t.Fatalf("expected 204, got %T", resp)
	}

	// Lock row released_at set.
	row2, _ := env.store.GetFinalizeLockByID(ctx, row.ID)
	if row2.ReleasedAt == nil {
		t.Error("released_at not set")
	}

	// Sessions pointer cleared.
	sess, _ := env.store.GetSession(ctx, env.orgID, env.sessID)
	if sess.FinalizeLockedByAccountID != nil {
		t.Errorf("FinalizeLockedByAccountID = %v, want nil", sess.FinalizeLockedByAccountID)
	}

	// Session status STAYS finalizing — release is not abandon.
	if sess.Status != "finalizing" {
		t.Errorf("session.status = %q, want finalizing", sess.Status)
	}
}

func TestReleaseFinalizeLock_Idempotent(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	if _, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	}); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	row, _ := env.store.GetActiveFinalizeLockForSession(ctx, env.sessID)

	for i := 0; i < 3; i++ {
		resp, err := env.handler.ReleaseFinalizeLock(env.callerCtx, openapi.ReleaseFinalizeLockRequestObject{
			OrgID:     env.orgID,
			SessionID: env.sessID,
			LockID:    row.ID,
		})
		if err != nil {
			t.Fatalf("release #%d: %v", i, err)
		}
		if _, ok := resp.(openapi.ReleaseFinalizeLock204Response); !ok {
			t.Errorf("release #%d: expected 204, got %T", i, resp)
		}
	}
}

func TestReleaseFinalizeLock_NonCaller_403(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	if _, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	}); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	row, _ := env.store.GetActiveFinalizeLockForSession(ctx, env.sessID)

	resp, err := env.handler.ReleaseFinalizeLock(env.otherCtx, openapi.ReleaseFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		LockID:    row.ID,
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, ok := resp.(openapi.ReleaseFinalizeLock403JSONResponse); !ok {
		t.Fatalf("expected 403, got %T", resp)
	}

	// Lock still active.
	row2, _ := env.store.GetFinalizeLockByID(ctx, row.ID)
	if row2.ReleasedAt != nil {
		t.Error("lock should not be released by non-caller attempt")
	}
}

// Build-time check that storage stub satisfies storage.Service.
var _ store.FinalizeLock // keeps store import live in this file

// ---------------------------------------------------------------------------
// Dep-failure test
// ---------------------------------------------------------------------------

// failingReleaseLockStore wraps a real store and returns a transient error
// from ReleaseFinalizeLock, simulating a DB connection failure during the
// write-side of the release flow (after the lock has been read).
//
// Implements finalizeStore (FinalizeLockStore + SessionStore + SessionMemberStore
// + OrgMemberStore + AccountStore), delegating all methods except
// ReleaseFinalizeLock to the real store.
type failingReleaseLockStore struct {
	realStore store.Store
}

// FinalizeLockStore delegation (ReleaseFinalizeLock overridden below)
func (f *failingReleaseLockStore) InsertFinalizeLock(ctx context.Context, p store.InsertFinalizeLockParams) error {
	return f.realStore.InsertFinalizeLock(ctx, p)
}
func (f *failingReleaseLockStore) GetFinalizeLockByID(ctx context.Context, id string) (store.FinalizeLock, error) {
	return f.realStore.GetFinalizeLockByID(ctx, id)
}
func (f *failingReleaseLockStore) GetActiveFinalizeLockForSession(ctx context.Context, sessionID string) (store.FinalizeLock, error) {
	return f.realStore.GetActiveFinalizeLockForSession(ctx, sessionID)
}
func (f *failingReleaseLockStore) UpdateFinalizeLockCuration(ctx context.Context, p store.UpdateFinalizeLockCurationParams) error {
	return f.realStore.UpdateFinalizeLockCuration(ctx, p)
}
func (f *failingReleaseLockStore) TouchFinalizeLock(ctx context.Context, p store.TouchFinalizeLockParams) error {
	return f.realStore.TouchFinalizeLock(ctx, p)
}
func (f *failingReleaseLockStore) ReleaseFinalizeLock(_ context.Context, _ store.ReleaseFinalizeLockParams) error {
	return errors.New("conn refused")
}
func (f *failingReleaseLockStore) SupersedeFinalizeLock(ctx context.Context, p store.SupersedeFinalizeLockParams) error {
	return f.realStore.SupersedeFinalizeLock(ctx, p)
}

// SessionStore delegation
func (f *failingReleaseLockStore) CreateSession(ctx context.Context, p store.CreateSessionParams) (store.Session, error) {
	return f.realStore.CreateSession(ctx, p)
}
func (f *failingReleaseLockStore) GetSession(ctx context.Context, orgID, id string) (store.Session, error) {
	return f.realStore.GetSession(ctx, orgID, id)
}
func (f *failingReleaseLockStore) GetSessionByID(ctx context.Context, id string) (store.Session, error) {
	return f.realStore.GetSessionByID(ctx, id)
}
func (f *failingReleaseLockStore) ListSessionsForOrg(ctx context.Context, orgID string) ([]store.Session, error) {
	return f.realStore.ListSessionsForOrg(ctx, orgID)
}
func (f *failingReleaseLockStore) ListSessionsForOrgWithCursor(ctx context.Context, p store.ListSessionsForOrgWithCursorParams) ([]store.Session, error) {
	return f.realStore.ListSessionsForOrgWithCursor(ctx, p)
}
func (f *failingReleaseLockStore) UpdateSessionStatus(ctx context.Context, p store.UpdateSessionStatusParams) error {
	return f.realStore.UpdateSessionStatus(ctx, p)
}
func (f *failingReleaseLockStore) UpdateSessionGoalScopeMode(ctx context.Context, p store.UpdateSessionGoalScopeModeParams) error {
	return f.realStore.UpdateSessionGoalScopeMode(ctx, p)
}
func (f *failingReleaseLockStore) SetSessionBaseSHA(ctx context.Context, p store.SetSessionBaseSHAParams) error {
	return f.realStore.SetSessionBaseSHA(ctx, p)
}
func (f *failingReleaseLockStore) SetSessionEndReason(ctx context.Context, p store.SetSessionEndReasonParams) error {
	return f.realStore.SetSessionEndReason(ctx, p)
}
func (f *failingReleaseLockStore) SetFinalizeLock(ctx context.Context, p store.SetFinalizeLockParams) error {
	return f.realStore.SetFinalizeLock(ctx, p)
}
func (f *failingReleaseLockStore) ClearFinalizeLock(ctx context.Context, p store.ClearFinalizeLockParams) error {
	return f.realStore.ClearFinalizeLock(ctx, p)
}
func (f *failingReleaseLockStore) DeleteSession(ctx context.Context, p store.DeleteSessionParams) error {
	return f.realStore.DeleteSession(ctx, p)
}

// SessionMemberStore delegation
func (f *failingReleaseLockStore) AddSessionMember(ctx context.Context, p store.AddSessionMemberParams) error {
	return f.realStore.AddSessionMember(ctx, p)
}
func (f *failingReleaseLockStore) GetSessionMember(ctx context.Context, p store.GetSessionMemberParams) (store.SessionMember, error) {
	return f.realStore.GetSessionMember(ctx, p)
}
func (f *failingReleaseLockStore) ListSessionMembers(ctx context.Context, p store.ListSessionMembersParams) ([]store.SessionMember, error) {
	return f.realStore.ListSessionMembers(ctx, p)
}
func (f *failingReleaseLockStore) RemoveSessionMember(ctx context.Context, p store.RemoveSessionMemberParams) error {
	return f.realStore.RemoveSessionMember(ctx, p)
}
func (f *failingReleaseLockStore) ListSessionMembershipsForAccount(ctx context.Context, accountID string) ([]store.SessionMembership, error) {
	return f.realStore.ListSessionMembershipsForAccount(ctx, accountID)
}
func (f *failingReleaseLockStore) NicknameTakenInSession(ctx context.Context, p store.NicknameTakenInSessionParams) (bool, error) {
	return f.realStore.NicknameTakenInSession(ctx, p)
}
func (f *failingReleaseLockStore) CountSessionMembers(ctx context.Context, p store.CountSessionMembersParams) (int64, error) {
	return f.realStore.CountSessionMembers(ctx, p)
}

// OrgMemberStore delegation
func (f *failingReleaseLockStore) AddOrgMember(ctx context.Context, p store.AddOrgMemberParams) error {
	return f.realStore.AddOrgMember(ctx, p)
}
func (f *failingReleaseLockStore) GetOrgMember(ctx context.Context, p store.GetOrgMemberParams) (store.OrgMember, error) {
	return f.realStore.GetOrgMember(ctx, p)
}
func (f *failingReleaseLockStore) ListOrgsForAccount(ctx context.Context, accountID string) ([]store.Org, error) {
	return f.realStore.ListOrgsForAccount(ctx, accountID)
}
func (f *failingReleaseLockStore) ListOrgMembers(ctx context.Context, orgID string) ([]store.OrgMemberWithAccount, error) {
	return f.realStore.ListOrgMembers(ctx, orgID)
}
func (f *failingReleaseLockStore) RemoveOrgMember(ctx context.Context, p store.RemoveOrgMemberParams) error {
	return f.realStore.RemoveOrgMember(ctx, p)
}

// AccountStore delegation
func (f *failingReleaseLockStore) CreateAccount(ctx context.Context, p store.CreateAccountParams) (store.Account, error) {
	return f.realStore.CreateAccount(ctx, p)
}
func (f *failingReleaseLockStore) CreateAnonymousAccount(ctx context.Context, p store.CreateAnonymousAccountParams) (store.Account, error) {
	return f.realStore.CreateAnonymousAccount(ctx, p)
}
func (f *failingReleaseLockStore) GetAccountByID(ctx context.Context, id string) (store.Account, error) {
	return f.realStore.GetAccountByID(ctx, id)
}
func (f *failingReleaseLockStore) GetAccountByEmail(ctx context.Context, email string) (store.Account, error) {
	return f.realStore.GetAccountByEmail(ctx, email)
}
func (f *failingReleaseLockStore) GetAccountByGitHubUserID(ctx context.Context, id *string) (store.Account, error) {
	return f.realStore.GetAccountByGitHubUserID(ctx, id)
}
func (f *failingReleaseLockStore) UpdateAccountDisplayName(ctx context.Context, p store.UpdateAccountDisplayNameParams) error {
	return f.realStore.UpdateAccountDisplayName(ctx, p)
}
func (f *failingReleaseLockStore) WithTx(ctx context.Context, fn func(store.TxStore) error) error {
	return f.realStore.WithTx(ctx, fn)
}

func TestReleaseFinalizeLock_DBUnavailable_WrapsAsDepDB(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	// Acquire to set up the active lock row in the underlying store.
	if _, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	}); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	row, _ := env.store.GetActiveFinalizeLockForSession(ctx, env.sessID)

	// Build a fresh handler against a wrapping store that fails the
	// release-write call. The strict-handler translator (wired in
	// production via cmd/portal/main.go) turns the dep-wrapped error
	// into a 503 envelope; here we assert that the wrap is in place so
	// the translator has the right input.
	depHandler := newFinalizeHandlerWith(t, &failingReleaseLockStore{realStore: env.store}, env.store)

	_, err := depHandler.ReleaseFinalizeLock(env.callerCtx, openapi.ReleaseFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		LockID:    row.ID,
	})
	if err == nil {
		t.Fatalf("expected dep-wrapped error, got nil")
	}
	if !errors.Is(err, deperr.ErrDB) {
		t.Errorf("expected errors.Is(err, deperr.ErrDB) = true, got err=%v", err)
	}
}
