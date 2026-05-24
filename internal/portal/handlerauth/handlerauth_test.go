package handlerauth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/handlerauth"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Minimal store stub
//
// stubStore satisfies store.Store but only implements GetOrgMember and
// GetSessionMember. Every other method panics to surface accidental calls.
// ---------------------------------------------------------------------------

type stubStore struct {
	getOrgMemberFn     func(ctx context.Context, arg store.GetOrgMemberParams) (store.OrgMember, error)
	getSessionMemberFn func(ctx context.Context, arg store.GetSessionMemberParams) (store.SessionMember, error)
}

// --- OrgStore ---

func (s *stubStore) CreateOrg(_ context.Context, _ store.CreateOrgParams) (store.Org, error) {
	panic("not implemented")
}
func (s *stubStore) CreateProtectedOrg(_ context.Context, _ store.CreateProtectedOrgParams) (store.Org, error) {
	panic("not implemented")
}
func (s *stubStore) GetOrgByID(_ context.Context, _ string) (store.Org, error) {
	panic("not implemented")
}
func (s *stubStore) GetOrgBySlug(_ context.Context, _ string) (store.Org, error) {
	panic("not implemented")
}
func (s *stubStore) UpdateOrgSessionInvitePolicy(_ context.Context, _ store.UpdateOrgSessionInvitePolicyParams) error {
	panic("not implemented")
}

// --- AccountStore ---

func (s *stubStore) CreateAccount(_ context.Context, _ store.CreateAccountParams) (store.Account, error) {
	panic("not implemented")
}
func (s *stubStore) CreateAnonymousAccount(_ context.Context, _ store.CreateAnonymousAccountParams) (store.Account, error) {
	panic("not implemented")
}
func (s *stubStore) GetAccountByID(_ context.Context, _ string) (store.Account, error) {
	panic("not implemented")
}
func (s *stubStore) GetAccountByEmail(_ context.Context, _ string) (store.Account, error) {
	panic("not implemented")
}
func (s *stubStore) GetAccountByGitHubUserID(_ context.Context, _ *string) (store.Account, error) {
	panic("not implemented")
}
func (s *stubStore) UpdateAccountDisplayName(_ context.Context, _ store.UpdateAccountDisplayNameParams) error {
	panic("not implemented")
}

// --- OrgMemberStore ---

func (s *stubStore) AddOrgMember(_ context.Context, _ store.AddOrgMemberParams) error {
	panic("not implemented")
}
func (s *stubStore) GetOrgMember(ctx context.Context, arg store.GetOrgMemberParams) (store.OrgMember, error) {
	if s.getOrgMemberFn == nil {
		panic("stubStore.GetOrgMember called unexpectedly")
	}
	return s.getOrgMemberFn(ctx, arg)
}
func (s *stubStore) ListOrgsForAccount(_ context.Context, _ string) ([]store.Org, error) {
	panic("not implemented")
}
func (s *stubStore) ListOrgMembers(_ context.Context, _ string) ([]store.OrgMemberWithAccount, error) {
	panic("not implemented")
}
func (s *stubStore) RemoveOrgMember(_ context.Context, _ store.RemoveOrgMemberParams) error {
	panic("not implemented")
}

// --- SessionStore ---

func (s *stubStore) CreateSession(_ context.Context, _ store.CreateSessionParams) (store.Session, error) {
	panic("not implemented")
}
func (s *stubStore) GetSession(_ context.Context, _, _ string) (store.Session, error) {
	panic("not implemented")
}
func (s *stubStore) GetSessionByID(_ context.Context, _ string) (store.Session, error) {
	panic("not implemented")
}
func (s *stubStore) ListSessionsForOrg(_ context.Context, _ string) ([]store.Session, error) {
	panic("not implemented")
}
func (s *stubStore) ListSessionsForOrgWithCursor(_ context.Context, _ store.ListSessionsForOrgWithCursorParams) ([]store.Session, error) {
	panic("not implemented")
}
func (s *stubStore) UpdateSessionStatus(_ context.Context, _ store.UpdateSessionStatusParams) error {
	panic("not implemented")
}
func (s *stubStore) UpdateSessionGoalScopeMode(_ context.Context, _ store.UpdateSessionGoalScopeModeParams) error {
	panic("not implemented")
}
func (s *stubStore) SetSessionBaseSHA(_ context.Context, _ store.SetSessionBaseSHAParams) error {
	panic("not implemented")
}
func (s *stubStore) SetSessionEndReason(_ context.Context, _ store.SetSessionEndReasonParams) error {
	panic("not implemented")
}
func (s *stubStore) SetFinalizeLock(_ context.Context, _ store.SetFinalizeLockParams) error {
	panic("not implemented")
}
func (s *stubStore) ClearFinalizeLock(_ context.Context, _ store.ClearFinalizeLockParams) error {
	panic("not implemented")
}
func (s *stubStore) DeleteSession(_ context.Context, _ store.DeleteSessionParams) error {
	panic("not implemented")
}

// --- RefModeStore ---

func (s *stubStore) UpsertRefMode(_ context.Context, _ store.UpsertRefModeParams) error {
	panic("not implemented")
}
func (s *stubStore) GetRefMode(_ context.Context, _ store.GetRefModeParams) (store.RefMode, error) {
	panic("not implemented")
}
func (s *stubStore) ListRefModesForSession(_ context.Context, _ string) ([]store.RefMode, error) {
	panic("not implemented")
}

// --- SessionMemberStore ---

func (s *stubStore) AddSessionMember(_ context.Context, _ store.AddSessionMemberParams) error {
	panic("not implemented")
}
func (s *stubStore) GetSessionMember(ctx context.Context, arg store.GetSessionMemberParams) (store.SessionMember, error) {
	if s.getSessionMemberFn == nil {
		panic("stubStore.GetSessionMember called unexpectedly")
	}
	return s.getSessionMemberFn(ctx, arg)
}
func (s *stubStore) ListSessionMembers(_ context.Context, _ store.ListSessionMembersParams) ([]store.SessionMember, error) {
	panic("not implemented")
}
func (s *stubStore) RemoveSessionMember(_ context.Context, _ store.RemoveSessionMemberParams) error {
	panic("not implemented")
}
func (s *stubStore) ListSessionMembershipsForAccount(_ context.Context, _ string) ([]store.SessionMembership, error) {
	panic("not implemented")
}

// --- ArchivedSessionStore ---

func (s *stubStore) InsertArchivedSession(_ context.Context, _ store.InsertArchivedSessionParams) error {
	panic("not implemented")
}
func (s *stubStore) GetArchivedSession(_ context.Context, _ store.GetArchivedSessionParams) (store.ArchivedSession, error) {
	panic("not implemented")
}

// --- OAuthTokenStore ---

func (s *stubStore) CreateOAuthToken(_ context.Context, _ store.CreateOAuthTokenParams) (store.OAuthToken, error) {
	panic("not implemented")
}
func (s *stubStore) GetOAuthTokenByHash(_ context.Context, _ string) (store.OAuthToken, error) {
	panic("not implemented")
}
func (s *stubStore) TouchOAuthTokenLastUsed(_ context.Context, _ store.TouchOAuthTokenLastUsedParams) error {
	panic("not implemented")
}
func (s *stubStore) RevokeOAuthToken(_ context.Context, _ store.RevokeOAuthTokenParams) error {
	panic("not implemented")
}
func (s *stubStore) RevokeAllOAuthTokensForAccount(_ context.Context, _ store.RevokeAllOAuthTokensForAccountParams) error {
	panic("not implemented")
}
func (s *stubStore) ListOAuthTokensForAccount(_ context.Context, _ string) ([]store.OAuthToken, error) {
	panic("not implemented")
}
func (s *stubStore) CreateAnonymousBearer(_ context.Context, _ store.CreateAnonymousBearerParams) (store.OAuthToken, error) {
	panic("not implemented")
}
func (s *stubStore) RevokeBearersForSession(_ context.Context, _ store.RevokeBearersForSessionParams) error {
	panic("not implemented")
}

// --- MagicLinkTokenStore ---

func (s *stubStore) CreateMagicLinkToken(_ context.Context, _ store.CreateMagicLinkTokenParams) (store.MagicLinkToken, error) {
	panic("not implemented")
}
func (s *stubStore) GetMagicLinkTokenByHash(_ context.Context, _ string) (store.MagicLinkToken, error) {
	panic("not implemented")
}
func (s *stubStore) ConsumeMagicLinkToken(_ context.Context, _ store.ConsumeMagicLinkTokenParams) error {
	panic("not implemented")
}

// --- EventLogStore ---

func (s *stubStore) EnsureEventSeqRow(_ context.Context, _ string) error {
	panic("not implemented")
}
func (s *stubStore) AllocateNextSeq(_ context.Context, _ string) (int64, error) {
	panic("not implemented")
}
func (s *stubStore) AllocateNextSeqN(_ context.Context, _ string, _ int64) (int64, error) {
	panic("not implemented")
}
func (s *stubStore) InsertEvent(_ context.Context, _ store.InsertEventParams) error {
	panic("not implemented")
}
func (s *stubStore) ListEventsSince(_ context.Context, _ store.ListEventsSinceParams) ([]store.Event, error) {
	panic("not implemented")
}
func (s *stubStore) ListEventsSinceForDigest(_ context.Context, _ store.ListEventsSinceForDigestParams) ([]store.Event, error) {
	panic("not implemented")
}

// --- PresenceStore ---

func (s *stubStore) UpsertPresence(_ context.Context, _ store.UpsertPresenceParams) error {
	panic("not implemented")
}
func (s *stubStore) ListPresenceForSession(_ context.Context, _ string) ([]store.PresenceRow, error) {
	panic("not implemented")
}

// --- OrgInviteStore ---

func (s *stubStore) InsertOrgInvite(_ context.Context, _ store.InsertOrgInviteParams) (store.OrgInvite, error) {
	panic("not implemented")
}
func (s *stubStore) GetOrgInviteByID(_ context.Context, _ string) (store.OrgInvite, error) {
	panic("not implemented")
}
func (s *stubStore) GetOrgInviteByTokenHash(_ context.Context, _ string) (store.OrgInvite, error) {
	panic("not implemented")
}
func (s *stubStore) MarkOrgInviteAccepted(_ context.Context, _ store.MarkOrgInviteAcceptedParams) error {
	panic("not implemented")
}
func (s *stubStore) ListPendingOrgInvitesForOrg(_ context.Context, _ store.ListPendingOrgInvitesForOrgParams) ([]store.OrgInvite, error) {
	panic("not implemented")
}
func (s *stubStore) ListPendingOrgInvitesForEmail(_ context.Context, _ store.ListPendingOrgInvitesForEmailParams) ([]store.OrgInvite, error) {
	panic("not implemented")
}

// --- SessionInviteStore ---

func (s *stubStore) InsertSessionInvite(_ context.Context, _ store.InsertSessionInviteParams) (store.SessionInvite, error) {
	panic("not implemented")
}
func (s *stubStore) GetSessionInviteByID(_ context.Context, _ string) (store.SessionInvite, error) {
	panic("not implemented")
}
func (s *stubStore) GetSessionInviteByTokenHash(_ context.Context, _ string) (store.SessionInvite, error) {
	panic("not implemented")
}
func (s *stubStore) MarkSessionInviteAccepted(_ context.Context, _ store.MarkSessionInviteAcceptedParams) error {
	panic("not implemented")
}
func (s *stubStore) ListPendingSessionInvitesForSession(_ context.Context, _ store.ListPendingSessionInvitesForSessionParams) ([]store.SessionInvite, error) {
	panic("not implemented")
}

// --- ConflictEventStore ---

func (s *stubStore) InsertConflictEvent(_ context.Context, _ store.InsertConflictEventParams) error {
	panic("not implemented")
}
func (s *stubStore) GetConflictEventByID(_ context.Context, _ string) (store.ConflictEvent, error) {
	panic("not implemented")
}
func (s *stubStore) MarkConflictEventResolved(_ context.Context, _ store.MarkConflictEventResolvedParams) error {
	panic("not implemented")
}
func (s *stubStore) ListOpenConflictEventsForSession(_ context.Context, _ string) ([]store.ConflictEvent, error) {
	panic("not implemented")
}

// --- OAuthStateStore ---

func (s *stubStore) InsertOAuthState(_ context.Context, _ store.InsertOAuthStateParams) error {
	panic("not implemented")
}
func (s *stubStore) ConsumeOAuthState(_ context.Context, _ string) (store.OAuthState, error) {
	panic("not implemented")
}
func (s *stubStore) CleanupExpiredOAuthState(_ context.Context, _ time.Time) error {
	panic("not implemented")
}

// --- CommentStore ---

func (s *stubStore) InsertComment(_ context.Context, _ store.InsertCommentParams) error {
	panic("not implemented")
}
func (s *stubStore) GetCommentByID(_ context.Context, _ string) (store.Comment, error) {
	panic("not implemented")
}
func (s *stubStore) ResolveComment(_ context.Context, _ store.ResolveCommentParams) error {
	panic("not implemented")
}
func (s *stubStore) ListCommentsForSession(_ context.Context, _ store.ListCommentsForSessionParams) ([]store.Comment, error) {
	panic("not implemented")
}

// --- FinalizeLockStore ---

func (s *stubStore) InsertFinalizeLock(_ context.Context, _ store.InsertFinalizeLockParams) error {
	panic("not implemented")
}
func (s *stubStore) GetFinalizeLockByID(_ context.Context, _ string) (store.FinalizeLock, error) {
	panic("not implemented")
}
func (s *stubStore) GetActiveFinalizeLockForSession(_ context.Context, _ string) (store.FinalizeLock, error) {
	panic("not implemented")
}
func (s *stubStore) UpdateFinalizeLockCuration(_ context.Context, _ store.UpdateFinalizeLockCurationParams) error {
	panic("not implemented")
}
func (s *stubStore) TouchFinalizeLock(_ context.Context, _ store.TouchFinalizeLockParams) error {
	panic("not implemented")
}
func (s *stubStore) ReleaseFinalizeLock(_ context.Context, _ store.ReleaseFinalizeLockParams) error {
	panic("not implemented")
}
func (s *stubStore) SupersedeFinalizeLock(_ context.Context, _ store.SupersedeFinalizeLockParams) error {
	panic("not implemented")
}

// --- LeaseStore ---

func (s *stubStore) IssueLeaseFencingToken(_ context.Context) (int64, error)             { panic("not implemented") }
func (s *stubStore) InsertLease(_ context.Context, _ store.InsertLeaseParams) (store.Lease, error) {
	panic("not implemented")
}
func (s *stubStore) MarkLeaseReleased(_ context.Context, _ string) error                 { panic("not implemented") }
func (s *stubStore) UpdateLeaseHeartbeat(_ context.Context, _ string) error              { panic("not implemented") }
func (s *stubStore) DeleteReleasedLeasesOlderThan(_ context.Context, _ time.Time) error  { panic("not implemented") }

// --- Store-level methods ---

func (s *stubStore) WithTx(_ context.Context, _ func(store.TxStore) error) error {
	panic("not implemented")
}
func (s *stubStore) Ping(_ context.Context) error { panic("not implemented") }
func (s *stubStore) Close() error                  { return nil }
func (s *stubStore) Dialect() string               { return "stub" }

// --- SessionMemberStore (playground additions) ---

func (s *stubStore) NicknameTakenInSession(_ context.Context, _ store.NicknameTakenInSessionParams) (bool, error) {
	panic("not implemented")
}
func (s *stubStore) CountSessionMembers(_ context.Context, _ store.CountSessionMembersParams) (int64, error) {
	panic("not implemented")
}

// --- TombstoneStore ---

func (s *stubStore) GetTombstone(_ context.Context, _ string) (store.Tombstone, error) {
	panic("not implemented")
}
func (s *stubStore) RecordTombstone(_ context.Context, _ store.RecordTombstoneParams) error {
	panic("not implemented")
}

// --- PlaygroundSessionStore ---

func (s *stubStore) ResetSessionIdleTimer(_ context.Context, _ store.ResetSessionIdleTimerParams) error {
	panic("not implemented")
}

// Compile-time interface check.
var _ store.Store = (*stubStore)(nil)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func accountCtx(acc *store.Account) context.Context {
	return tokens.ContextWithAccount(context.Background(), acc)
}

func testAccount() *store.Account {
	return &store.Account{ID: "acct-001", Email: "alice@example.com", DisplayName: "Alice"}
}

// ---------------------------------------------------------------------------
// RequireAccount
// ---------------------------------------------------------------------------

func TestRequireAccount_NoAccount(t *testing.T) {
	ctx := context.Background()
	acc, fail, ok := handlerauth.RequireAccount(ctx)
	if ok {
		t.Fatal("want ok=false when no account in context")
	}
	if acc != nil {
		t.Error("want nil account on failure")
	}
	if fail.Status != 401 {
		t.Errorf("want status 401, got %d", fail.Status)
	}
	if fail.Unauthorized.Error != "auth.invalid_token" {
		t.Errorf("want error=auth.invalid_token, got %q", fail.Unauthorized.Error)
	}
	if fail.Unauthorized.Message != "invalid token" {
		t.Errorf("want message='invalid token', got %q", fail.Unauthorized.Message)
	}
}

func TestRequireAccount_WithAccount(t *testing.T) {
	want := testAccount()
	ctx := accountCtx(want)
	acc, fail, ok := handlerauth.RequireAccount(ctx)
	if !ok {
		t.Fatal("want ok=true when account is in context")
	}
	if acc == nil || acc.ID != want.ID {
		t.Errorf("want account %q, got %v", want.ID, acc)
	}
	if fail.Status != 0 {
		t.Errorf("want zero AuthFail on success, got status %d", fail.Status)
	}
}

// ---------------------------------------------------------------------------
// RequireOrgMember
// ---------------------------------------------------------------------------

func TestRequireOrgMember_NoAccount(t *testing.T) {
	ctx := context.Background()
	s := &stubStore{} // GetOrgMember should not be called
	_, _, fail, ok := handlerauth.RequireOrgMember(ctx, s, "org-1")
	if ok {
		t.Fatal("want ok=false when no account in context")
	}
	if fail.Status != 401 {
		t.Errorf("want status 401, got %d", fail.Status)
	}
}

func TestRequireOrgMember_OrgNotFound(t *testing.T) {
	want := testAccount()
	ctx := accountCtx(want)
	s := &stubStore{
		getOrgMemberFn: func(_ context.Context, _ store.GetOrgMemberParams) (store.OrgMember, error) {
			return store.OrgMember{}, store.ErrNotFound
		},
	}
	_, _, fail, ok := handlerauth.RequireOrgMember(ctx, s, "org-1")
	if ok {
		t.Fatal("want ok=false when org member not found")
	}
	if fail.Status != 403 {
		t.Errorf("want status 403, got %d", fail.Status)
	}
	if fail.Forbidden.Error != "auth.insufficient_permission" {
		t.Errorf("want error=auth.insufficient_permission, got %q", fail.Forbidden.Error)
	}
	if fail.Forbidden.Message != "not a member of this org" {
		t.Errorf("want message='not a member of this org', got %q", fail.Forbidden.Message)
	}
}

func TestRequireOrgMember_StoreError(t *testing.T) {
	want := testAccount()
	ctx := accountCtx(want)
	storeErr := errors.New("database is on fire")
	s := &stubStore{
		getOrgMemberFn: func(_ context.Context, _ store.GetOrgMemberParams) (store.OrgMember, error) {
			return store.OrgMember{}, storeErr
		},
	}
	_, _, fail, ok := handlerauth.RequireOrgMember(ctx, s, "org-1")
	if ok {
		t.Fatal("want ok=false on store error")
	}
	if fail.Status != 500 {
		t.Errorf("want status 500, got %d", fail.Status)
	}
	if !errors.Is(fail.Err, storeErr) {
		t.Errorf("want Err to wrap storeErr, got %v", fail.Err)
	}
}

func TestRequireOrgMember_Success(t *testing.T) {
	want := testAccount()
	ctx := accountCtx(want)
	wantMember := store.OrgMember{OrgID: "org-1", AccountID: want.ID, Role: "member"}
	s := &stubStore{
		getOrgMemberFn: func(_ context.Context, arg store.GetOrgMemberParams) (store.OrgMember, error) {
			if arg.OrgID != "org-1" || arg.AccountID != want.ID {
				t.Errorf("unexpected GetOrgMember args: %+v", arg)
			}
			return wantMember, nil
		},
	}
	acc, member, fail, ok := handlerauth.RequireOrgMember(ctx, s, "org-1")
	if !ok {
		t.Fatal("want ok=true on success")
	}
	if acc == nil || acc.ID != want.ID {
		t.Errorf("want account %q, got %v", want.ID, acc)
	}
	if member.OrgID != wantMember.OrgID || member.AccountID != wantMember.AccountID {
		t.Errorf("unexpected member: %+v", member)
	}
	if fail.Status != 0 {
		t.Errorf("want zero AuthFail on success, got status %d", fail.Status)
	}
}

// ---------------------------------------------------------------------------
// RequireSessionMember
// ---------------------------------------------------------------------------

func TestRequireSessionMember_NoAccount(t *testing.T) {
	ctx := context.Background()
	s := &stubStore{} // GetSessionMember should not be called
	_, _, fail, ok := handlerauth.RequireSessionMember(ctx, s, "org-1", "sess-1")
	if ok {
		t.Fatal("want ok=false when no account in context")
	}
	if fail.Status != 401 {
		t.Errorf("want status 401, got %d", fail.Status)
	}
}

func TestRequireSessionMember_SessionNotFound(t *testing.T) {
	want := testAccount()
	ctx := accountCtx(want)
	s := &stubStore{
		getSessionMemberFn: func(_ context.Context, _ store.GetSessionMemberParams) (store.SessionMember, error) {
			return store.SessionMember{}, store.ErrNotFound
		},
	}
	_, _, fail, ok := handlerauth.RequireSessionMember(ctx, s, "org-1", "sess-1")
	if ok {
		t.Fatal("want ok=false when session member not found")
	}
	if fail.Status != 403 {
		t.Errorf("want status 403, got %d", fail.Status)
	}
	if fail.Forbidden.Error != "auth.insufficient_permission" {
		t.Errorf("want error=auth.insufficient_permission, got %q", fail.Forbidden.Error)
	}
	if fail.Forbidden.Message != "not a member of this session" {
		t.Errorf("want message='not a member of this session', got %q", fail.Forbidden.Message)
	}
}

func TestRequireSessionMember_StoreError(t *testing.T) {
	want := testAccount()
	ctx := accountCtx(want)
	storeErr := errors.New("database timeout")
	s := &stubStore{
		getSessionMemberFn: func(_ context.Context, _ store.GetSessionMemberParams) (store.SessionMember, error) {
			return store.SessionMember{}, storeErr
		},
	}
	_, _, fail, ok := handlerauth.RequireSessionMember(ctx, s, "org-1", "sess-1")
	if ok {
		t.Fatal("want ok=false on store error")
	}
	if fail.Status != 500 {
		t.Errorf("want status 500, got %d", fail.Status)
	}
	if !errors.Is(fail.Err, storeErr) {
		t.Errorf("want Err to wrap storeErr, got %v", fail.Err)
	}
}

func TestRequireSessionMember_Success(t *testing.T) {
	want := testAccount()
	ctx := accountCtx(want)
	wantMember := store.SessionMember{OrgID: "org-1", SessionID: "sess-1", AccountID: want.ID, Role: "creator"}
	s := &stubStore{
		getSessionMemberFn: func(_ context.Context, arg store.GetSessionMemberParams) (store.SessionMember, error) {
			if arg.OrgID != "org-1" || arg.SessionID != "sess-1" || arg.AccountID != want.ID {
				t.Errorf("unexpected GetSessionMember args: %+v", arg)
			}
			return wantMember, nil
		},
	}
	acc, member, fail, ok := handlerauth.RequireSessionMember(ctx, s, "org-1", "sess-1")
	if !ok {
		t.Fatal("want ok=true on success")
	}
	if acc == nil || acc.ID != want.ID {
		t.Errorf("want account %q, got %v", want.ID, acc)
	}
	if member.SessionID != wantMember.SessionID || member.Role != wantMember.Role {
		t.Errorf("unexpected member: %+v", member)
	}
	if fail.Status != 0 {
		t.Errorf("want zero AuthFail on success, got status %d", fail.Status)
	}
}
