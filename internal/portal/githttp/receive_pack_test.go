package githttp_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/githttp"
	"jamsesh/internal/portal/postreceive"
	"jamsesh/internal/portal/prereceive"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Push test environment
// ---------------------------------------------------------------------------

type pushEnv struct {
	store       store.Store
	tokenSvc    tokens.Service
	storageSvc  storage.Service
	storageRoot string
	eventLog    *events.Log
	server      *httptest.Server
}

func newPushEnv(t *testing.T) *pushEnv {
	t.Helper()
	ctx := context.Background()

	storageRoot := t.TempDir()

	s, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	tokenSvc := tokens.New(s)
	storageSvc := storage.New(storageRoot, s)
	eventLog := events.New(s)

	h := &githttp.Handler{
		Store:    s,
		Tokens:   tokenSvc,
		Storage:  storageSvc,
		Validator: &prereceive.Validator{MaxPackBytes: 50 * 1024 * 1024},
		Emitter:  &postreceive.Emitter{Log: eventLog},
	}

	r := chi.NewRouter()
	h.Mount(r)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &pushEnv{
		store:       s,
		tokenSvc:    tokenSvc,
		storageSvc:  storageSvc,
		storageRoot: storageRoot,
		eventLog:    eventLog,
		server:      srv,
	}
}

func (e *pushEnv) mustIssueToken(t *testing.T, email string) (store.Account, string) {
	t.Helper()
	ctx := context.Background()

	acc, err := e.store.CreateAccount(ctx, store.CreateAccountParams{
		ID:          nextID("pacc"),
		Email:       email,
		DisplayName: email,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAccount(%q): %v", email, err)
	}

	pair, err := e.tokenSvc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue token for %q: %v", email, err)
	}

	return acc, pair.AccessToken
}

func (e *pushEnv) mustCreateSession(t *testing.T, acc store.Account, writableScope string) (orgID, sessionID string) {
	t.Helper()
	ctx := context.Background()

	orgID = nextID("porg")
	org, err := e.store.CreateOrg(ctx, store.CreateOrgParams{
		ID:        orgID,
		Name:      "Push Org",
		Slug:      orgID,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}

	sessionID = nextID("psess")
	_, err = e.store.CreateSession(ctx, store.CreateSessionParams{
		ID:            sessionID,
		OrgID:         org.ID,
		Name:          "Push Session",
		Goal:          "push testing",
		WritableScope: writableScope,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	err = e.store.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID:     org.ID,
		SessionID: sessionID,
		AccountID: acc.ID,
		Role:      "member",
		JoinedAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("AddSessionMember: %v", err)
	}

	return org.ID, sessionID
}

// initBareRepo initialises a bare git repository at bareDir. Returns the
// path to a working (non-bare) clone of it for use as the push source.
func initBareRepo(t *testing.T, bareDir string) (workDir string) {
	t.Helper()

	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}
	runGit(t, "", "init", "--bare", bareDir)

	workDir = t.TempDir()
	runGit(t, workDir, "init", "-b", "main")
	runGit(t, workDir, "config", "user.email", "test@jamsesh.test")
	runGit(t, workDir, "config", "user.name", "Test")
	runGit(t, workDir, "remote", "add", "origin", bareDir)

	return workDir
}

// makeCommitWithTrailers commits a file change to workDir with required jam
// trailers so the push passes pre-receive. sessionID and accountID are used
// to set the correct namespace for the trailers.
func makeCommitWithTrailers(t *testing.T, workDir, sessionID, accountID, filename, content string) {
	t.Helper()

	fp := filepath.Join(workDir, filename)
	if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
	runGit(t, workDir, "add", filename)

	msg := fmt.Sprintf("feat: add %s\n\nJam-Session: %s\nJam-Turn: 1\nJam-Author: %s",
		filename, sessionID, accountID)
	runGit(t, workDir, "commit", "-m", msg)
}

// makeCommitNoTrailers commits a file without required trailers (for testing rejections).
func makeCommitNoTrailers(t *testing.T, workDir, filename, content string) {
	t.Helper()

	fp := filepath.Join(workDir, filename)
	if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
	runGit(t, workDir, "add", filename)
	runGit(t, workDir, "commit", "-m", "feat: no trailers here")
}

// pushURL returns the authenticated git push URL for the given org+session on
// the test server.
func (e *pushEnv) pushURL(token, orgID, sessionID string) string {
	host := strings.TrimPrefix(e.server.URL, "http://")
	return fmt.Sprintf("http://x-access-token:%s@%s/%s/%s.git",
		token, host, orgID, sessionID)
}

// runGit runs a git command and fails the test on non-zero exit.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// runGitExpectFail runs a git command and returns the combined output. Unlike
// runGit it does NOT fail the test on non-zero exit; the caller inspects.
func runGitExpectFail(dir string, args ...string) (output string, ok bool) {
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestReceivePack_WrongContentType verifies that a POST without the correct
// Content-Type is rejected with 400.
func TestReceivePack_WrongContentType(t *testing.T) {
	env := newPushEnv(t)
	acc, token := env.mustIssueToken(t, "push-ct@example.com")
	orgID, sessionID := env.mustCreateSession(t, acc, `["**"]`)

	// Create bare repo on disk (required for the auth middleware chain to proceed
	// past session lookup, but we don't care about the 400 from archive check).
	bareDir := filepath.Join(env.storageRoot, "orgs", orgID, "sessions", sessionID+".git")
	runGit(t, "", "init", "--bare", bareDir)

	url := env.server.URL + "/" + orgID + "/" + sessionID + ".git/git-receive-pack"
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(""))
	req.SetBasicAuth("x-access-token", token)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400 for wrong content-type, got %d", resp.StatusCode)
	}
}

// TestReceivePack_SuccessfulPush verifies an end-to-end push with correct
// trailers succeeds: ref is updated on disk and a commit.arrived event is emitted.
func TestReceivePack_SuccessfulPush(t *testing.T) {
	env := newPushEnv(t)
	acc, token := env.mustIssueToken(t, "push-alice@example.com")
	orgID, sessionID := env.mustCreateSession(t, acc, `["**"]`)

	bareDir := filepath.Join(env.storageRoot, "orgs", orgID, "sessions", sessionID+".git")
	workDir := initBareRepo(t, bareDir)

	// Stage a commit with valid trailers in the user's jam namespace.
	refName := fmt.Sprintf("refs/heads/jam/%s/%s/main", sessionID, acc.ID)
	makeCommitWithTrailers(t, workDir, sessionID, acc.ID, "src/hello.go", "package main", )

	// Push to the user's jam ref namespace via the HTTP server.
	pushURLStr := env.pushURL(token, orgID, sessionID)
	pushCmd := exec.Command("git", "push", pushURLStr,
		fmt.Sprintf("HEAD:%s", refName))
	pushCmd.Dir = workDir
	pushCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := pushCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git push failed: %v\n%s", err, out)
	}

	// Verify the ref exists in the bare repo.
	revParseOutput := runGit(t, bareDir, "rev-parse", refName)
	if revParseOutput == "" {
		t.Error("pushed ref not found in bare repo")
	}

	// Verify commit.arrived event was emitted.
	ctx := context.Background()
	evs, err := env.eventLog.ListSince(ctx, sessionID, 0, 100)
	if err != nil {
		t.Fatalf("ListSince: %v", err)
	}
	if len(evs) == 0 {
		t.Error("expected at least one commit.arrived event after push, got 0")
	}
	if evs[0].Type != "commit.arrived" {
		t.Errorf("event type = %q; want commit.arrived", evs[0].Type)
	}
}

// TestReceivePack_RejectedMissingTrailers verifies that a push missing required
// trailers is rejected with a 200 report-status containing "ng" lines. The git
// client should surface the rejection inline.
func TestReceivePack_RejectedMissingTrailers(t *testing.T) {
	env := newPushEnv(t)
	acc, token := env.mustIssueToken(t, "push-bob@example.com")
	orgID, sessionID := env.mustCreateSession(t, acc, `["**"]`)

	bareDir := filepath.Join(env.storageRoot, "orgs", orgID, "sessions", sessionID+".git")
	workDir := initBareRepo(t, bareDir)

	// Commit without required trailers — pre-receive should reject this.
	makeCommitNoTrailers(t, workDir, "src/bad.go", "bad commit")

	refName := fmt.Sprintf("refs/heads/jam/%s/%s/main", sessionID, acc.ID)
	pushURLStr := env.pushURL(token, orgID, sessionID)

	output, ok := runGitExpectFail(workDir, "push", pushURLStr,
		fmt.Sprintf("HEAD:%s", refName))
	if ok {
		t.Fatal("git push should have failed (rejected by pre-receive), but it succeeded")
	}

	// The git client surfaces rejection lines from the report-status as
	// "remote: error:" or similar. Verify "ng" was in the server response by
	// checking git's output contains rejection evidence.
	if !strings.Contains(output, "ng ") && !strings.Contains(output, "reject") && !strings.Contains(output, "error") {
		t.Errorf("expected rejection in push output, got:\n%s", output)
	}

	// Verify the ref was NOT created in the bare repo.
	cmd := exec.Command("git", "rev-parse", "--verify", refName)
	cmd.Dir = bareDir
	if err := cmd.Run(); err == nil {
		t.Error("rejected ref should not exist in bare repo, but rev-parse succeeded")
	}

	// Verify no events were emitted.
	ctx := context.Background()
	evs, _ := env.eventLog.ListSince(ctx, sessionID, 0, 100)
	if len(evs) != 0 {
		t.Errorf("expected 0 events for rejected push, got %d", len(evs))
	}
}

// TestReceivePack_MultipleCommits verifies that multiple new commits in a single
// push all emit commit.arrived events in chronological order.
func TestReceivePack_MultipleCommits(t *testing.T) {
	env := newPushEnv(t)
	acc, token := env.mustIssueToken(t, "push-charlie@example.com")
	orgID, sessionID := env.mustCreateSession(t, acc, `["**"]`)

	bareDir := filepath.Join(env.storageRoot, "orgs", orgID, "sessions", sessionID+".git")
	workDir := initBareRepo(t, bareDir)

	refName := fmt.Sprintf("refs/heads/jam/%s/%s/main", sessionID, acc.ID)

	// Make 3 commits, each with valid trailers.
	for i := 1; i <= 3; i++ {
		makeCommitWithTrailers(t, workDir, sessionID, acc.ID,
			fmt.Sprintf("src/file%d.go", i),
			fmt.Sprintf("package main // %d", i),
		)
	}

	pushURLStr := env.pushURL(token, orgID, sessionID)
	pushCmd := exec.Command("git", "push", pushURLStr,
		fmt.Sprintf("HEAD:%s", refName))
	pushCmd.Dir = workDir
	pushCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := pushCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git push: %v\n%s", err, out)
	}

	ctx := context.Background()
	evs, err := env.eventLog.ListSince(ctx, sessionID, 0, 100)
	if err != nil {
		t.Fatalf("ListSince: %v", err)
	}

	if len(evs) != 3 {
		t.Fatalf("expected 3 commit.arrived events, got %d", len(evs))
	}

	// Verify seqs are contiguous starting at 1.
	for i, e := range evs {
		if e.Type != "commit.arrived" {
			t.Errorf("evs[%d].Type = %q; want commit.arrived", i, e.Type)
		}
		if e.Seq != int64(i+1) {
			t.Errorf("evs[%d].Seq = %d; want %d", i, e.Seq, i+1)
		}
	}
}

// TestReceivePack_PackSizeLimitExceeded verifies that the 413 response is
// returned when the body exceeds MaxPackBytes. We test this by constructing a
// very small limit on a test handler.
func TestReceivePack_PackSizeLimitExceeded(t *testing.T) {
	ctx := context.Background()
	storageRoot := t.TempDir()

	s, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	tokenSvc := tokens.New(s)
	storageSvc := storage.New(storageRoot, s)

	// Very small limit: 1 byte.
	h := &githttp.Handler{
		Store:    s,
		Tokens:   tokenSvc,
		Storage:  storageSvc,
		Validator: &prereceive.Validator{MaxPackBytes: 1},
		Emitter:  nil,
	}

	r := chi.NewRouter()
	h.Mount(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	acc, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: nextID("slim-acc"), Email: "slim@example.com", DisplayName: "slim",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	pair, err := tokenSvc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	token := pair.AccessToken

	orgID := nextID("slim-org")
	_, err = s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: "Slim Org", Slug: orgID, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	sessionID := nextID("slim-sess")
	_, err = s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessionID, OrgID: orgID, Name: "Slim", Goal: "slim",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	err = s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: orgID, SessionID: sessionID, AccountID: acc.ID,
		Role: "member", JoinedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("AddSessionMember: %v", err)
	}

	// Send a request body larger than the (1 + 16*1024) byte limit.
	largeBody := strings.NewReader(strings.Repeat("x", 20*1024))
	url := srv.URL + "/" + orgID + "/" + sessionID + ".git/git-receive-pack"
	req, _ := http.NewRequest(http.MethodPost, url, largeBody)
	req.SetBasicAuth("x-access-token", token)
	req.Header.Set("Content-Type", "application/x-git-receive-pack-request")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("want 413 for oversized body, got %d", resp.StatusCode)
	}
}
