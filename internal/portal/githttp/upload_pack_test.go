package githttp_test

import (
	"context"
	"encoding/json"
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
	"jamsesh/internal/portal/githttp"
	"jamsesh/internal/portal/prereceive"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Helpers specific to upload-pack / fetch tests
// ---------------------------------------------------------------------------

// newFetchEnv creates a test environment rooted at a real temp directory so
// bare repos can be created on disk.
func newFetchEnv(t *testing.T) (*testFetchEnv, string) {
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

	h := &githttp.Handler{
		Store:     s,
		Tokens:    tokenSvc,
		Storage:   storageSvc,
		Validator: &prereceive.Validator{MaxPackBytes: 50 * 1024 * 1024},
		Emitter:   nil,
	}

	r := chi.NewRouter()
	h.Mount(r)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &testFetchEnv{
		store:      s,
		tokenSvc:   tokenSvc,
		storageSvc: storageSvc,
		storageRoot: storageRoot,
		server:     srv,
	}, storageRoot
}

type testFetchEnv struct {
	store       store.Store
	tokenSvc    tokens.Service
	storageSvc  storage.Service
	storageRoot string
	server      *httptest.Server
}

func (e *testFetchEnv) mustIssueToken(t *testing.T, email string) (store.Account, string) {
	t.Helper()
	ctx := context.Background()

	acc, err := e.store.CreateAccount(ctx, store.CreateAccountParams{
		ID:          nextID("facc"),
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

func (e *testFetchEnv) mustCreateSessionWithMember(t *testing.T, acc store.Account) (orgID, sessionID string) {
	t.Helper()
	ctx := context.Background()

	orgID = nextID("forg")
	org, err := e.store.CreateOrg(ctx, store.CreateOrgParams{
		ID:        orgID,
		Name:      "Fetch Org",
		Slug:      orgID,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}

	sessionID = nextID("fsess")
	_, err = e.store.CreateSession(ctx, store.CreateSessionParams{
		ID:            sessionID,
		OrgID:         org.ID,
		Name:          "Fetch Session",
		Goal:          "fetch testing",
		WritableScope: `["src/"]`,
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

// gitRun runs a git command and fails the test if it exits non-zero.
func gitRun(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// gitRunDir runs a git command in a specific directory.
func gitRunDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

// makeSyntheticBareRepo creates a bare repo at bareDir with a few commits.
// It does this by initialising a normal repo, committing some files, then
// cloning it as --bare into bareDir.
func makeSyntheticBareRepo(t *testing.T, bareDir string) (commitSHA string) {
	t.Helper()

	workDir := t.TempDir()

	// Init a normal (non-bare) working repo.
	gitRunDir(t, workDir, "init", "-b", "main")
	gitRunDir(t, workDir, "config", "user.email", "test@example.com")
	gitRunDir(t, workDir, "config", "user.name", "Test User")

	// Add a few commits.
	for i, content := range []string{"hello", "world"} {
		fp := filepath.Join(workDir, "file.txt")
		if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
			t.Fatalf("write file %d: %v", i, err)
		}
		gitRunDir(t, workDir, "add", "file.txt")
		gitRunDir(t, workDir, "commit", "-m", content)
	}

	// Get the HEAD SHA.
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = workDir
	sha, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	commitSHA = strings.TrimSpace(string(sha))

	// Clone as bare into bareDir.
	_ = os.MkdirAll(bareDir, 0o755)
	gitRun(t, "clone", "--bare", workDir, bareDir)

	return commitSHA
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestInfoRefs_UploadPack verifies that GET /info/refs?service=git-upload-pack
// returns 200, the correct Content-Type, and a valid smart-HTTP advertisement.
func TestInfoRefs_UploadPack(t *testing.T) {
	env, storageRoot := newFetchEnv(t)
	acc, token := env.mustIssueToken(t, "fetch-alice@example.com")
	orgID, sessionID := env.mustCreateSessionWithMember(t, acc)

	// Create the bare repo at the path the storage service expects.
	bareDir := filepath.Join(storageRoot, "orgs", orgID, "sessions", sessionID+".git")
	makeSyntheticBareRepo(t, bareDir)

	url := env.server.URL + "/" + orgID + "/" + sessionID + ".git/info/refs?service=git-upload-pack"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.SetBasicAuth("x-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET info/refs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	want := "application/x-git-upload-pack-advertisement"
	if got := resp.Header.Get("Content-Type"); got != want {
		t.Errorf("Content-Type = %q; want %q", got, want)
	}

	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q; want no-cache", got)
	}
}

// TestInfoRefs_InvalidService verifies that unsupported service values yield 400.
func TestInfoRefs_InvalidService(t *testing.T) {
	env, storageRoot := newFetchEnv(t)
	acc, token := env.mustIssueToken(t, "fetch-bob@example.com")
	orgID, sessionID := env.mustCreateSessionWithMember(t, acc)

	bareDir := filepath.Join(storageRoot, "orgs", orgID, "sessions", sessionID+".git")
	makeSyntheticBareRepo(t, bareDir)

	for _, svc := range []string{"git-bad-pack", "", "../../etc/passwd"} {
		url := env.server.URL + "/" + orgID + "/" + sessionID + ".git/info/refs?service=" + svc
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.SetBasicAuth("x-access-token", token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET info/refs service=%q: %v", svc, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("service=%q: want 400, got %d", svc, resp.StatusCode)
		}
	}
}

// TestGitClone_EndToEnd performs a real `git clone` against an httptest server
// hosting a synthetic bare repo, verifying that the fetch path works end-to-end.
func TestGitClone_EndToEnd(t *testing.T) {
	env, storageRoot := newFetchEnv(t)
	acc, token := env.mustIssueToken(t, "fetch-carol@example.com")
	orgID, sessionID := env.mustCreateSessionWithMember(t, acc)

	// Create the bare repo at the storage path.
	bareDir := filepath.Join(storageRoot, "orgs", orgID, "sessions", sessionID+".git")
	wantSHA := makeSyntheticBareRepo(t, bareDir)

	// Build the clone URL with credentials embedded.
	// Format: http://x-access-token:<token>@host/orgID/sessionID.git
	host := strings.TrimPrefix(env.server.URL, "http://")
	cloneURL := "http://x-access-token:" + token + "@" + host +
		"/" + orgID + "/" + sessionID + ".git"

	cloneDir := t.TempDir()

	cmd := exec.Command("git", "clone", cloneURL, cloneDir)
	cmd.Env = append(os.Environ(),
		// Suppress credential prompting in CI.
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git clone failed: %v\n%s", err, out)
	}

	// Verify HEAD commit matches what we seeded.
	check := exec.Command("git", "rev-parse", "HEAD")
	check.Dir = cloneDir
	gotSHA, err := check.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD in clone: %v", err)
	}

	if got := strings.TrimSpace(string(gotSHA)); got != wantSHA {
		t.Errorf("clone HEAD = %q; want %q", got, wantSHA)
	}
}

// TestInfoRefs_SubprocessFailure_ReturnsDepGitSubprocessFailed verifies that
// when the `git` subprocess for info/refs exits non-zero (here forced by
// pointing the handler at a non-existent repo path on disk while auth still
// passes), the response is a 500 carrying the typed
// `{"error":"dep.git_subprocess_failed",...}` envelope with NO Retry-After
// header. This is the dep-failure error-code contract for git smart-HTTP.
func TestInfoRefs_SubprocessFailure_ReturnsDepGitSubprocessFailed(t *testing.T) {
	env, _ := newFetchEnv(t)
	acc, token := env.mustIssueToken(t, "fetch-dep-fail@example.com")
	orgID, sessionID := env.mustCreateSessionWithMember(t, acc)

	// Deliberately skip creating the bare repo. The auth + archive middleware
	// chain only consults the DB (membership + archived_sessions), so the
	// request reaches the infoRefs handler. The handler spawns
	// `git upload-pack --stateless-rpc --advertise-refs <nonexistent-path>`
	// which exits non-zero — exactly the subprocess-failure path we want to
	// assert on.

	url := env.server.URL + "/" + orgID + "/" + sessionID +
		".git/info/refs?service=git-upload-pack"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.SetBasicAuth("x-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET info/refs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q; want application/json; charset=utf-8", got)
	}
	if got := resp.Header.Get("Retry-After"); got != "" {
		t.Errorf("Retry-After = %q; want unset for git subprocess failure", got)
	}

	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error != "dep.git_subprocess_failed" {
		t.Errorf("error = %q; want dep.git_subprocess_failed", body.Error)
	}
	if body.Message == "" {
		t.Error("message field is empty; expected a human-readable message")
	}
}
