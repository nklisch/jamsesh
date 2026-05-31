package finalizecmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/internal/api/openapi"
)

// setupFetchSourceEnv wires the per-session state directory the
// chooser reads from (org_id sidecar + local_path placeholder when
// the test wants the local-first branch).
func setupFetchSourceEnv(t *testing.T, sessionID string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)

	_ = os.Chmod(dir, 0o700)
	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}
	return dir
}

// writeTokenFile creates the access-token state file portalclient.Do
// reads on every request. The HTTPS fallback path needs this so the
// bearer attach step doesn't fail before the test server sees anything.
func writeTokenFile(t *testing.T, dir, tok string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(tok), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
}

// TestChooseFetchSource_LocalFirstHappy verifies the local-first branch:
// when local_path exists and points to a valid git repo, the chooser
// returns Kind:"local" and never touches the portal.
func TestChooseFetchSource_LocalFirstHappy(t *testing.T) {
	sessionID := "sess-local"
	dir := setupFetchSourceEnv(t, sessionID)

	// Prepare a real git repo so looksLikeGitRepo passes (worktree
	// flavor — contains .git).
	localRepo := t.TempDir()
	mustGitCwd(t, localRepo, "init")
	// Write the local_path state file pointing at it.
	if err := os.WriteFile(filepath.Join(dir, "sessions", sessionID, "local_path"),
		[]byte(localRepo), 0o600); err != nil {
		t.Fatalf("write local_path: %v", err)
	}

	// Portal server that fails the test if it's hit — local-first must
	// short-circuit before any portal call.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("portal hit on local-first path: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	pc := &portalclient.Client{BaseURL: srv.URL}

	fs, err := chooseFetchSource(context.Background(), pc, &Plan{}, "org1", sessionID)
	if err != nil {
		t.Fatalf("chooseFetchSource: %v", err)
	}
	if fs.Kind != "local" {
		t.Errorf("Kind = %q, want local", fs.Kind)
	}
	if fs.URL != localRepo {
		t.Errorf("URL = %q, want %q", fs.URL, localRepo)
	}
	if err := fs.cleanup(); err != nil {
		t.Errorf("local cleanup should be a no-op, got: %v", err)
	}
}

// TestChooseFetchSource_LocalPathMissingFallsBackToHTTPS verifies that
// an absent local_path file routes us through the HTTPS-fallback path.
// The portal mock returns a canned token response and the chooser must
// register the jamsesh remote.
func TestChooseFetchSource_LocalPathMissingFallsBackToHTTPS(t *testing.T) {
	sessionID := "sess-https"
	dir := setupFetchSourceEnv(t, sessionID)
	writeTokenFile(t, dir, "test-bearer-token")
	// No local_path file written → local-first miss → HTTPS fallback.

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/api/orgs/org1/sessions/" + sessionID + "/finalize/fetch-token"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-bearer-token" {
			t.Errorf("Authorization = %q, want Bearer test-bearer-token", got)
		}
		resp := openapi.FetchTokenResponse{
			Token:     "ephem-fetch-tok",
			RemoteUrl: "https://portal.example/git/org1/" + sessionID + ".git",
			ExpiresAt: time.Now().Add(5 * time.Minute).UTC(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	pc := &portalclient.Client{BaseURL: srv.URL}

	// Stub the git invocations the HTTPS branch calls so we don't
	// need a real repo for this test.
	var gitArgs [][]string
	oldRunGit := runGit
	oldRunGitCombined := runGitCombined
	t.Cleanup(func() {
		runGit = oldRunGit
		runGitCombined = oldRunGitCombined
	})
	runGit = func(args ...string) error {
		gitArgs = append(gitArgs, append([]string(nil), args...))
		return nil
	}
	runGitCombined = func(args ...string) (string, error) {
		gitArgs = append(gitArgs, append([]string(nil), args...))
		return "", nil
	}

	fs, err := chooseFetchSource(context.Background(), pc, &Plan{}, "org1", sessionID)
	if err != nil {
		t.Fatalf("chooseFetchSource: %v", err)
	}
	if fs.Kind != "https" {
		t.Errorf("Kind = %q, want https", fs.Kind)
	}
	// URL must be plain — no embedded credentials.
	if strings.Contains(fs.URL, "x-access-token") || strings.Contains(fs.URL, "@") {
		t.Errorf("URL must not contain embedded credentials: %q", fs.URL)
	}
	if !strings.Contains(fs.URL, "portal.example/git/org1/") {
		t.Errorf("URL missing expected path: %q", fs.URL)
	}
	// Token must be stored separately on the fetchSource.
	if fs.Token != "ephem-fetch-tok" {
		t.Errorf("Token = %q, want ephem-fetch-tok", fs.Token)
	}
	// Should have run: pre-clean remote remove (combined) + remote add.
	if len(gitArgs) < 2 {
		t.Fatalf("expected ≥2 git invocations, got %d: %v", len(gitArgs), gitArgs)
	}
	// First call: pre-clean `remote remove jamsesh` (best-effort).
	if !equalStrSlice(gitArgs[0], []string{"remote", "remove", "jamsesh"}) {
		t.Errorf("first git call = %v, want pre-clean remote remove", gitArgs[0])
	}
	// Second call: `remote add jamsesh <url>` — URL must be credential-free.
	if len(gitArgs[1]) < 4 || gitArgs[1][0] != "remote" || gitArgs[1][1] != "add" || gitArgs[1][2] != "jamsesh" {
		t.Errorf("second git call = %v, want remote add jamsesh <url>", gitArgs[1])
	}
	if strings.Contains(gitArgs[1][3], "x-access-token") || strings.Contains(gitArgs[1][3], "@") {
		t.Errorf("remote add URL must not contain embedded credentials: %v", gitArgs[1])
	}
}

// TestChooseFetchSource_LocalPathNotARepoFallsBackToHTTPS verifies that
// a local_path pointing at a non-git directory triggers the HTTPS
// fallback rather than returning a broken local source.
func TestChooseFetchSource_LocalPathNotARepoFallsBackToHTTPS(t *testing.T) {
	sessionID := "sess-bogus"
	dir := setupFetchSourceEnv(t, sessionID)
	writeTokenFile(t, dir, "tok")

	// Point local_path at a real directory that is NOT a git repo.
	notRepo := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sessions", sessionID, "local_path"),
		[]byte(notRepo), 0o600); err != nil {
		t.Fatalf("write local_path: %v", err)
	}

	httpsHit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpsHit = true
		resp := openapi.FetchTokenResponse{
			Token:     "t",
			RemoteUrl: "https://portal.example/git/org1/" + sessionID + ".git",
			ExpiresAt: time.Now().Add(5 * time.Minute).UTC(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	oldRunGit := runGit
	oldRunGitCombined := runGitCombined
	t.Cleanup(func() {
		runGit = oldRunGit
		runGitCombined = oldRunGitCombined
	})
	runGit = func(args ...string) error { return nil }
	runGitCombined = func(args ...string) (string, error) { return "", nil }

	pc := &portalclient.Client{BaseURL: srv.URL}
	fs, err := chooseFetchSource(context.Background(), pc, &Plan{}, "org1", sessionID)
	if err != nil {
		t.Fatalf("chooseFetchSource: %v", err)
	}
	if !httpsHit {
		t.Errorf("expected HTTPS fallback when local_path is not a repo")
	}
	if fs.Kind != "https" {
		t.Errorf("Kind = %q, want https", fs.Kind)
	}
}

// TestChooseFetchSource_HTTPS_401PropagatesError verifies that a 401
// from the token endpoint surfaces as an error (after the portalclient
// gives up retrying — Refresh isn't wired in this test, so the first
// 401 returns immediately).
func TestChooseFetchSource_HTTPS_401PropagatesError(t *testing.T) {
	sessionID := "sess-401"
	dir := setupFetchSourceEnv(t, sessionID)
	writeTokenFile(t, dir, "stale-tok")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"auth.invalid_token","message":"invalid token"}`))
	}))
	defer srv.Close()
	pc := &portalclient.Client{BaseURL: srv.URL} // no Refresh — single-shot 401

	_ = dir
	_, err := chooseFetchSource(context.Background(), pc, &Plan{}, "org1", sessionID)
	if err == nil {
		t.Fatalf("expected error from 401 token endpoint")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should reference the 401 status: %v", err)
	}
}

// TestChooseFetchSource_HTTPS_EmptyRemoteURLErrors guards against a
// degenerate portal response (token issued but remote_url empty) —
// without this check the caller would `git remote add jamsesh` with no
// URL, which git would happily reject with a confusing message.
func TestChooseFetchSource_HTTPS_EmptyRemoteURLErrors(t *testing.T) {
	sessionID := "sess-empty"
	dir := setupFetchSourceEnv(t, sessionID)
	writeTokenFile(t, dir, "tok")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"token":"t","remote_url":"","expires_at":"2030-01-01T00:00:00Z"}`))
	}))
	defer srv.Close()
	pc := &portalclient.Client{BaseURL: srv.URL}
	_ = dir

	_, err := chooseFetchSource(context.Background(), pc, &Plan{}, "org1", sessionID)
	if err == nil || !strings.Contains(err.Error(), "empty remote_url") {
		t.Fatalf("expected 'empty remote_url' error, got: %v", err)
	}
}

// TestRemoveJamseshRemote_IdempotentOnMissing verifies the cleanup func
// swallows the "No such remote" error from git so double-invocation
// (main flow + SIGINT watcher) is harmless.
func TestRemoveJamseshRemote_IdempotentOnMissing(t *testing.T) {
	repo := gitInTempRepo(t)
	pinGitToCwd(t, repo)

	// First call: no jamsesh remote exists — must NOT error.
	if err := removeJamseshRemote(); err != nil {
		t.Errorf("first removeJamseshRemote: %v", err)
	}
	// Second call: still no remote — still must NOT error.
	if err := removeJamseshRemote(); err != nil {
		t.Errorf("second removeJamseshRemote: %v", err)
	}
}

// TestRemoveJamseshRemote_RemovesExistingRemote verifies the happy path:
// when the jamsesh remote IS present, removeJamseshRemote successfully
// deletes it and a subsequent `git remote -v` shows no entry.
func TestRemoveJamseshRemote_RemovesExistingRemote(t *testing.T) {
	repo := gitInTempRepo(t)
	pinGitToCwd(t, repo)

	if err := runGit("remote", "add", "jamsesh", "https://example.com/foo.git"); err != nil {
		t.Fatalf("remote add: %v", err)
	}
	if err := removeJamseshRemote(); err != nil {
		t.Errorf("removeJamseshRemote: %v", err)
	}
	// Verify gone.
	listed, err := runGitOutput("remote", "-v")
	if err != nil {
		t.Fatalf("remote -v: %v", err)
	}
	if strings.Contains(listed, "jamsesh") {
		t.Errorf("jamsesh remote still present after removal:\n%s", listed)
	}
}

// TestPerformFetch_LocalKind verifies performFetch routes the local-kind
// source into `git fetch <path>`. Uses a real local repo as the source
// so git is happy with the URL.
func TestPerformFetch_LocalKind(t *testing.T) {
	// Source repo with a commit.
	src := gitInTempRepo(t)
	commit(t, src, "a.txt", "a", "init")

	// Destination repo — we'll pin git to here.
	dst := gitInTempRepo(t)
	pinGitToCwd(t, dst)

	fs := &fetchSource{Kind: "local", URL: src, cleanup: func() error { return nil }}
	var buf bytes.Buffer
	if err := performFetch(&buf, fs); err != nil {
		t.Fatalf("performFetch: %v", err)
	}
	if !strings.Contains(buf.String(), "+ git fetch "+src) {
		t.Errorf("missing verbose fetch log:\n%s", buf.String())
	}
}

func envLookup(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix)
		}
	}
	return ""
}

// TestPerformFetch_HTTPSKindPassesTokenViaExtraHeader verifies the HTTPS
// branch fetches by the named `jamsesh` remote and passes the bearer token
// through Git's GIT_CONFIG_* environment channel as http.extraHeader rather
// than embedding it in the remote URL or argv.
func TestPerformFetch_HTTPSKindPassesTokenViaExtraHeader(t *testing.T) {
	var gotArgs []string
	var gotEnv []string
	oldRunGitWithEnv := runGitWithEnv
	t.Cleanup(func() { runGitWithEnv = oldRunGitWithEnv })
	runGitWithEnv = func(env []string, args ...string) error {
		gotEnv = append([]string(nil), env...)
		gotArgs = append([]string(nil), args...)
		return nil
	}

	fs := &fetchSource{
		Kind:    "https",
		URL:     "https://portal.example/git/org1/sess1.git",
		Token:   "secret-token",
		cleanup: func() error { return nil },
	}
	var buf bytes.Buffer
	if err := performFetch(&buf, fs); err != nil {
		t.Fatalf("performFetch: %v", err)
	}
	want := []string{"fetch", "jamsesh"}
	if !equalStrSlice(gotArgs, want) {
		t.Errorf("git args = %v, want %v", gotArgs, want)
	}
	if got := envLookup(gotEnv, "GIT_CONFIG_KEY_0"); got != "http.extraHeader" {
		t.Errorf("GIT_CONFIG_KEY_0 = %q, want http.extraHeader; env=%v", got, gotEnv)
	}
	if got := envLookup(gotEnv, "GIT_CONFIG_VALUE_0"); got != "Authorization: Bearer secret-token" {
		t.Errorf("GIT_CONFIG_VALUE_0 = %q, want bearer header", got)
	}
	for _, arg := range gotArgs {
		if strings.Contains(arg, "secret-token") || strings.Contains(arg, "http.extraHeader") {
			t.Errorf("credential material leaked into git argv: %q", arg)
		}
	}
	// CRITICAL: the remote URL passed to git remote add must not contain
	// the token — the URL in the remote config is credential-free.
	if strings.Contains(buf.String(), "x-access-token:") {
		t.Errorf("verbose log contains credential-bearing URL form:\n%s", buf.String())
	}
}

// TestPerformFetch_UnknownKindErrors covers the defensive default in
// performFetch — a kind value that isn't local or https is a hard
// programmer error.
func TestPerformFetch_UnknownKindErrors(t *testing.T) {
	fs := &fetchSource{Kind: "rsync", URL: "?"}
	var buf bytes.Buffer
	err := performFetch(&buf, fs)
	if err == nil || !strings.Contains(err.Error(), "unknown fetch source kind") {
		t.Errorf("expected unknown-kind error, got %v", err)
	}
}
