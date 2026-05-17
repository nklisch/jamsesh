package finalizecmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v3"

	"jamsesh/internal/api/openapi"
)

// setupFinalizeRunEnv stages the per-session state files finalize-run
// reads (org_id sidecar + token + portal URL via env) and returns the
// CLAUDE_PLUGIN_DATA root. Mirrors the join-time layout written by
// sessioncmd.writeSessionState.
func setupFinalizeRunEnv(t *testing.T, sessionID, orgID, portalURL string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
	t.Setenv("JAMSESH_PORTAL_URL", portalURL)

	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "org_id"), []byte(orgID), 0o600); err != nil {
		t.Fatalf("write org_id: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("test-bearer"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	return dir
}

// setupSourceRepoForFinalize builds a fresh git repo with a base
// commit on main and a feature branch carrying two commits. The
// returned (repo, base, f1, f2) tuple gives the test SHAs to bake
// into the plan response.
func setupSourceRepoForFinalize(t *testing.T) (string, string, string, string) {
	t.Helper()
	repo := gitInTempRepo(t)
	base := commit(t, repo, "a.txt", "a\n", "base")
	mustGitCwd(t, repo, "checkout", "-b", "feature")
	f1 := commit(t, repo, "b.txt", "b\n", "feat: add b")
	f2 := commit(t, repo, "c.txt", "c\n", "feat: add c")
	mustGitCwd(t, repo, "checkout", "main")
	// Set up a bare origin so preflight's `git ls-remote origin` succeeds.
	originPath := t.TempDir()
	mustGitCwd(t, originPath, "init", "--bare")
	mustGitCwd(t, repo, "remote", "add", "origin", originPath)
	mustGitCwd(t, repo, "push", "origin", "main")
	return repo, base, f1, f2
}

// portalMockForFinalize stands up an httptest server that serves both
// the plan and the fetch-token endpoints with the canned payload the
// test feeds in. The handler short-circuits on path prefix so we can
// reuse one server for both endpoints.
func portalMockForFinalize(t *testing.T, sessionID, orgID, baseSHA, f1SHA, f2SHA, remoteURL string) *httptest.Server {
	t.Helper()
	planPath := "/api/orgs/" + orgID + "/sessions/" + sessionID + "/finalize-plan"
	tokenPath := "/api/orgs/" + orgID + "/sessions/" + sessionID + "/finalize/fetch-token"

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, planPath):
			plan := openapi.PlanResponse{
				PlanId:       sessionID + ":lock1",
				Mode:         openapi.Preserve,
				TargetBranch: "ready",
				BaseSha:      baseSHA,
				Script:       "# script body",
				SelectedCommits: []openapi.PlanCommit{
					{Sha: f1SHA, Subject: "feat: add b", AuthorName: "A", AuthorEmail: "a@x"},
					{Sha: f2SHA, Subject: "feat: add c", AuthorName: "A", AuthorEmail: "a@x"},
				},
				FetchSource: openapi.FetchSource{
					Kind:           openapi.Https,
					RemoteUrl:      remoteURL,
					TokenExpiresAt: time.Now().Add(5 * time.Minute).UTC(),
				},
				LockStatus: openapi.LockStatus{
					LockId:   "lock1",
					IsCaller: true,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(plan)
		case strings.HasPrefix(r.URL.Path, tokenPath):
			resp := openapi.FetchTokenResponse{
				Token:     "ephem",
				RemoteUrl: remoteURL,
				ExpiresAt: time.Now().Add(5 * time.Minute).UTC(),
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Errorf("unexpected portal path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestFinalizeRun_HappyPath_RemoteRemovedAfterRun runs the full
// finalize-run flow against a real source repo + an httptest portal,
// then asserts that `git remote -v` has no `jamsesh` entry — i.e. the
// cleanup stack drained the temporary remote on clean exit.
//
// The "remote_url" we feed the portal mock points at the same source
// repo (path form) so `git fetch jamsesh` actually succeeds against
// real git. This proves the end-to-end add/fetch/remove cycle.
func TestFinalizeRun_HappyPath_RemoteRemovedAfterRun(t *testing.T) {
	sessionID := "sessint1"
	orgID := "orgint1"

	srcRepo, baseSHA, f1SHA, f2SHA := setupSourceRepoForFinalize(t)

	// Spin up a bare "source-of-truth" repo and point the jamsesh
	// remote at it. We fetch FROM it; the cherry-picks come from
	// commits already in srcRepo so this is mostly a no-op fetch.
	bareSrc := t.TempDir()
	mustGitCwd(t, bareSrc, "init", "--bare")
	mustGitCwd(t, srcRepo, "push", bareSrc, "feature:refs/heads/feature")

	srv := portalMockForFinalize(t, sessionID, orgID, baseSHA, f1SHA, f2SHA, bareSrc)
	defer srv.Close()

	setupFinalizeRunEnv(t, sessionID, orgID, srv.URL)
	pinGitToCwd(t, srcRepo)

	app := &cli.Command{
		Commands: []*cli.Command{FinalizeRunCommand()},
	}
	err := app.Run(context.Background(), []string{"jamsesh", "finalize-run", "--yes", sessionID + ":lock1"})
	if err != nil {
		t.Fatalf("finalize-run: %v", err)
	}

	// The cleanup stack must have removed the temporary jamsesh remote.
	listed := gitOutputCwd(t, srcRepo, "remote", "-v")
	if strings.Contains(listed, "jamsesh") {
		t.Errorf("jamsesh remote leaked after clean finalize-run:\n%s", listed)
	}
	// And the cherry-pick must have actually happened — ready branch
	// exists with 3 commits.
	log := gitOutputCwd(t, srcRepo, "log", "--format=%s", "ready")
	wantLines := []string{"feat: add c", "feat: add b", "base"}
	gotLines := strings.Split(strings.TrimSpace(log), "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("ready branch commit count: got %d (%v), want %d", len(gotLines), gotLines, len(wantLines))
	}
}

// TestFinalizeRun_SIGINTSimulated_RemoteRemovedAfterCancel verifies the
// SIGINT path: cancelling the root context mid-flight drains the cleanup
// stack and removes the temporary jamsesh remote even though the run
// errored out before completing the cherry-pick.
//
// We simulate SIGINT by stubbing `git fetch jamsesh` to cancel the
// context and return an error. By that point chooseFetchSource has
// already (a) added the jamsesh remote and (b) registered the
// fetch-source cleanup on the stack. The deferred cleanup.Run on the
// action's exit path then drains the cleanup with outcomeAborted; the
// watcher goroutine may also race in, but the stack's idempotency
// guard makes the second drain a no-op.
//
// The assertion is the SAME end-to-end invariant the story spec calls
// out: `git remote -v` shows no jamsesh entry after a SIGINT mid-run.
// We don't depend on the action returning a specific error message —
// the cancel-vs-stub race makes that non-deterministic — only on the
// cleanup outcome.
func TestFinalizeRun_SIGINTSimulated_RemoteRemovedAfterCancel(t *testing.T) {
	sessionID := "sessint2"
	orgID := "orgint2"

	srcRepo, baseSHA, f1SHA, f2SHA := setupSourceRepoForFinalize(t)

	bareSrc := t.TempDir()
	mustGitCwd(t, bareSrc, "init", "--bare")
	mustGitCwd(t, srcRepo, "push", bareSrc, "feature:refs/heads/feature")

	srv := portalMockForFinalize(t, sessionID, orgID, baseSHA, f1SHA, f2SHA, bareSrc)
	defer srv.Close()

	setupFinalizeRunEnv(t, sessionID, orgID, srv.URL)
	pinGitToCwd(t, srcRepo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Stub runGit so the FIRST `git fetch jamsesh` invocation simulates
	// the SIGINT-mid-fetch scenario:
	//   - cancels the root context (proxy for the OS delivering SIGINT
	//     to signal.NotifyContext);
	//   - briefly sleeps so the watcher goroutine has a chance to fire
	//     and try draining the stack BEFORE the main path returns from
	//     this stub (exercising the race the idempotency guard exists
	//     for);
	//   - returns a fetch error so performFetch propagates failure to
	//     the action and the deferred cleanup.Run drains the stack.
	oldRunGit := runGit
	t.Cleanup(func() { runGit = oldRunGit })
	fetchSeen := false
	runGit = func(args ...string) error {
		if !fetchSeen && len(args) >= 2 && args[0] == "fetch" && args[1] == "jamsesh" {
			fetchSeen = true
			cancel()
			time.Sleep(150 * time.Millisecond)
			return errFakeFetchAborted
		}
		return oldRunGit(args...)
	}

	app := &cli.Command{
		Commands: []*cli.Command{FinalizeRunCommand()},
	}
	err := app.Run(ctx, []string{"jamsesh", "finalize-run", "--yes", sessionID + ":lock1"})
	// Action MUST return a non-nil error — the fetch failed.
	if err == nil {
		t.Fatal("expected fetch failure to propagate, got nil")
	}

	// Give any lingering watcher goroutine a moment to complete its
	// drain so the assertion below isn't racy.
	time.Sleep(100 * time.Millisecond)

	listed := gitOutputCwd(t, srcRepo, "remote", "-v")
	if strings.Contains(listed, "jamsesh") {
		t.Errorf("jamsesh remote leaked after SIGINT simulation:\n%s", listed)
	}
	// Cherry-pick must NOT have happened — the run errored out before
	// the execute step.
	branches := gitOutputCwd(t, srcRepo, "branch")
	if strings.Contains(branches, "ready") {
		t.Errorf("ready branch was created despite fetch failure:\n%s", branches)
	}
}

// errFakeFetchAborted is the sentinel the SIGINT-simulation stub
// returns; declared at file scope so the test's stub closure can
// reference it without the linter griping about unused vars.
var errFakeFetchAborted = errFakeFetch("simulated SIGINT during fetch")

// errFakeFetch is a tiny named-string error type so the SIGINT
// simulation can return a distinct sentinel without depending on
// errors.New at package scope (which would be flagged by some lints
// when only used inside one test).
type errFakeFetch string

func (e errFakeFetch) Error() string { return string(e) }
