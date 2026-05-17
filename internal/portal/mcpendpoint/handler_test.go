package mcpendpoint_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/oklog/ulid/v2"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/comments"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/mcpendpoint"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// MCP JSON-RPC helpers
// ---------------------------------------------------------------------------

// jsonrpcRequest is a minimal MCP JSON-RPC 2.0 request.
type jsonrpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

// jsonrpcResponse is a minimal MCP JSON-RPC 2.0 response.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// initializeSession performs the MCP initialize+initialized handshake and
// returns the Mcp-Session-Id for subsequent calls.
func initializeSession(t *testing.T, srv *httptest.Server, token string) string {
	t.Helper()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test-client", "version": "0.1.0"},
		},
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initialize: want 200, got %d", resp.StatusCode)
	}

	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("initialize: no Mcp-Session-Id header in response")
	}

	// Send the initialized notification.
	notif := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	}
	nbody, _ := json.Marshal(notif)
	nreq, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", bytes.NewReader(nbody))
	nreq.Header.Set("Authorization", "Bearer "+token)
	nreq.Header.Set("Content-Type", "application/json")
	nreq.Header.Set("Accept", "application/json, text/event-stream")
	nreq.Header.Set("Mcp-Session-Id", sessionID)
	nresp, err := http.DefaultClient.Do(nreq)
	if err != nil {
		t.Fatalf("initialized notification: %v", err)
	}
	defer nresp.Body.Close()

	return sessionID
}

// callTool sends a tools/call JSON-RPC request and returns the raw result.
func callTool(t *testing.T, srv *httptest.Server, token, sessionID, toolName string, args map[string]any) (json.RawMessage, *jsonrpcError) {
	t.Helper()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("callTool %s: %v", toolName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		// Read up to 512 bytes of error body for diagnostics.
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		t.Fatalf("callTool %s: want 200, got %d — body: %s", toolName, resp.StatusCode, buf[:n])
	}

	// Parse SSE or plain JSON response.
	var raw []byte
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	raw = buf[:n]

	// Handle SSE response: lines start with "data: ".
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			raw = []byte(strings.TrimPrefix(line, "data: "))
			break
		}
	}

	var rpc jsonrpcResponse
	if err := json.Unmarshal(raw, &rpc); err != nil {
		t.Fatalf("callTool %s: decode response: %v — raw: %s", toolName, err, raw)
	}

	return rpc.Result, rpc.Error
}

// ---------------------------------------------------------------------------
// Test environment
// ---------------------------------------------------------------------------

type testEnv struct {
	s       store.Store
	log     *events.Log
	svc     *comments.Service
	storage storage.Service
	tokens  tokens.Service
	handler *mcpendpoint.Endpoint
	srv     *httptest.Server
	orgID   string
	sessID  string
	accID   string
	token   string
	repoDir string // temp dir for bare repo
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	s, err := db.Open(context.Background(), "sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	log := events.New(s)
	commentsSvc := &comments.Service{Store: s, Log: log}
	tokenSvc := tokens.New(s)

	// Use a temp dir as the storage root.
	storageRoot := t.TempDir()
	storageSvc := storage.New(storageRoot, s)

	endpoint := &mcpendpoint.Endpoint{
		Store:    s,
		Tokens:   tokenSvc,
		Storage:  storageSvc,
		Log:      log,
		Comments: commentsSvc,
	}

	srv := httptest.NewServer(endpoint.Handler())
	t.Cleanup(srv.Close)

	// Seed: org + account + session + session member.
	ctx := context.Background()
	now := time.Now().UTC()
	orgID := ulid.Make().String()
	accID := ulid.Make().String()
	sessID := ulid.Make().String()

	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: "testorg", Slug: fmt.Sprintf("to-%s", orgID[:8]), CreatedAt: now,
	}); err != nil {
		t.Fatalf("create org: %v", err)
	}
	if _, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: accID, Email: fmt.Sprintf("agent%s@example.com", accID[:8]), DisplayName: "Agent User", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID: orgID, AccountID: accID, Role: "member", CreatedAt: now,
	}); err != nil {
		t.Fatalf("add org member: %v", err)
	}
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessID, OrgID: orgID, Name: "test-sess", Goal: "test goal",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: orgID, SessionID: sessID, AccountID: accID, Role: "creator", JoinedAt: now,
	}); err != nil {
		t.Fatalf("add session member: %v", err)
	}

	pair, err := tokenSvc.Issue(ctx, accID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	return &testEnv{
		s: s, log: log, svc: commentsSvc, storage: storageSvc,
		tokens: tokenSvc, handler: endpoint, srv: srv,
		orgID: orgID, sessID: sessID, accID: accID,
		token: pair.AccessToken,
	}
}

// initBareRepo creates a bare git repo under storageRoot/orgs/<orgID>/sessions/<sessID>.git
// and returns the repo path and a commit SHA that can be used as a fork target.
func (e *testEnv) initBareRepo(t *testing.T) (repoPath string, commitSHA string) {
	t.Helper()

	// First create a regular repo to get a real commit.
	workDir := t.TempDir()
	run(t, workDir, "git", "init", "-b", "main")
	run(t, workDir, "git", "config", "user.email", "test@jamsesh.test")
	run(t, workDir, "git", "config", "user.name", "Test")

	// Write a file and commit.
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run(t, workDir, "git", "add", "-A")
	run(t, workDir, "git", "commit", "-m", "initial commit")

	// Get the commit SHA.
	shaOut := runOut(t, workDir, "git", "rev-parse", "HEAD")
	commitSHA = strings.TrimSpace(shaOut)

	// Create the bare repo directory and clone.
	repoPath = e.storage.RepoPath(e.orgID, e.sessID)
	if err := os.MkdirAll(filepath.Dir(repoPath), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	run(t, workDir, "git", "clone", "--bare", workDir, repoPath)

	return repoPath, commitSHA
}

// run executes a command in dir and fatals on error.
func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run %v: %v\n%s", args, err, out)
	}
}

// runOut executes a command and returns stdout.
func runOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("runOut %v: %v", args, err)
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestMCPEndpoint_Auth_BadToken verifies that a missing or invalid Bearer token
// returns 401 before the MCP protocol is reached.
func TestMCPEndpoint_Auth_BadToken(t *testing.T) {
	env := newTestEnv(t)

	// No Authorization header.
	req, _ := http.NewRequest(http.MethodPost, env.srv.URL+"/mcp", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: want 401, got %d", resp.StatusCode)
	}

	// Invalid Bearer token.
	req2, _ := http.NewRequest(http.MethodPost, env.srv.URL+"/mcp", bytes.NewReader([]byte("{}")))
	req2.Header.Set("Authorization", "Bearer invalid-token-abc")
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Accept", "application/json, text/event-stream")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("request invalid token: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("invalid token: want 401, got %d", resp2.StatusCode)
	}
}

// TestMCPEndpoint_PostComment_HappyPath verifies that post_comment creates a
// comment and returns the comment ID.
func TestMCPEndpoint_PostComment_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	mcpSess := initializeSession(t, env.srv, env.token)

	sha := "abc1234567890abcdef1234567890abcdef12345"
	result, rpcErr := callTool(t, env.srv, env.token, mcpSess, "post_comment", map[string]any{
		"session_id": env.sessID,
		"commit_sha": sha,
		"body":       "Test comment from agent",
		"kind":       "fyi",
	})
	if rpcErr != nil {
		t.Fatalf("post_comment rpc error: %v", rpcErr)
	}

	// The result should contain content with comment_id.
	var toolResult struct {
		Content []struct {
			Type string          `json:"type"`
			Text string          `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &toolResult); err != nil {
		t.Fatalf("unmarshal result: %v — raw: %s", err, result)
	}

	if toolResult.IsError {
		t.Fatalf("post_comment tool returned error: %s", result)
	}

	// Extract the text content and decode the output struct.
	if len(toolResult.Content) == 0 {
		t.Fatalf("post_comment: empty content in result")
	}
	var out mcpendpoint.PostCommentOutput
	if err := json.Unmarshal([]byte(toolResult.Content[0].Text), &out); err != nil {
		t.Fatalf("decode PostCommentOutput: %v — text: %s", err, toolResult.Content[0].Text)
	}
	if out.CommentID == "" {
		t.Error("post_comment: expected non-empty comment_id")
	}

	// Verify the comment exists in the DB.
	ctx := context.Background()
	comms, _, err := env.svc.List(ctx, comments.ListParams{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comms) != 1 {
		t.Fatalf("want 1 comment, got %d", len(comms))
	}
	if comms[0].Body != "Test comment from agent" {
		t.Errorf("comment body mismatch: got %q", comms[0].Body)
	}
}

// TestMCPEndpoint_PostComment_NonMember verifies that calling post_comment for
// a session the caller is not a member of returns an error.
func TestMCPEndpoint_PostComment_NonMember(t *testing.T) {
	env := newTestEnv(t)

	// Create a second account that is NOT a session member.
	ctx := context.Background()
	now := time.Now().UTC()
	acc2ID := ulid.Make().String()
	if _, err := env.s.CreateAccount(ctx, store.CreateAccountParams{
		ID: acc2ID, Email: fmt.Sprintf("other%s@example.com", acc2ID[:8]),
		DisplayName: "Other User", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create account 2: %v", err)
	}
	pair2, err := env.tokens.Issue(ctx, acc2ID)
	if err != nil {
		t.Fatalf("issue token 2: %v", err)
	}

	mcpSess := initializeSession(t, env.srv, pair2.AccessToken)

	result, _ := callTool(t, env.srv, pair2.AccessToken, mcpSess, "post_comment", map[string]any{
		"session_id": env.sessID,
		"commit_sha": "abc1234567890abcdef1234567890abcdef12345",
		"body":       "Should fail",
	})

	// Result must contain IsError: true.
	var toolResult struct {
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &toolResult); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !toolResult.IsError {
		t.Error("post_comment non-member: expected IsError=true")
	}
}

// TestMCPEndpoint_ResolveComment_HappyPath verifies that resolve_comment marks
// a comment as resolved.
func TestMCPEndpoint_ResolveComment_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Create a comment via the service directly.
	c, err := env.svc.Create(ctx, comments.CreateParams{
		OrgID:           env.orgID,
		SessionID:       env.sessID,
		AuthorAccountID: env.accID,
		AuthorKind:      "agent",
		AnchorCommitSHA: "abc1234567890abcdef1234567890abcdef12345",
		Body:            "Resolve me",
		Kind:            "action-request",
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	mcpSess := initializeSession(t, env.srv, env.token)
	result, rpcErr := callTool(t, env.srv, env.token, mcpSess, "resolve_comment", map[string]any{
		"session_id": env.sessID,
		"comment_id": c.ID,
	})
	if rpcErr != nil {
		t.Fatalf("resolve_comment rpc error: %v", rpcErr)
	}

	var toolResult struct {
		Content []struct{ Text string `json:"text"` } `json:"content"`
		IsError bool                                   `json:"isError"`
	}
	if err := json.Unmarshal(result, &toolResult); err != nil {
		t.Fatalf("decode result: %v — raw: %s", err, result)
	}
	if toolResult.IsError {
		t.Fatalf("resolve_comment returned error: %s", result)
	}

	var out mcpendpoint.ResolveCommentOutput
	if len(toolResult.Content) > 0 {
		if err := json.Unmarshal([]byte(toolResult.Content[0].Text), &out); err != nil {
			t.Fatalf("decode output: %v", err)
		}
	}
	if !out.Resolved {
		t.Error("resolve_comment: expected Resolved=true")
	}
}

// TestMCPEndpoint_Fork_HappyPath verifies that fork creates a ref in the bare
// repo and returns the ref name.
func TestMCPEndpoint_Fork_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	repoPath, commitSHA := env.initBareRepo(t)

	mcpSess := initializeSession(t, env.srv, env.token)
	result, rpcErr := callTool(t, env.srv, env.token, mcpSess, "fork", map[string]any{
		"session_id":       env.sessID,
		"target_commit_sha": commitSHA,
		"target_ref":       "my-branch",
	})
	if rpcErr != nil {
		t.Fatalf("fork rpc error: %v", rpcErr)
	}

	var toolResult struct {
		Content []struct{ Text string `json:"text"` } `json:"content"`
		IsError bool                                   `json:"isError"`
	}
	if err := json.Unmarshal(result, &toolResult); err != nil {
		t.Fatalf("decode result: %v — raw: %s", err, result)
	}
	if toolResult.IsError {
		t.Fatalf("fork returned error: %s", result)
	}

	var out mcpendpoint.ForkOutput
	if len(toolResult.Content) > 0 {
		if err := json.Unmarshal([]byte(toolResult.Content[0].Text), &out); err != nil {
			t.Fatalf("decode output: %v", err)
		}
	}
	if out.Ref == "" {
		t.Fatal("fork: expected non-empty ref")
	}
	if out.SHA != commitSHA {
		t.Errorf("fork: sha mismatch: want %s, got %s", commitSHA, out.SHA)
	}

	// Verify the ref exists in the repo.
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	ref, err := repo.Reference(plumbing.ReferenceName(out.Ref), true)
	if err != nil {
		t.Fatalf("ref not found after fork: %v", err)
	}
	if ref.Hash().String() != commitSHA {
		t.Errorf("ref sha: want %s, got %s", commitSHA, ref.Hash().String())
	}
}

// TestMCPEndpoint_Fork_BadCommit verifies that forking from a non-existent
// commit returns a tool error.
func TestMCPEndpoint_Fork_BadCommit(t *testing.T) {
	env := newTestEnv(t)
	env.initBareRepo(t) // ensure repo exists

	mcpSess := initializeSession(t, env.srv, env.token)
	result, _ := callTool(t, env.srv, env.token, mcpSess, "fork", map[string]any{
		"session_id":        env.sessID,
		"target_commit_sha": "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	})

	var toolResult struct{ IsError bool `json:"isError"` }
	if err := json.Unmarshal(result, &toolResult); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !toolResult.IsError {
		t.Error("fork bad commit: expected IsError=true")
	}
}

// TestMCPEndpoint_QuerySessionState_HappyPath verifies query_session_state
// returns the expected session fields.
func TestMCPEndpoint_QuerySessionState_HappyPath(t *testing.T) {
	env := newTestEnv(t)

	mcpSess := initializeSession(t, env.srv, env.token)
	result, rpcErr := callTool(t, env.srv, env.token, mcpSess, "query_session_state", map[string]any{
		"session_id": env.sessID,
		"include":    []string{"goal", "scope"},
	})
	if rpcErr != nil {
		t.Fatalf("query_session_state rpc error: %v", rpcErr)
	}

	var toolResult struct {
		Content []struct{ Text string `json:"text"` } `json:"content"`
		IsError bool                                   `json:"isError"`
	}
	if err := json.Unmarshal(result, &toolResult); err != nil {
		t.Fatalf("decode result: %v — raw: %s", err, result)
	}
	if toolResult.IsError {
		t.Fatalf("query_session_state returned error: %s", result)
	}

	var out mcpendpoint.QuerySessionStateOutput
	if len(toolResult.Content) > 0 {
		if err := json.Unmarshal([]byte(toolResult.Content[0].Text), &out); err != nil {
			t.Fatalf("decode output: %v", err)
		}
	}
	if out.Goal != "test goal" {
		t.Errorf("goal: want %q, got %q", "test goal", out.Goal)
	}
	if !strings.Contains(out.Scope, "**") {
		t.Errorf("scope missing '**': got %q", out.Scope)
	}
}

// TestMCPEndpoint_QuerySessionState_NonMember verifies that querying state for
// a session the caller is not a member of returns a tool error.
func TestMCPEndpoint_QuerySessionState_NonMember(t *testing.T) {
	env := newTestEnv(t)

	// Create a second account that is NOT a session member.
	ctx := context.Background()
	now := time.Now().UTC()
	acc2ID := ulid.Make().String()
	if _, err := env.s.CreateAccount(ctx, store.CreateAccountParams{
		ID: acc2ID, Email: fmt.Sprintf("other2%s@example.com", acc2ID[:8]),
		DisplayName: "Other", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	pair2, err := env.tokens.Issue(ctx, acc2ID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	mcpSess := initializeSession(t, env.srv, pair2.AccessToken)
	result, _ := callTool(t, env.srv, pair2.AccessToken, mcpSess, "query_session_state", map[string]any{
		"session_id": env.sessID,
	})

	var toolResult struct{ IsError bool `json:"isError"` }
	if err := json.Unmarshal(result, &toolResult); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !toolResult.IsError {
		t.Error("query_session_state non-member: expected IsError=true")
	}
}

// TestMCPEndpoint_QuerySessionState_RecentEvents verifies that recent_events
// are populated when the session has events.
func TestMCPEndpoint_QuerySessionState_RecentEvents(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Emit an event.
	payload, _ := json.Marshal(map[string]string{"sha": "abc123", "ref": "refs/heads/main"})
	if _, err := env.log.Emit(ctx, env.orgID, env.sessID, "commit.arrived", payload); err != nil {
		t.Fatalf("emit event: %v", err)
	}

	mcpSess := initializeSession(t, env.srv, env.token)
	result, rpcErr := callTool(t, env.srv, env.token, mcpSess, "query_session_state", map[string]any{
		"session_id": env.sessID,
		"include":    []string{"recent_events"},
	})
	if rpcErr != nil {
		t.Fatalf("query_session_state rpc error: %v", rpcErr)
	}

	var toolResult struct {
		Content []struct{ Text string `json:"text"` } `json:"content"`
		IsError bool                                   `json:"isError"`
	}
	if err := json.Unmarshal(result, &toolResult); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if toolResult.IsError {
		t.Fatalf("query_session_state returned error: %s", result)
	}

	var out mcpendpoint.QuerySessionStateOutput
	if len(toolResult.Content) > 0 {
		_ = json.Unmarshal([]byte(toolResult.Content[0].Text), &out)
	}
	if len(out.RecentEvents) == 0 {
		t.Error("expected at least one recent event")
	} else if out.RecentEvents[0].Type != "commit.arrived" {
		t.Errorf("event type: want commit.arrived, got %s", out.RecentEvents[0].Type)
	}
}

// Ensure object package is imported (used via initBareRepo indirectly).
var _ = object.Blob{}
