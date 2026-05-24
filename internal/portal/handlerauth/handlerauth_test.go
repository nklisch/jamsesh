package handlerauth_test

import (
	"context"
	"errors"
	"testing"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/handlerauth"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Minimal store stub
//
// stubStore implements only store.OrgMemberStore and store.SessionMemberStore —
// the two narrow interfaces that handlerauth.RequireOrgMember and
// RequireSessionMember accept. Every non-overridden method panics to surface
// accidental calls.
// ---------------------------------------------------------------------------

type stubStore struct {
	getOrgMemberFn     func(ctx context.Context, arg store.GetOrgMemberParams) (store.OrgMember, error)
	getSessionMemberFn func(ctx context.Context, arg store.GetSessionMemberParams) (store.SessionMember, error)
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
func (s *stubStore) NicknameTakenInSession(_ context.Context, _ store.NicknameTakenInSessionParams) (bool, error) {
	panic("not implemented")
}
func (s *stubStore) CountSessionMembers(_ context.Context, _ store.CountSessionMembersParams) (int64, error) {
	panic("not implemented")
}

// Compile-time interface checks: stubStore satisfies both narrow interfaces.
var _ store.OrgMemberStore = (*stubStore)(nil)
var _ store.SessionMemberStore = (*stubStore)(nil)

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
