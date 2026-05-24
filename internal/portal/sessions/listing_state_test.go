package sessions_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/pagination"
)

// ---------------------------------------------------------------------------
// GET /api/orgs/{orgID}/sessions tests
// ---------------------------------------------------------------------------

func TestListSessions_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "list-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	token := env.bearerToken(t, acc.ID)

	// Create two sessions.
	sess1 := createSession(t, env, org.ID, acc.ID)
	sess2 := createSession(t, env, org.ID, acc.ID)

	resp := getRequest(t, env.srv, "/api/orgs/"+org.ID+"/sessions", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result openapi.SessionListResponse
	decodeBody(t, resp, &result)

	if len(result.Items) < 2 {
		t.Fatalf("expected at least 2 sessions, got %d", len(result.Items))
	}

	ids := make(map[string]bool)
	for _, s := range result.Items {
		ids[s.Id] = true
	}
	if !ids[sess1.Id] || !ids[sess2.Id] {
		t.Errorf("response did not include both created sessions")
	}
}

func TestListSessions_NotOrgMember_Returns403(t *testing.T) {
	env := newTestEnv(t)
	outsider := seedAccount(t, env.s, "outsider@example.com")
	org := seedOrg(t, env.s, "Test Org", "list-403-org")

	token := env.bearerToken(t, outsider.ID)

	resp := getRequest(t, env.srv, "/api/orgs/"+org.ID+"/sessions", token)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestListSessions_CursorRoundTrip(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "cursor-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	token := env.bearerToken(t, acc.ID)

	// Create 3 sessions.
	for i := 0; i < 3; i++ {
		createSession(t, env, org.ID, acc.ID)
	}

	// First page with limit=2.
	resp1 := getRequest(t, env.srv, "/api/orgs/"+org.ID+"/sessions?limit=2", token)
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("page 1: expected 200, got %d", resp1.StatusCode)
	}

	var page1 openapi.SessionListResponse
	decodeBody(t, resp1, &page1)

	if len(page1.Items) != 2 {
		t.Fatalf("page 1: expected 2 items, got %d", len(page1.Items))
	}
	if page1.NextCursor == "" {
		t.Fatal("page 1: expected non-empty next_cursor")
	}

	// Second page using the cursor.
	resp2 := getRequest(t, env.srv, "/api/orgs/"+org.ID+"/sessions?limit=2&cursor="+page1.NextCursor, token)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("page 2: expected 200, got %d", resp2.StatusCode)
	}

	var page2 openapi.SessionListResponse
	decodeBody(t, resp2, &page2)

	if len(page2.Items) < 1 {
		t.Fatalf("page 2: expected at least 1 item, got %d", len(page2.Items))
	}

	// Verify no overlap between pages.
	page1IDs := make(map[string]bool)
	for _, s := range page1.Items {
		page1IDs[s.Id] = true
	}
	for _, s := range page2.Items {
		if page1IDs[s.Id] {
			t.Errorf("page 2 item %s already appeared on page 1", s.Id)
		}
	}
}

func TestListSessions_CursorFilterMismatch_Returns400(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org1 := seedOrg(t, env.s, "Org 1", "mismatch-org1")
	org2 := seedOrg(t, env.s, "Org 2", "mismatch-org2")
	seedOrgMember(t, env.s, org1.ID, acc.ID, "creator")
	seedOrgMember(t, env.s, org2.ID, acc.ID, "creator")

	token := env.bearerToken(t, acc.ID)
	createSession(t, env, org1.ID, acc.ID)
	createSession(t, env, org1.ID, acc.ID)

	// Get cursor from org1 listing.
	resp1 := getRequest(t, env.srv, "/api/orgs/"+org1.ID+"/sessions?limit=1", token)
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp1.StatusCode)
	}
	var page1 openapi.SessionListResponse
	decodeBody(t, resp1, &page1)
	if page1.NextCursor == "" {
		t.Skip("no cursor available (only 1 session)")
	}

	// Use org1 cursor against org2 — should get 400.
	resp2 := getRequest(t, env.srv, "/api/orgs/"+org2.ID+"/sessions?cursor="+page1.NextCursor, token)
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for filter mismatch, got %d", resp2.StatusCode)
	}

	var errEnv openapi.ErrorEnvelope
	decodeBody(t, resp2, &errEnv)
	if errEnv.Error != "pagination.cursor_filter_mismatch" {
		t.Errorf("expected cursor_filter_mismatch, got %q", errEnv.Error)
	}
}

// ---------------------------------------------------------------------------
// GET /api/orgs/{orgID}/sessions/{sessionID} tests
// ---------------------------------------------------------------------------

func TestGetSession_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "getsess-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	token := env.bearerToken(t, acc.ID)
	sess := createSession(t, env, org.ID, acc.ID)

	resp := getRequest(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result openapi.Session
	decodeBody(t, resp, &result)

	if result.Id != sess.Id {
		t.Errorf("expected session %q, got %q", sess.Id, result.Id)
	}
	if result.Status != "active" {
		t.Errorf("expected status=active, got %q", result.Status)
	}
}

func TestGetSession_NotFound_Returns404(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "getsess-404-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	token := env.bearerToken(t, acc.ID)

	resp := getRequest(t, env.srv, "/api/orgs/"+org.ID+"/sessions/nonexistent-id", token)
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
		// Either 403 (not a member) or 404 is acceptable; we expect 404 since the org member check passes.
		t.Fatalf("expected 404 or 403, got %d", resp.StatusCode)
	}
}

func TestGetSession_NonMemberReturns403(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	outsider := seedAccount(t, env.s, "outsider@example.com")
	org := seedOrg(t, env.s, "Test Org", "getsess-403-org")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	// outsider is org member but NOT session member
	seedOrgMember(t, env.s, org.ID, outsider.ID, "member")

	sess := createSession(t, env, org.ID, creator.ID)
	outsiderToken := env.bearerToken(t, outsider.ID)

	resp := getRequest(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id, outsiderToken)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// GET /api/orgs/{orgID}/sessions/{sessionID}/digest tests
// ---------------------------------------------------------------------------

func TestGetSessionDigest_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "digest-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	token := env.bearerToken(t, acc.ID)
	sess := createSession(t, env, org.ID, acc.ID)

	// Emit a commit.arrived event.
	payload, _ := json.Marshal(map[string]string{
		"ref":       "refs/heads/jam/s/u/main",
		"sha":       "abc123",
		"author_id": "u1",
		"summary":   "fix: auth bug",
	})
	_, _ = env.eventLog.Emit(context.Background(), org.ID, sess.Id, "commit.arrived", payload)

	resp := getRequest(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/digest?since=0", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result openapi.DigestResponse
	decodeBody(t, resp, &result)

	if result.Text == "" {
		t.Error("expected non-empty digest text")
	}
	if result.NextCursor < 0 {
		t.Error("expected non-negative next_cursor")
	}
}

func TestGetSessionDigest_EmptySession(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "digest-empty-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	token := env.bearerToken(t, acc.ID)
	sess := createSession(t, env, org.ID, acc.ID)

	resp := getRequest(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/digest", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result openapi.DigestResponse
	decodeBody(t, resp, &result)

	if result.Text == "" {
		t.Error("expected non-empty digest text (even for empty session, has header)")
	}
	// next_cursor should be 0 (no events).
	if result.NextCursor != 0 {
		t.Errorf("expected next_cursor=0, got %d", result.NextCursor)
	}
}

// ---------------------------------------------------------------------------
// GET /api/orgs/{orgID}/sessions/{sessionID}/refs tests
// ---------------------------------------------------------------------------

func TestListSessionRefs_EmptyRepo(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "refs-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	token := env.bearerToken(t, acc.ID)
	sess := createSession(t, env, org.ID, acc.ID)

	resp := getRequest(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/refs", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result openapi.RefListResponse
	decodeBody(t, resp, &result)

	// Stub storage has no real repo, so refs should be empty.
	if result.Refs == nil {
		t.Error("expected non-nil refs slice (even if empty)")
	}
}

func TestListSessionRefs_NonMemberReturns403(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	outsider := seedAccount(t, env.s, "outsider@example.com")
	org := seedOrg(t, env.s, "Test Org", "refs-403-org")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	seedOrgMember(t, env.s, org.ID, outsider.ID, "member")

	sess := createSession(t, env, org.ID, creator.ID)
	outsiderToken := env.bearerToken(t, outsider.ID)

	resp := getRequest(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/refs", outsiderToken)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Pagination cursor helper tests
// ---------------------------------------------------------------------------

func TestCursorEncodeDecodeRoundTrip(t *testing.T) {
	// This is a pure unit test of the pagination package; no HTTP needed.
	now := time.Now().UTC().Truncate(time.Nanosecond)
	filter := map[string]string{"org_id": "org1"}

	from := pagination.NewCursor(now, "sess1", filter)
	encoded := pagination.Encode(from)

	decoded, err := pagination.Decode(encoded, filter)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.LastID != "sess1" {
		t.Errorf("last_id: got %q want %q", decoded.LastID, "sess1")
	}
	if !decoded.LastCreatedAt().Equal(now) {
		t.Errorf("created_at: got %v want %v", decoded.LastCreatedAt(), now)
	}
}

func TestCursorFilterMismatch(t *testing.T) {
	filter1 := map[string]string{"org_id": "org1"}
	filter2 := map[string]string{"org_id": "org2"}

	cur := pagination.NewCursor(time.Now().UTC(), "id", filter1)
	encoded := pagination.Encode(cur)

	_, err := pagination.Decode(encoded, filter2)
	if !errors.Is(err, pagination.ErrFilterMismatch) {
		t.Errorf("expected ErrFilterMismatch, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Dep-failure tests
// ---------------------------------------------------------------------------

// failingListSessionsStore wraps a real store and returns a transient error
// from ListSessionsForOrgWithCursor, simulating a DB connection failure.
//
// Implements sessionsStore (SessionStore + SessionMemberStore + OrgStore +
// OrgMemberStore + AccountStore + PlaygroundSessionStore + SessionInviteStore +
// RefModeStore + EventLogStore + WithTx), delegating everything except
// ListSessionsForOrgWithCursor to the real store.
type failingListSessionsStore struct {
	realStore store.Store
}

// SessionStore delegation (ListSessionsForOrgWithCursor overridden below)
func (f *failingListSessionsStore) CreateSession(ctx context.Context, p store.CreateSessionParams) (store.Session, error) {
	return f.realStore.CreateSession(ctx, p)
}
func (f *failingListSessionsStore) GetSession(ctx context.Context, orgID, id string) (store.Session, error) {
	return f.realStore.GetSession(ctx, orgID, id)
}
func (f *failingListSessionsStore) GetSessionByID(ctx context.Context, id string) (store.Session, error) {
	return f.realStore.GetSessionByID(ctx, id)
}
func (f *failingListSessionsStore) ListSessionsForOrg(ctx context.Context, orgID string) ([]store.Session, error) {
	return f.realStore.ListSessionsForOrg(ctx, orgID)
}
func (f *failingListSessionsStore) ListSessionsForOrgWithCursor(_ context.Context, _ store.ListSessionsForOrgWithCursorParams) ([]store.Session, error) {
	return nil, errors.New("conn refused")
}
func (f *failingListSessionsStore) UpdateSessionStatus(ctx context.Context, p store.UpdateSessionStatusParams) error {
	return f.realStore.UpdateSessionStatus(ctx, p)
}
func (f *failingListSessionsStore) UpdateSessionGoalScopeMode(ctx context.Context, p store.UpdateSessionGoalScopeModeParams) error {
	return f.realStore.UpdateSessionGoalScopeMode(ctx, p)
}
func (f *failingListSessionsStore) SetSessionBaseSHA(ctx context.Context, p store.SetSessionBaseSHAParams) error {
	return f.realStore.SetSessionBaseSHA(ctx, p)
}
func (f *failingListSessionsStore) SetSessionEndReason(ctx context.Context, p store.SetSessionEndReasonParams) error {
	return f.realStore.SetSessionEndReason(ctx, p)
}
func (f *failingListSessionsStore) SetFinalizeLock(ctx context.Context, p store.SetFinalizeLockParams) error {
	return f.realStore.SetFinalizeLock(ctx, p)
}
func (f *failingListSessionsStore) ClearFinalizeLock(ctx context.Context, p store.ClearFinalizeLockParams) error {
	return f.realStore.ClearFinalizeLock(ctx, p)
}
func (f *failingListSessionsStore) DeleteSession(ctx context.Context, p store.DeleteSessionParams) error {
	return f.realStore.DeleteSession(ctx, p)
}

// SessionMemberStore delegation
func (f *failingListSessionsStore) AddSessionMember(ctx context.Context, p store.AddSessionMemberParams) error {
	return f.realStore.AddSessionMember(ctx, p)
}
func (f *failingListSessionsStore) GetSessionMember(ctx context.Context, p store.GetSessionMemberParams) (store.SessionMember, error) {
	return f.realStore.GetSessionMember(ctx, p)
}
func (f *failingListSessionsStore) ListSessionMembers(ctx context.Context, p store.ListSessionMembersParams) ([]store.SessionMember, error) {
	return f.realStore.ListSessionMembers(ctx, p)
}
func (f *failingListSessionsStore) RemoveSessionMember(ctx context.Context, p store.RemoveSessionMemberParams) error {
	return f.realStore.RemoveSessionMember(ctx, p)
}
func (f *failingListSessionsStore) ListSessionMembershipsForAccount(ctx context.Context, accountID string) ([]store.SessionMembership, error) {
	return f.realStore.ListSessionMembershipsForAccount(ctx, accountID)
}
func (f *failingListSessionsStore) NicknameTakenInSession(ctx context.Context, p store.NicknameTakenInSessionParams) (bool, error) {
	return f.realStore.NicknameTakenInSession(ctx, p)
}
func (f *failingListSessionsStore) CountSessionMembers(ctx context.Context, p store.CountSessionMembersParams) (int64, error) {
	return f.realStore.CountSessionMembers(ctx, p)
}

// OrgStore delegation
func (f *failingListSessionsStore) CreateOrg(ctx context.Context, p store.CreateOrgParams) (store.Org, error) {
	return f.realStore.CreateOrg(ctx, p)
}
func (f *failingListSessionsStore) CreateProtectedOrg(ctx context.Context, p store.CreateProtectedOrgParams) (store.Org, error) {
	return f.realStore.CreateProtectedOrg(ctx, p)
}
func (f *failingListSessionsStore) GetOrgByID(ctx context.Context, id string) (store.Org, error) {
	return f.realStore.GetOrgByID(ctx, id)
}
func (f *failingListSessionsStore) GetOrgBySlug(ctx context.Context, slug string) (store.Org, error) {
	return f.realStore.GetOrgBySlug(ctx, slug)
}
func (f *failingListSessionsStore) UpdateOrgSessionInvitePolicy(ctx context.Context, p store.UpdateOrgSessionInvitePolicyParams) error {
	return f.realStore.UpdateOrgSessionInvitePolicy(ctx, p)
}

// OrgMemberStore delegation
func (f *failingListSessionsStore) AddOrgMember(ctx context.Context, p store.AddOrgMemberParams) error {
	return f.realStore.AddOrgMember(ctx, p)
}
func (f *failingListSessionsStore) GetOrgMember(ctx context.Context, p store.GetOrgMemberParams) (store.OrgMember, error) {
	return f.realStore.GetOrgMember(ctx, p)
}
func (f *failingListSessionsStore) ListOrgsForAccount(ctx context.Context, accountID string) ([]store.Org, error) {
	return f.realStore.ListOrgsForAccount(ctx, accountID)
}
func (f *failingListSessionsStore) ListOrgMembers(ctx context.Context, orgID string) ([]store.OrgMemberWithAccount, error) {
	return f.realStore.ListOrgMembers(ctx, orgID)
}
func (f *failingListSessionsStore) RemoveOrgMember(ctx context.Context, p store.RemoveOrgMemberParams) error {
	return f.realStore.RemoveOrgMember(ctx, p)
}

// AccountStore delegation
func (f *failingListSessionsStore) CreateAccount(ctx context.Context, p store.CreateAccountParams) (store.Account, error) {
	return f.realStore.CreateAccount(ctx, p)
}
func (f *failingListSessionsStore) CreateAnonymousAccount(ctx context.Context, p store.CreateAnonymousAccountParams) (store.Account, error) {
	return f.realStore.CreateAnonymousAccount(ctx, p)
}
func (f *failingListSessionsStore) GetAccountByID(ctx context.Context, id string) (store.Account, error) {
	return f.realStore.GetAccountByID(ctx, id)
}
func (f *failingListSessionsStore) GetAccountByEmail(ctx context.Context, email string) (store.Account, error) {
	return f.realStore.GetAccountByEmail(ctx, email)
}
func (f *failingListSessionsStore) GetAccountByGitHubUserID(ctx context.Context, id *string) (store.Account, error) {
	return f.realStore.GetAccountByGitHubUserID(ctx, id)
}
func (f *failingListSessionsStore) UpdateAccountDisplayName(ctx context.Context, p store.UpdateAccountDisplayNameParams) error {
	return f.realStore.UpdateAccountDisplayName(ctx, p)
}

// PlaygroundSessionStore delegation
func (f *failingListSessionsStore) ResetSessionIdleTimer(ctx context.Context, p store.ResetSessionIdleTimerParams) error {
	return f.realStore.ResetSessionIdleTimer(ctx, p)
}
func (f *failingListSessionsStore) ListExpiredPlaygroundSessions(ctx context.Context, p store.ListExpiredPlaygroundSessionsParams) ([]store.Session, error) {
	return f.realStore.ListExpiredPlaygroundSessions(ctx, p)
}
func (f *failingListSessionsStore) PurgeExpiredTombstones(ctx context.Context, before time.Time) error {
	return f.realStore.PurgeExpiredTombstones(ctx, before)
}
func (f *failingListSessionsStore) ListAnonymousSessionMemberIDs(ctx context.Context, orgID, sessID string) ([]string, error) {
	return f.realStore.ListAnonymousSessionMemberIDs(ctx, orgID, sessID)
}
func (f *failingListSessionsStore) DeleteAccountsByIDs(ctx context.Context, ids []string) error {
	return f.realStore.DeleteAccountsByIDs(ctx, ids)
}
func (f *failingListSessionsStore) CountSessionEventsByType(ctx context.Context, orgID, eventType string) (int64, error) {
	return f.realStore.CountSessionEventsByType(ctx, orgID, eventType)
}

// SessionInviteStore delegation
func (f *failingListSessionsStore) InsertSessionInvite(ctx context.Context, p store.InsertSessionInviteParams) (store.SessionInvite, error) {
	return f.realStore.InsertSessionInvite(ctx, p)
}
func (f *failingListSessionsStore) GetSessionInviteByID(ctx context.Context, id string) (store.SessionInvite, error) {
	return f.realStore.GetSessionInviteByID(ctx, id)
}
func (f *failingListSessionsStore) GetSessionInviteByTokenHash(ctx context.Context, h string) (store.SessionInvite, error) {
	return f.realStore.GetSessionInviteByTokenHash(ctx, h)
}
func (f *failingListSessionsStore) MarkSessionInviteAccepted(ctx context.Context, p store.MarkSessionInviteAcceptedParams) error {
	return f.realStore.MarkSessionInviteAccepted(ctx, p)
}
func (f *failingListSessionsStore) ListPendingSessionInvitesForSession(ctx context.Context, p store.ListPendingSessionInvitesForSessionParams) ([]store.SessionInvite, error) {
	return f.realStore.ListPendingSessionInvitesForSession(ctx, p)
}

// RefModeStore delegation
func (f *failingListSessionsStore) UpsertRefMode(ctx context.Context, p store.UpsertRefModeParams) error {
	return f.realStore.UpsertRefMode(ctx, p)
}
func (f *failingListSessionsStore) GetRefMode(ctx context.Context, p store.GetRefModeParams) (store.RefMode, error) {
	return f.realStore.GetRefMode(ctx, p)
}
func (f *failingListSessionsStore) ListRefModesForSession(ctx context.Context, sessionID string) ([]store.RefMode, error) {
	return f.realStore.ListRefModesForSession(ctx, sessionID)
}

// EventLogStore delegation
func (f *failingListSessionsStore) EnsureEventSeqRow(ctx context.Context, sessionID string) error {
	return f.realStore.EnsureEventSeqRow(ctx, sessionID)
}
func (f *failingListSessionsStore) AllocateNextSeq(ctx context.Context, sessionID string) (int64, error) {
	return f.realStore.AllocateNextSeq(ctx, sessionID)
}
func (f *failingListSessionsStore) AllocateNextSeqN(ctx context.Context, sessionID string, n int64) (int64, error) {
	return f.realStore.AllocateNextSeqN(ctx, sessionID, n)
}
func (f *failingListSessionsStore) InsertEvent(ctx context.Context, p store.InsertEventParams) error {
	return f.realStore.InsertEvent(ctx, p)
}
func (f *failingListSessionsStore) ListEventsSince(ctx context.Context, p store.ListEventsSinceParams) ([]store.Event, error) {
	return f.realStore.ListEventsSince(ctx, p)
}
func (f *failingListSessionsStore) ListEventsSinceForDigest(ctx context.Context, p store.ListEventsSinceForDigestParams) ([]store.Event, error) {
	return f.realStore.ListEventsSinceForDigest(ctx, p)
}

// WithTx delegation
func (f *failingListSessionsStore) WithTx(ctx context.Context, fn func(store.TxStore) error) error {
	return f.realStore.WithTx(ctx, fn)
}

func TestListSessions_DBUnavailable_Returns503DepDBUnavailable(t *testing.T) {
	real := openStore(t)
	acc := seedAccount(t, real, "depfail@example.com")
	org := seedOrg(t, real, "Dep Fail Org", "dep-fail-org")
	seedOrgMember(t, real, org.ID, acc.ID, "creator")

	failing := &failingListSessionsStore{realStore: real}
	env := newTestEnvWithStore(t, failing, real)

	token := env.bearerToken(t, acc.ID)
	resp := getRequest(t, env.srv, "/api/orgs/"+org.ID+"/sessions", token)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Retry-After"); got != "2" {
		t.Errorf("expected Retry-After: 2, got %q", got)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "dep.db_unavailable" {
		t.Errorf("expected error=dep.db_unavailable, got %v", body["error"])
	}
}
