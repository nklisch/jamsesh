package comments_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/comments"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Test environment
// ---------------------------------------------------------------------------

type testEnv struct {
	s       store.Store
	svc     *comments.Service
	handler *comments.Handler
	srv     *httptest.Server
	token   string
	orgID   string
	sessID  string
	accID   string
}

// commentsOnlyStrict wires only the comments handler and panics on everything else.
type commentsOnlyStrict struct {
	*comments.Handler
}

func (c *commentsOnlyStrict) ExchangeMagicLink(_ context.Context, _ openapi.ExchangeMagicLinkRequestObject) (openapi.ExchangeMagicLinkResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) RequestMagicLink(_ context.Context, _ openapi.RequestMagicLinkRequestObject) (openapi.RequestMagicLinkResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) OauthCallback(_ context.Context, _ openapi.OauthCallbackRequestObject) (openapi.OauthCallbackResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) StartOAuth(_ context.Context, _ openapi.StartOAuthRequestObject) (openapi.StartOAuthResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) RefreshToken(_ context.Context, _ openapi.RefreshTokenRequestObject) (openapi.RefreshTokenResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) RevokeToken(_ context.Context, _ openapi.RevokeTokenRequestObject) (openapi.RevokeTokenResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) GetMe(_ context.Context, _ openapi.GetMeRequestObject) (openapi.GetMeResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) CreateOrg(_ context.Context, _ openapi.CreateOrgRequestObject) (openapi.CreateOrgResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) ListOrgMembers(_ context.Context, _ openapi.ListOrgMembersRequestObject) (openapi.ListOrgMembersResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) CreateOrgInvite(_ context.Context, _ openapi.CreateOrgInviteRequestObject) (openapi.CreateOrgInviteResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) AcceptOrgInvite(_ context.Context, _ openapi.AcceptOrgInviteRequestObject) (openapi.AcceptOrgInviteResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) CreateSession(_ context.Context, _ openapi.CreateSessionRequestObject) (openapi.CreateSessionResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) PatchSession(_ context.Context, _ openapi.PatchSessionRequestObject) (openapi.PatchSessionResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) FinalizeSession(_ context.Context, _ openapi.FinalizeSessionRequestObject) (openapi.FinalizeSessionResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) AbandonSession(_ context.Context, _ openapi.AbandonSessionRequestObject) (openapi.AbandonSessionResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) ListSessions(_ context.Context, _ openapi.ListSessionsRequestObject) (openapi.ListSessionsResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) GetSession(_ context.Context, _ openapi.GetSessionRequestObject) (openapi.GetSessionResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) ListSessionRefs(_ context.Context, _ openapi.ListSessionRefsRequestObject) (openapi.ListSessionRefsResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) GetSessionDigest(_ context.Context, _ openapi.GetSessionDigestRequestObject) (openapi.GetSessionDigestResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) InviteToSession(_ context.Context, _ openapi.InviteToSessionRequestObject) (openapi.InviteToSessionResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) AcceptSessionInvite(_ context.Context, _ openapi.AcceptSessionInviteRequestObject) (openapi.AcceptSessionInviteResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) RemoveSessionMember(_ context.Context, _ openapi.RemoveSessionMemberRequestObject) (openapi.RemoveSessionMemberResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) GetSessionFile(_ context.Context, _ openapi.GetSessionFileRequestObject) (openapi.GetSessionFileResponseObject, error) {
	panic("not wired")
}
func (c *commentsOnlyStrict) UpsertRefMode(_ context.Context, _ openapi.UpsertRefModeRequestObject) (openapi.UpsertRefModeResponseObject, error) {
	panic("not wired")
}

var _ openapi.StrictServerInterface = (*commentsOnlyStrict)(nil)

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	s, err := db.Open(context.Background(), "sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	log := events.New(s)
	svc := &comments.Service{Store: s, Log: log}
	handler := comments.NewHandler(svc)

	tokenSvc := tokens.New(s)

	// Seed: org + account + session member.
	ctx := context.Background()
	now := time.Now().UTC()
	orgID := ulid.Make().String()
	accID := ulid.Make().String()
	sessID := ulid.Make().String()

	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: "testorg", Slug: fmt.Sprintf("testorg-%s", orgID[:8]), CreatedAt: now,
	}); err != nil {
		t.Fatalf("create org: %v", err)
	}
	if _, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: accID, Email: fmt.Sprintf("user%s@example.com", accID[:8]), DisplayName: "Test User", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID: orgID, AccountID: accID, Role: "member", CreatedAt: now,
	}); err != nil {
		t.Fatalf("add org member: %v", err)
	}
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessID, OrgID: orgID, Name: "test-session", Goal: "test goal",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: orgID, SessionID: sessID, AccountID: accID, Role: "creator", JoinedAt: now,
	}); err != nil {
		t.Fatalf("add session member: %v", err)
	}

	// Issue a token for the account.
	pair, err := tokenSvc.Issue(ctx, accID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	rawToken := pair.AccessToken

	// Build HTTP test server.
	strictAPI := openapi.NewStrictHandler(&commentsOnlyStrict{Handler: handler}, nil)
	apiWrapper := &openapi.ServerInterfaceWrapper{
		Handler: strictAPI,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		},
	}

	r := chi.NewRouter()
	r.Use(tokens.BearerMiddleware(tokenSvc))
	r.Get("/api/orgs/{orgID}/sessions/{sessionID}/comments", apiWrapper.ListComments)
	r.Post("/api/orgs/{orgID}/sessions/{sessionID}/comments", apiWrapper.CreateComment)
	r.Post("/api/orgs/{orgID}/sessions/{sessionID}/comments/{commentId}/resolve", apiWrapper.ResolveComment)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &testEnv{
		s:       s,
		svc:     svc,
		handler: handler,
		srv:     srv,
		token:   rawToken,
		orgID:   orgID,
		sessID:  sessID,
		accID:   accID,
	}
}

func (e *testEnv) createComment(t *testing.T, params comments.CreateParams) store.Comment {
	t.Helper()
	if params.OrgID == "" {
		params.OrgID = e.orgID
	}
	if params.SessionID == "" {
		params.SessionID = e.sessID
	}
	if params.AuthorAccountID == "" {
		params.AuthorAccountID = e.accID
	}
	if params.AuthorKind == "" {
		params.AuthorKind = "human"
	}
	if params.AnchorCommitSHA == "" {
		params.AnchorCommitSHA = "abc123"
	}
	if params.Body == "" {
		params.Body = "test body"
	}
	if params.Kind == "" {
		params.Kind = "fyi"
	}
	c, err := e.svc.Create(context.Background(), params)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	return c
}

func (e *testEnv) get(t *testing.T, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+e.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func (e *testEnv) post(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+path, &buf)
	req.Header.Set("Authorization", "Bearer "+e.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func decode(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestServiceCreate(t *testing.T) {
	env := newTestEnv(t)

	fp := "main.go"
	ls := int32(10)
	le := int32(15)
	addr := "@all-agents"

	c, err := env.svc.Create(context.Background(), comments.CreateParams{
		OrgID:           env.orgID,
		SessionID:       env.sessID,
		AuthorAccountID: env.accID,
		AuthorKind:      "human",
		AnchorCommitSHA: "deadbeef",
		AnchorFilePath:  &fp,
		AnchorLineStart: &ls,
		AnchorLineEnd:   &le,
		Body:            "Please check this",
		AddressedTo:     &addr,
		Kind:            "question",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.ID == "" {
		t.Error("expected non-empty ID")
	}
	if c.AnchorFilePath == nil || *c.AnchorFilePath != fp {
		t.Errorf("expected anchor file path %q, got %v", fp, c.AnchorFilePath)
	}
	if c.AnchorLineStart == nil || *c.AnchorLineStart != ls {
		t.Errorf("expected anchor line start %d, got %v", ls, c.AnchorLineStart)
	}
	if c.AnchorLineEnd == nil || *c.AnchorLineEnd != le {
		t.Errorf("expected anchor line end %d, got %v", le, c.AnchorLineEnd)
	}
	if c.AddressedTo == nil || *c.AddressedTo != addr {
		t.Errorf("expected addressed_to %q, got %v", addr, c.AddressedTo)
	}
	if c.ResolvedAt != nil {
		t.Error("expected unresolved comment")
	}

	// Verify the comment.added event was written to the DB.
	evts, err := env.s.ListEventsSince(context.Background(), store.ListEventsSinceParams{
		SessionID: env.sessID, SinceSeq: -1, Limit: 10,
	})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	found := false
	for _, e := range evts {
		if e.Type == "comment.added" {
			found = true
		}
	}
	if !found {
		t.Error("expected comment.added event in event log")
	}
}

func TestServiceResolve(t *testing.T) {
	env := newTestEnv(t)

	// Create a comment.
	c := env.createComment(t, comments.CreateParams{Kind: "action-request"})

	// Resolve it.
	note := "done"
	resolved, err := env.svc.Resolve(context.Background(), comments.ResolveParams{
		OrgID:          env.orgID,
		SessionID:      env.sessID,
		CommentID:      c.ID,
		AccountID:      env.accID,
		ResolutionNote: &note,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.ResolvedAt == nil {
		t.Error("expected resolved_at to be set")
	}
	if resolved.ResolvedByAccountID == nil || *resolved.ResolvedByAccountID != env.accID {
		t.Errorf("expected resolved_by=%s, got %v", env.accID, resolved.ResolvedByAccountID)
	}
	if resolved.ResolutionNote == nil || *resolved.ResolutionNote != note {
		t.Errorf("expected resolution_note=%q, got %v", note, resolved.ResolutionNote)
	}

	// Double-resolve → ErrAlreadyResolved.
	_, err = env.svc.Resolve(context.Background(), comments.ResolveParams{
		OrgID: env.orgID, SessionID: env.sessID, CommentID: c.ID, AccountID: env.accID,
	})
	if err == nil {
		t.Fatal("expected error on double-resolve, got nil")
	}
	if !isAlreadyResolved(err) {
		t.Errorf("expected ErrAlreadyResolved, got: %v", err)
	}
}

func isAlreadyResolved(err error) bool {
	return err != nil && err.Error() == comments.ErrAlreadyResolved.Error()
}

func TestServiceListAndFilters(t *testing.T) {
	env := newTestEnv(t)

	sha1 := "sha000001"
	sha2 := "sha000002"
	fp := "file.go"
	addr := "@agent/main"

	// Create several comments.
	env.createComment(t, comments.CreateParams{Kind: "fyi", AnchorCommitSHA: sha1})
	env.createComment(t, comments.CreateParams{Kind: "question", AnchorCommitSHA: sha1, AddressedTo: &addr})
	env.createComment(t, comments.CreateParams{Kind: "suggestion", AnchorCommitSHA: sha2, AnchorFilePath: &fp})

	ctx := context.Background()

	// List all — should return 3.
	all, _, err := env.svc.List(ctx, comments.ListParams{
		OrgID: env.orgID, SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 comments, got %d", len(all))
	}

	// Filter by kind = fyi.
	byKind, _, err := env.svc.List(ctx, comments.ListParams{
		OrgID: env.orgID, SessionID: env.sessID, Kind: "fyi",
	})
	if err != nil {
		t.Fatalf("List by kind: %v", err)
	}
	if len(byKind) != 1 {
		t.Errorf("expected 1 fyi comment, got %d", len(byKind))
	}

	// Filter by addressed_to substring.
	byAddr, _, err := env.svc.List(ctx, comments.ListParams{
		OrgID: env.orgID, SessionID: env.sessID, AddressedTo: "agent",
	})
	if err != nil {
		t.Fatalf("List by addressed_to: %v", err)
	}
	if len(byAddr) != 1 {
		t.Errorf("expected 1 comment addressed to agent, got %d", len(byAddr))
	}

	// Filter by anchor_commit_sha.
	bySHA, _, err := env.svc.List(ctx, comments.ListParams{
		OrgID: env.orgID, SessionID: env.sessID, AnchorCommitSHA: sha2,
	})
	if err != nil {
		t.Fatalf("List by anchor_commit_sha: %v", err)
	}
	if len(bySHA) != 1 {
		t.Errorf("expected 1 comment for sha2, got %d", len(bySHA))
	}

	// Filter by anchor_file_path.
	byFP, _, err := env.svc.List(ctx, comments.ListParams{
		OrgID: env.orgID, SessionID: env.sessID, AnchorFilePath: fp,
	})
	if err != nil {
		t.Fatalf("List by anchor_file_path: %v", err)
	}
	if len(byFP) != 1 {
		t.Errorf("expected 1 comment for file path, got %d", len(byFP))
	}

	// Filter unresolved only.
	f := false
	unresolvedAll, _, err := env.svc.List(ctx, comments.ListParams{
		OrgID: env.orgID, SessionID: env.sessID, Resolved: &f,
	})
	if err != nil {
		t.Fatalf("List unresolved: %v", err)
	}
	if len(unresolvedAll) != 3 {
		t.Errorf("expected 3 unresolved, got %d", len(unresolvedAll))
	}

	// Resolve one and check resolved filter.
	_, err = env.svc.Resolve(ctx, comments.ResolveParams{
		OrgID: env.orgID, SessionID: env.sessID, CommentID: all[0].ID, AccountID: env.accID,
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	tr := true
	resolvedOnly, _, err := env.svc.List(ctx, comments.ListParams{
		OrgID: env.orgID, SessionID: env.sessID, Resolved: &tr,
	})
	if err != nil {
		t.Fatalf("List resolved only: %v", err)
	}
	if len(resolvedOnly) != 1 {
		t.Errorf("expected 1 resolved comment, got %d", len(resolvedOnly))
	}

	unresolvedOnly, _, err := env.svc.List(ctx, comments.ListParams{
		OrgID: env.orgID, SessionID: env.sessID, Resolved: &f,
	})
	if err != nil {
		t.Fatalf("List unresolved only: %v", err)
	}
	if len(unresolvedOnly) != 2 {
		t.Errorf("expected 2 unresolved comments, got %d", len(unresolvedOnly))
	}
}

func TestServiceListCursorPagination(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Create 5 comments with staggered timestamps (use distinct created_at via
	// sleeping briefly or by adjusting via the store directly — here we use a
	// small sleep to guarantee ordering).
	for i := 0; i < 5; i++ {
		env.createComment(t, comments.CreateParams{
			Body: fmt.Sprintf("comment %d", i),
		})
		// Tiny pause so created_at is distinct and ordered.
		time.Sleep(2 * time.Millisecond)
	}

	// Page 1: limit=2.
	page1, cursor1, err := env.svc.List(ctx, comments.ListParams{
		OrgID: env.orgID, SessionID: env.sessID, Limit: 2,
	})
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 comments on page1, got %d", len(page1))
	}
	if cursor1 == "" {
		t.Fatal("expected non-empty cursor after page1")
	}

	// Page 2: limit=2 with cursor.
	page2, cursor2, err := env.svc.List(ctx, comments.ListParams{
		OrgID: env.orgID, SessionID: env.sessID, Limit: 2, Cursor: cursor1,
	})
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("expected 2 comments on page2, got %d", len(page2))
	}
	if cursor2 == "" {
		t.Fatal("expected non-empty cursor after page2")
	}

	// Page 3: limit=2 with cursor — should return 1 item (5 total, 2+2+1).
	page3, cursor3, err := env.svc.List(ctx, comments.ListParams{
		OrgID: env.orgID, SessionID: env.sessID, Limit: 2, Cursor: cursor2,
	})
	if err != nil {
		t.Fatalf("List page3: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("expected 1 comment on page3, got %d", len(page3))
	}
	if cursor3 != "" {
		t.Error("expected empty cursor on last page")
	}

	// All IDs should be distinct.
	seen := map[string]bool{}
	for _, pg := range [][]store.Comment{page1, page2, page3} {
		for _, c := range pg {
			if seen[c.ID] {
				t.Errorf("duplicate comment ID across pages: %s", c.ID)
			}
			seen[c.ID] = true
		}
	}
}

func TestHandlerListComments(t *testing.T) {
	env := newTestEnv(t)

	// Create 2 comments.
	env.createComment(t, comments.CreateParams{Kind: "fyi"})
	env.createComment(t, comments.CreateParams{Kind: "question"})

	path := fmt.Sprintf("/api/orgs/%s/sessions/%s/comments", env.orgID, env.sessID)
	resp := env.get(t, path)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body openapi.CommentListResponse
	decode(t, resp, &body)
	if len(body.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(body.Items))
	}
}

func TestHandlerListCommentsFilter(t *testing.T) {
	env := newTestEnv(t)

	addr := "@user/main"
	env.createComment(t, comments.CreateParams{Kind: "fyi", AddressedTo: &addr})
	env.createComment(t, comments.CreateParams{Kind: "question"})

	path := fmt.Sprintf("/api/orgs/%s/sessions/%s/comments?kind=fyi", env.orgID, env.sessID)
	resp := env.get(t, path)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body openapi.CommentListResponse
	decode(t, resp, &body)
	if len(body.Items) != 1 {
		t.Errorf("expected 1 fyi comment, got %d", len(body.Items))
	}
}

func TestHandlerResolveComment(t *testing.T) {
	env := newTestEnv(t)

	c := env.createComment(t, comments.CreateParams{Kind: "action-request"})

	path := fmt.Sprintf("/api/orgs/%s/sessions/%s/comments/%s/resolve", env.orgID, env.sessID, c.ID)
	resp := env.post(t, path, map[string]string{"resolution_note": "done"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var resolved openapi.Comment
	decode(t, resp, &resolved)
	if resolved.ResolvedAt.IsZero() {
		t.Error("expected resolved_at to be set")
	}
	if resolved.ResolvedBy == "" {
		t.Error("expected resolved_by to be set")
	}
	if resolved.ResolutionNote != "done" {
		t.Errorf("expected resolution_note=done, got %q", resolved.ResolutionNote)
	}
}

func TestHandlerResolveCommentAlreadyResolved(t *testing.T) {
	env := newTestEnv(t)

	c := env.createComment(t, comments.CreateParams{Kind: "action-request"})

	path := fmt.Sprintf("/api/orgs/%s/sessions/%s/comments/%s/resolve", env.orgID, env.sessID, c.ID)
	resp1 := env.post(t, path, nil)
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first resolve: expected 200, got %d", resp1.StatusCode)
	}

	// Second resolve → 409.
	resp2 := env.post(t, path, nil)
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("double-resolve: expected 409, got %d", resp2.StatusCode)
	}
}

func TestHandlerResolveCommentNotFound(t *testing.T) {
	env := newTestEnv(t)

	path := fmt.Sprintf("/api/orgs/%s/sessions/%s/comments/%s/resolve",
		env.orgID, env.sessID, "nonexistent-id")
	resp := env.post(t, path, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// CreateComment handler tests
// ---------------------------------------------------------------------------

func TestHandlerCreateComment_Success(t *testing.T) {
	env := newTestEnv(t)

	path := fmt.Sprintf("/api/orgs/%s/sessions/%s/comments", env.orgID, env.sessID)
	body := map[string]any{
		"anchor_commit_sha": "abc1234",
		"anchor_file_path":  "pkg/auth.go",
		"anchor_line_start": 10,
		"anchor_line_end":   10,
		"body":              "This looks suspect.",
		"kind":              "question",
		"addressed_to":      "@reviewer",
	}
	resp := env.post(t, path, body)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}

	var c map[string]any
	decode(t, resp, &c)
	if c["id"] == nil {
		t.Error("want id in response, got nil")
	}
	if c["body"] != "This looks suspect." {
		t.Errorf("want body=%q, got %v", "This looks suspect.", c["body"])
	}
	if c["kind"] != "question" {
		t.Errorf("want kind=question, got %v", c["kind"])
	}
	if c["author_kind"] != "human" {
		t.Errorf("want author_kind=human, got %v", c["author_kind"])
	}
}

func TestHandlerCreateComment_MissingBody(t *testing.T) {
	env := newTestEnv(t)
	path := fmt.Sprintf("/api/orgs/%s/sessions/%s/comments", env.orgID, env.sessID)
	resp := env.post(t, path, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestHandlerCreateComment_WrongSession(t *testing.T) {
	env := newTestEnv(t)
	path := fmt.Sprintf("/api/orgs/%s/sessions/%s/comments", env.orgID, "nonexistent-session")
	body := map[string]any{
		"anchor_commit_sha": "abc",
		"body":              "hi",
		"kind":              "fyi",
	}
	resp := env.post(t, path, body)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 403 or 404, got %d", resp.StatusCode)
	}
}

func TestHandlerCreateComment_CommitLevelAnchor(t *testing.T) {
	env := newTestEnv(t)
	path := fmt.Sprintf("/api/orgs/%s/sessions/%s/comments", env.orgID, env.sessID)
	body := map[string]any{
		"anchor_commit_sha": "deadbeef",
		"body":              "Commit-level comment.",
		"kind":              "fyi",
	}
	resp := env.post(t, path, body)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}

	var c map[string]any
	decode(t, resp, &c)
	anchor := c["anchor"].(map[string]any)
	if anchor["file_path"] != nil && anchor["file_path"] != "" {
		t.Errorf("want no file_path for commit-level comment, got %v", anchor["file_path"])
	}
}
