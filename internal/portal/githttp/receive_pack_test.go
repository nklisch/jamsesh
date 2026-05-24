package githttp_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	"jamsesh/internal/portal/storage/objectstore"
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

	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
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

	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
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

// TestReceivePack_ObjectStorageFailure verifies the RPO=0 contract:
// when the object-storage sync fails (e.g. NoSuchBucket), the receive-pack
// endpoint returns a non-2xx status so the git client exits non-zero.
// The push must NOT be silently acknowledged.
func TestReceivePack_ObjectStorageFailure(t *testing.T) {
	ctx := context.Background()
	storageRoot := t.TempDir()

	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	tokenSvc := tokens.New(s)
	storageSvc := storage.New(storageRoot, s)
	eventLog := events.New(s)

	// Wire a failing backend into the Syncer so every PutObject returns an error
	// simulating NoSuchBucket from a misconfigured S3 endpoint.
	failBackend := &errBackend{err: errors.New("NoSuchBucket: bucket does not exist")}
	manifests := &objectstore.ManifestStore{Backend: failBackend}
	syncer := &objectstore.Syncer{
		Backend:   failBackend,
		Manifests: manifests,
		Storage:   storageSvc,
		QueueSize: 16,
	}
	emitter := &postreceive.Emitter{
		Log:     eventLog,
		Syncer:  syncer,
		Storage: storageSvc,
		// Lifecycle is nil — Syncer uses a noop lease handle (fencing token 0).
	}

	h := &githttp.Handler{
		Store:     s,
		Tokens:    tokenSvc,
		Storage:   storageSvc,
		Validator: &prereceive.Validator{MaxPackBytes: 50 * 1024 * 1024},
		Emitter:   emitter,
	}

	r := chi.NewRouter()
	h.Mount(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// Create account + session.
	acc, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: nextID("osf-acc"), Email: "osf@example.com", DisplayName: "osf",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	pair, err := tokenSvc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue token: %v", err)
	}
	token := pair.AccessToken

	orgID := nextID("osf-org")
	_, err = s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: "OSF Org", Slug: orgID, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	sessionID := nextID("osf-sess")
	_, err = s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessionID, OrgID: orgID, Name: "OSF Session", Goal: "osf",
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

	// Initialise bare repo and a working clone.
	bareDir := filepath.Join(storageRoot, "orgs", orgID, "sessions", sessionID+".git")
	workDir := initBareRepo(t, bareDir)

	// Commit with valid trailers so pre-receive accepts it.
	refName := fmt.Sprintf("refs/heads/jam/%s/%s/main", sessionID, acc.ID)
	makeCommitWithTrailers(t, workDir, sessionID, acc.ID, "src/hello.go", "package main")

	// Push — the object-storage backend will fail. Expect non-zero exit from git.
	host := strings.TrimPrefix(srv.URL, "http://")
	pushURLStr := fmt.Sprintf("http://x-access-token:%s@%s/%s/%s.git",
		token, host, orgID, sessionID)

	output, ok := runGitExpectFail(workDir, "push", pushURLStr,
		fmt.Sprintf("HEAD:%s", refName))

	t.Logf("push output: %s", output)

	// The push must NOT succeed — the portal must return non-2xx when object
	// storage is broken so the git client exits non-zero.
	if ok {
		t.Fatal("git push should have failed due to object-storage error, but it exited 0 (2xx) — RPO=0 violation")
	}
}

// TestReceivePack_ConcurrentPushSemaphore_BoundsConcurrency verifies that the
// per-instance receive-pack semaphore correctly limits concurrent handlers.
//
// Strategy: construct a Handler with ReceivePackSem capped at 2. Send 5
// concurrent requests, each with an io.Pipe body. The pipe's write end is held
// by the test; as long as no bytes are written, the server's io.Copy blocks
// waiting for body data, holding the semaphore slot. Once the semaphore is
// full the remaining requests hit the non-blocking select default: path and
// return 503 immediately. A generous time.Sleep gives all 5 goroutines time to
// reach the semaphore acquire point before we release the pipe writers.
//
// We do NOT exercise the full push path (pkt-line parsing, git subprocess,
// etc.). The semaphore is the outermost gate in receivePack; a blocking pipe
// body is enough to hold it.
func TestReceivePack_ConcurrentPushSemaphore_BoundsConcurrency(t *testing.T) {
	const (
		semCap = 2
		total  = 5
	)

	ctx := context.Background()
	storageRoot := t.TempDir()

	// Use a file-based SQLite DB (not :memory:) so that concurrent reads from
	// multiple goroutines go through the WAL path rather than serialising on a
	// single in-memory connection. busy_timeout(5000) is injected automatically
	// by db.Open, so brief write contention does not cause spurious 500 errors
	// that would obscure the semaphore-rejection assertions below.
	dbPath := filepath.Join(t.TempDir(), "sem_test.db")
	s, rawDB, err := db.Open(ctx, "sqlite", dbPath, db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Enable WAL mode so concurrent goroutines can hold read transactions
	// simultaneously while the test goroutine (writer) sets up fixtures.
	if _, err := rawDB.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("enable WAL: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	tokenSvc := tokens.New(s)
	storageSvc := storage.New(storageRoot, s)

	h := &githttp.Handler{
		Store:          s,
		Tokens:         tokenSvc,
		Storage:        storageSvc,
		Validator:      &prereceive.Validator{MaxPackBytes: 50 * 1024 * 1024},
		Emitter:        nil,
		ReceivePackSem: make(chan struct{}, semCap),
	}

	r := chi.NewRouter()
	h.Mount(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// Set up account + session so the auth/session middleware chains pass.
	acc, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: nextID("sem-acc"), Email: "sem@example.com", DisplayName: "sem",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	pair, err := tokenSvc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue token: %v", err)
	}
	token := pair.AccessToken

	orgID := nextID("sem-org")
	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: "Sem Org", Slug: orgID, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	sessionID := nextID("sem-sess")
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessionID, OrgID: orgID, Name: "Sem Session", Goal: "sem",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: orgID, SessionID: sessionID, AccountID: acc.ID,
		Role: "member", JoinedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddSessionMember: %v", err)
	}

	// Create a bare repo so that checkArchived's LookupArchived returns
	// ErrNotFound (not-archived), allowing the request to reach receivePack.
	// Without a bare repo, the storage service may behave unexpectedly.
	bareDir := filepath.Join(storageRoot, "orgs", orgID, "sessions", sessionID+".git")
	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}
	runGit(t, "", "init", "--bare", bareDir)

	// Create one io.Pipe per request. The write end (pw) is held by the test;
	// as long as pw is open and no bytes are written, the server's
	// io.Copy(bodyFile, r.Body) blocks, holding the semaphore slot.
	prs := make([]*io.PipeReader, total)
	pws := make([]*io.PipeWriter, total)
	for i := range prs {
		prs[i], pws[i] = io.Pipe()
	}
	// Ensure all pipe writers are closed when the test finishes, so goroutines
	// that are still blocked can exit cleanly.
	t.Cleanup(func() {
		for _, pw := range pws {
			pw.Close()
		}
	})

	endpointURL := srv.URL + "/" + orgID + "/" + sessionID + ".git/git-receive-pack"

	// Use a transport that does not reuse connections, so all 5 requests hit
	// separate server goroutines and contend on the semaphore concurrently.
	transport := &http.Transport{DisableKeepAlives: true}

	type result struct {
		status     int
		retryAfter string
	}
	results := make(chan result, total)

	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodPost, endpointURL, prs[i])
			if err != nil {
				results <- result{status: -1}
				return
			}
			req.SetBasicAuth("x-access-token", token)
			req.Header.Set("Content-Type", "application/x-git-receive-pack-request")

			client := &http.Client{Transport: transport}
			resp, err := client.Do(req)
			if err != nil {
				// A pipe-closed error arrives when the test closes pw[i] while
				// the server hasn't responded yet. Count as -1; not expected for
				// the 503 path (which responds before reading any body).
				results <- result{status: -1}
				return
			}
			defer resp.Body.Close()
			results <- result{
				status:     resp.StatusCode,
				retryAfter: resp.Header.Get("Retry-After"),
			}
		}(i)
	}

	// Wait for all 5 goroutines to reach the server and contend on the
	// semaphore. 200 ms is generous on any scheduler — the httptest.Server
	// dispatches goroutines synchronously as connections arrive.
	//
	// Why sleep rather than poll: the only observable signal from outside the
	// handler is the semaphore channel length, but reading len() on a channel
	// is non-atomic and not guaranteed to see the final state before a
	// concurrent select runs. A fixed sleep with a comfortable margin is
	// simpler and more predictable than a spin-poll.
	time.Sleep(200 * time.Millisecond)

	// At this point: semCap handlers hold the semaphore and are blocked on
	// io.Copy waiting for body bytes; the remaining (total-semCap) handlers
	// have already returned 503 and are no longer in flight.
	//
	// Close the pipe writers for the admitted requests so their io.Copy
	// completes (body = empty → 400 malformed pkt-line) and they return.
	for i := 0; i < semCap; i++ {
		pws[i].Close()
	}

	// Collect only the admitted results first; the 503s already landed.
	// Then close remaining pipes (for requests that got 503 and whose
	// goroutines have already returned, these are no-ops).
	for i := semCap; i < total; i++ {
		pws[i].Close()
	}

	wg.Wait()
	close(results)

	var (
		admitted int // requests that acquired the semaphore (200 or 400)
		got503   int
		badRA    []int // 503s missing Retry-After
		other    []int
	)
	for res := range results {
		switch res.status {
		case http.StatusOK, http.StatusBadRequest:
			// 200: full pipeline ran (unexpected here, but not wrong)
			// 400: semaphore acquired, body empty → malformed pkt-line
			// Both mean the request was admitted (semaphore slot granted).
			admitted++
		case http.StatusServiceUnavailable:
			got503++
			if res.retryAfter == "" {
				badRA = append(badRA, res.status)
			}
		default:
			other = append(other, res.status)
		}
	}

	// At most semCap requests should have been admitted.
	if admitted > semCap {
		t.Errorf("semaphore over-admitted: %d requests acquired a slot, cap=%d", admitted, semCap)
	}

	// The remainder must have been rejected with 503.
	expectedRejections := total - semCap
	if got503 < expectedRejections {
		t.Errorf("expected at least %d 503 responses (semaphore rejections), got %d (admitted=%d other=%v)",
			expectedRejections, got503, admitted, other)
	}

	// Every 503 must carry Retry-After.
	if len(badRA) > 0 {
		t.Errorf("%d 503 response(s) missing Retry-After header", len(badRA))
	}

	t.Logf("semaphore test: admitted=%d, rejected-503=%d, other=%v (cap=%d, total=%d)",
		admitted, got503, other, semCap, total)
}

// ---------------------------------------------------------------------------
// errBackend — a Backend that returns a fixed error on every write.
// ---------------------------------------------------------------------------

// errBackend implements objectstore.Backend with all writes returning a
// configurable error. Reads return ErrNotFound. Used to simulate a broken
// S3 endpoint (e.g. NoSuchBucket) without real infrastructure.
type errBackend struct {
	mu  sync.Mutex
	err error
}

func (b *errBackend) Put(_ context.Context, _ string, _ []byte, _ int64, _ string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return "", b.err
}

func (b *errBackend) PutIdempotent(_ context.Context, _ string, _ []byte, _ int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

func (b *errBackend) Get(_ context.Context, _ string) ([]byte, string, int64, error) {
	return nil, "", 0, objectstore.ErrNotFound
}

func (b *errBackend) Delete(_ context.Context, _ string) error { return nil }

func (b *errBackend) List(_ context.Context, _ string, _ func(string) error) error { return nil }

func (b *errBackend) Probe(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

// Ensure errBackend is used as objectstore.Backend (compile-time check).
var _ objectstore.Backend = (*errBackend)(nil)

// ---------------------------------------------------------------------------
// base_sha stamping tests
// ---------------------------------------------------------------------------

// passthroughStore wraps a real store.Store and allows selectively overriding
// SetSessionBaseSHA for failure-injection tests.
type passthroughStore struct {
	store.Store
	setBaseSHAFn func(ctx context.Context, p store.SetSessionBaseSHAParams) error
}

func (s *passthroughStore) SetSessionBaseSHA(ctx context.Context, p store.SetSessionBaseSHAParams) error {
	if s.setBaseSHAFn != nil {
		return s.setBaseSHAFn(ctx, p)
	}
	return s.Store.SetSessionBaseSHA(ctx, p)
}

// TestPostReceive_BaseRefStampsBaseSHA verifies that pushing the base ref
// (refs/heads/jam/<sessionID>/base) for the first time causes the post-receive
// handler to call SetSessionBaseSHA, populating sessions.base_sha with the
// pushed commit SHA.
func TestPostReceive_BaseRefStampsBaseSHA(t *testing.T) {
	env := newPushEnv(t)
	acc, token := env.mustIssueToken(t, "base-sha-alice@example.com")
	orgID, sessionID := env.mustCreateSession(t, acc, `["**"]`)

	bareDir := filepath.Join(env.storageRoot, "orgs", orgID, "sessions", sessionID+".git")
	workDir := initBareRepo(t, bareDir)

	// Commit with valid trailers so pre-receive accepts the push.
	makeCommitWithTrailers(t, workDir, sessionID, acc.ID, "src/init.go", "package main")

	// Capture the local HEAD SHA so we can verify it's stamped.
	headSHA := runGit(t, workDir, "rev-parse", "HEAD")

	// Push to the base ref — this is what the CLI's pushBaseRef does.
	baseRef := fmt.Sprintf("refs/heads/jam/%s/base", sessionID)
	pushURLStr := env.pushURL(token, orgID, sessionID)
	pushCmd := exec.Command("git", "push", pushURLStr, fmt.Sprintf("HEAD:%s", baseRef))
	pushCmd.Dir = workDir
	pushCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := pushCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git push base ref failed: %v\n%s", err, out)
	}

	// Verify base_sha is now populated on the session row.
	ctx := context.Background()
	sess, err := env.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.BaseSHA == nil {
		t.Fatal("sessions.base_sha is nil after base ref push; expected it to be stamped")
	}
	if *sess.BaseSHA != headSHA {
		t.Errorf("sessions.base_sha = %q; want %q", *sess.BaseSHA, headSHA)
	}
}

// TestPostReceive_NonBaseRefDoesNotReStamp verifies that pushing a non-base
// ref (e.g. a user working ref) does not overwrite a previously-stamped
// base_sha.
func TestPostReceive_NonBaseRefDoesNotReStamp(t *testing.T) {
	env := newPushEnv(t)
	acc, token := env.mustIssueToken(t, "base-sha-bob@example.com")
	orgID, sessionID := env.mustCreateSession(t, acc, `["**"]`)

	bareDir := filepath.Join(env.storageRoot, "orgs", orgID, "sessions", sessionID+".git")
	workDir := initBareRepo(t, bareDir)

	// First, push the base ref so the repo is seeded and base_sha is stamped.
	makeCommitWithTrailers(t, workDir, sessionID, acc.ID, "src/init.go", "package main")
	baseSHA := runGit(t, workDir, "rev-parse", "HEAD")
	baseRef := fmt.Sprintf("refs/heads/jam/%s/base", sessionID)
	pushURLStr := env.pushURL(token, orgID, sessionID)
	pushCmd := exec.Command("git", "push", pushURLStr, fmt.Sprintf("HEAD:%s", baseRef))
	pushCmd.Dir = workDir
	pushCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := pushCmd.CombinedOutput(); err != nil {
		t.Fatalf("base ref push failed: %v\n%s", err, out)
	}

	// Confirm base_sha was stamped.
	ctx := context.Background()
	sess, err := env.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		t.Fatalf("GetSession after base push: %v", err)
	}
	if sess.BaseSHA == nil || *sess.BaseSHA != baseSHA {
		t.Fatalf("pre-condition: base_sha not stamped correctly (got %v)", sess.BaseSHA)
	}

	// Now push a user working ref (not the base ref).
	makeCommitWithTrailers(t, workDir, sessionID, acc.ID, "src/work.go", "package main // v2")
	userRef := fmt.Sprintf("refs/heads/jam/%s/%s/main", sessionID, acc.ID)
	pushCmd2 := exec.Command("git", "push", pushURLStr, fmt.Sprintf("HEAD:%s", userRef))
	pushCmd2.Dir = workDir
	pushCmd2.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := pushCmd2.CombinedOutput(); err != nil {
		t.Fatalf("user ref push failed: %v\n%s", err, out)
	}

	// Verify base_sha is unchanged — still the original base commit SHA.
	sess2, err := env.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		t.Fatalf("GetSession after user push: %v", err)
	}
	if sess2.BaseSHA == nil {
		t.Fatal("base_sha became nil after user ref push")
	}
	if *sess2.BaseSHA != baseSHA {
		t.Errorf("base_sha changed after user ref push: got %q, want %q", *sess2.BaseSHA, baseSHA)
	}
}

// TestPostReceive_SetBaseSHAFailureIsNonFatal verifies the non-fatal
// degradation path: when SetSessionBaseSHA returns an error (e.g. transient DB
// failure), the push still completes successfully from the git client's
// perspective (exit 0, 200 OK). The push is not rolled back.
func TestPostReceive_SetBaseSHAFailureIsNonFatal(t *testing.T) {
	// Build the environment manually so we can inject a wrapping store that
	// fails SetSessionBaseSHA.
	ctx := context.Background()
	storageRoot := t.TempDir()

	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Wrap the real store so only SetSessionBaseSHA fails.
	var setBaseSHACalled bool
	wrappedStore := &passthroughStore{
		Store: s,
		setBaseSHAFn: func(_ context.Context, _ store.SetSessionBaseSHAParams) error {
			setBaseSHACalled = true
			return errors.New("transient DB error injected by test")
		},
	}

	tokenSvc := tokens.New(s) // token operations use the real store directly
	storageSvc := storage.New(storageRoot, s)
	eventLog := events.New(s)

	h := &githttp.Handler{
		Store:     wrappedStore,
		Tokens:    tokenSvc,
		Storage:   storageSvc,
		Validator: &prereceive.Validator{MaxPackBytes: 50 * 1024 * 1024},
		Emitter:   &postreceive.Emitter{Log: eventLog},
	}

	r := chi.NewRouter()
	h.Mount(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// Create account, org, session using the real (unwrapped) store for fixtures.
	acc, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID:          nextID("bsf-acc"),
		Email:       "base-sha-fail@example.com",
		DisplayName: "base-sha-fail",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	pair, err := tokenSvc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue token: %v", err)
	}
	token := pair.AccessToken

	orgID := nextID("bsf-org")
	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: "BSF Org", Slug: orgID, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	sessionID := nextID("bsf-sess")
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessionID, OrgID: orgID, Name: "BSF Session", Goal: "bsf",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: orgID, SessionID: sessionID, AccountID: acc.ID,
		Role: "member", JoinedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddSessionMember: %v", err)
	}

	bareDir := filepath.Join(storageRoot, "orgs", orgID, "sessions", sessionID+".git")
	workDir := initBareRepo(t, bareDir)
	makeCommitWithTrailers(t, workDir, sessionID, acc.ID, "src/init.go", "package main")

	// Push the base ref — SetSessionBaseSHA will fail inside the handler.
	host := strings.TrimPrefix(srv.URL, "http://")
	pushURLStr := fmt.Sprintf("http://x-access-token:%s@%s/%s/%s.git",
		token, host, orgID, sessionID)
	baseRef := fmt.Sprintf("refs/heads/jam/%s/base", sessionID)
	pushCmd := exec.Command("git", "push", pushURLStr, fmt.Sprintf("HEAD:%s", baseRef))
	pushCmd.Dir = workDir
	pushCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := pushCmd.CombinedOutput()

	// The push must still succeed despite the SetSessionBaseSHA failure.
	if err != nil {
		t.Fatalf("git push should succeed even when SetSessionBaseSHA fails; got error: %v\n%s", err, out)
	}

	// Verify SetSessionBaseSHA was actually called (so we know the code path ran).
	if !setBaseSHACalled {
		t.Error("SetSessionBaseSHA was not called; expected it to be attempted on base ref push")
	}

	// Verify the ref landed in the bare repo (push really did succeed).
	revParseOut := runGit(t, bareDir, "rev-parse", fmt.Sprintf("refs/heads/jam/%s/base", sessionID))
	if revParseOut == "" {
		t.Error("base ref not found in bare repo after push")
	}
}
