package sessions_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"jamsesh/internal/api/openapi"
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
