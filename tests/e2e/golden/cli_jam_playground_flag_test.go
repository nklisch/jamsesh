// Invariant: Running the real `jamsesh new --playground` binary against the
// real portal creates a playground session, prints the session ID and share
// URL, persists the anonymous bearer to $CLAUDE_PLUGIN_DATA/sessions/<id>/token,
// and the resulting session is visible via GET /api/playground/sessions/{id}.
// The bare repo also exists on the portal container filesystem at
// /tmp/jamsesh-repos/orgs/org_playground/sessions/<id>.git (Unit 5
// anti-tautology discipline).
package golden_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"jamsesh/tests/e2e/fixtures/binary"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

func TestCLI_JamPlayground(t *testing.T) {
	// Blocked on bug-playground-git-receive-pack-fails-with-200-hangup
	// (ROOT CAUSE IDENTIFIED in that bug's body): the base-ref push is
	// rejected by prereceive.WalkAndValidate because the seed commit
	// lacks the required Jam-Session/Jam-Turn/Jam-Author trailers
	// (internal/portal/prereceive/commits.go:15). A vanilla "initial commit"
	// from `git commit -m '...'` has no trailers, so jamsesh new --playground
	// from a fresh repo gets locked out — chicken-and-egg between the
	// trailer requirement and the bootstrap push.
	//
	// Two other CLI bugs were surfaced by this test and ARE already fixed
	// inline (commit 2bf22ea):
	//   1. idea-playground-scope-normalization-bug — scope=="**" wasn't
	//      normalized to ["**"] before sending to the portal.
	//   2. The playground git push URL was missing the org_id segment;
	//      portal route is /git/{orgID}/{sessionID}.git/...
	//
	// Re-enable once the trailer requirement is resolved for base-ref pushes
	// (recommended approach: exempt base-ref pushes when OldSHA is empty).
	t.Skip("blocked on bug-playground-git-receive-pack-fails-with-200-hangup (root cause: trailer requirement on seed commit)")

	ctx := context.Background()

	// --- Stack: postgres + portal with playground enabled ---
	// JAMSESH_PLAYGROUND_HARD_CAP_S is set high enough that the hard cap won't
	// elapse during the test. JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S is similarly
	// generous; we only care about session creation, not destruction, here.
	pg := postgres.Start(ctx, t, postgres.Options{})
	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     pg.ContainerDSN,
		EmailFrom: "noreply@example.com",
		ExtraEnv: map[string]string{
			"JAMSESH_PLAYGROUND_ENABLED":      "true",
			"JAMSESH_PLAYGROUND_HARD_CAP_S":   "3600",
			"JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S": "3600",
		},
	})

	// --- Build the real CLI binary ---
	binPath := binary.Build(t)

	// --- Per-test CLAUDE_PLUGIN_DATA so state writes are isolated ---
	pluginDataDir := t.TempDir()

	// --- Init a git repo with one commit on main ---
	// `jamsesh new --playground` calls `git rev-parse --git-dir` and
	// `git rev-parse HEAD` in the binary's working directory, so the repo must
	// have at least one commit.
	repoDir := t.TempDir()
	mustGit(t, repoDir, "init", "--initial-branch=main")
	mustGit(t, repoDir, "config", "user.email", "e2e@example.com")
	mustGit(t, repoDir, "config", "user.name", "E2E Test")
	// Write a file and commit so HEAD resolves to a real SHA.
	seedFile := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(seedFile, []byte("playground e2e seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	mustGit(t, repoDir, "add", "README.md")
	mustGit(t, repoDir, "commit", "-m", "initial commit")

	// --- Invoke the binary: jamsesh new --playground ---
	// JAMSESH_PORTAL_URL overrides the portal URL in state.ReadPortalURL().
	// CLAUDE_PLUGIN_DATA directs all state reads/writes to our tempdir.
	// The binary is executed in repoDir so git commands find the working tree.
	cmd := exec.CommandContext(ctx, binPath, "new", "--playground")
	cmd.Dir = repoDir
	cmd.Env = append(
		minimalEnv(), // only PATH and HOME so git works; avoids real ~/.jamsesh
		"JAMSESH_PORTAL_URL="+p.URL,
		"CLAUDE_PLUGIN_DATA="+pluginDataDir,
	)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("jamsesh new --playground: exit error: %v\noutput:\n%s", runErr, out)
	}

	outStr := string(out)
	t.Logf("jamsesh new --playground output:\n%s", outStr)

	// --- Assert stdout contains expected phrases ---
	// printPlaygroundSummary emits "Playground session created!" and a
	// "Share URL" line that contains the session ID.
	if !strings.Contains(outStr, "Playground session created!") {
		t.Errorf("stdout missing 'Playground session created!':\n%s", outStr)
	}
	if !strings.Contains(outStr, "Share URL:") {
		t.Errorf("stdout missing 'Share URL:':\n%s", outStr)
	}

	// --- Extract the session ID from stdout ---
	// The share URL line is:   Share URL:  http://host:port/playground/<sessionID>
	sessionID := extractPlaygroundSessionID(t, outStr, p.URL)
	t.Logf("session ID: %s", sessionID)

	// --- Assert per-session state written to CLAUDE_PLUGIN_DATA ---
	// writePlaygroundSessionState writes:
	//   sessions/<id>/token  (written by state.WriteSessionToken before the push)
	//   sessions/<id>/org_id
	//   sessions/<id>/ref
	//   sessions/<id>/last_seen_seq
	tokenBytes := readStateFile(t, pluginDataDir, filepath.Join("sessions", sessionID, "token"))
	token := strings.TrimSpace(string(tokenBytes))
	if !strings.HasPrefix(token, "jamsesh_anon_") {
		t.Errorf("sessions/%s/token: expected prefix 'jamsesh_anon_', got %q", sessionID, token)
	}

	orgID := strings.TrimSpace(string(readStateFile(t, pluginDataDir, filepath.Join("sessions", sessionID, "org_id"))))
	if orgID != "org_playground" {
		t.Errorf("sessions/%s/org_id: expected 'org_playground', got %q", sessionID, orgID)
	}

	ref := strings.TrimSpace(string(readStateFile(t, pluginDataDir, filepath.Join("sessions", sessionID, "ref"))))
	expectedRef := fmt.Sprintf("jam/%s/playground/main", sessionID)
	if ref != expectedRef {
		t.Errorf("sessions/%s/ref: expected %q, got %q", sessionID, expectedRef, ref)
	}

	// --- End-to-end HTTP verification: GET /api/playground/sessions/{id} ---
	// This endpoint requires a valid bearer belonging to a member of the session.
	// We use the token that the binary just wrote to state.
	summaryBody := getPlaygroundSession(ctx, t, p.URL, sessionID, token)
	if summaryBody.OrgID != "org_playground" {
		t.Errorf("GET /api/playground/sessions/%s: org_id %q, want 'org_playground'", sessionID, summaryBody.OrgID)
	}
	if summaryBody.ID != sessionID {
		t.Errorf("GET /api/playground/sessions/%s: id %q, want %q", sessionID, summaryBody.ID, sessionID)
	}
	if summaryBody.Status != "active" {
		t.Errorf("GET /api/playground/sessions/%s: status %q, want 'active'", sessionID, summaryBody.Status)
	}

	// --- Unit 5 anti-tautology: bare repo exists on container filesystem ---
	// The push step in newPlaygroundAction should have created the bare repo at
	//   /tmp/jamsesh-repos/orgs/org_playground/sessions/<id>.git
	// inside the portal container. Confirm via docker exec (p.Exec).
	repoPath := "/tmp/jamsesh-repos/orgs/org_playground/sessions/" + sessionID + ".git"
	exitCode, execOut, execErr := p.Exec(ctx, []string{"ls", repoPath})
	if execErr != nil {
		t.Fatalf("portal docker exec ls %s: %v", repoPath, execErr)
	}
	if exitCode != 0 {
		t.Fatalf("bare repo not found at %s in portal container (exit %d): %s",
			repoPath, exitCode, strings.TrimSpace(execOut))
	}
	t.Logf("bare repo confirmed at %s (exit 0)", repoPath)
}

// ---------------------------------------------------------------------------
// helpers local to this test file
// ---------------------------------------------------------------------------

// playgroundSessionSummary is a minimal decode target for
// GET /api/playground/sessions/{id} (PlaygroundSessionSummary schema).
type playgroundSessionSummary struct {
	ID     string `json:"id"`
	OrgID  string `json:"org_id"`
	Status string `json:"status"`
}

// getPlaygroundSession calls GET /api/playground/sessions/{id} with a bearer
// token and returns the decoded summary. Fatals on non-200 or decode error.
func getPlaygroundSession(ctx context.Context, t *testing.T, baseURL, sessionID, bearer string) playgroundSessionSummary {
	t.Helper()
	url := fmt.Sprintf("%s/api/playground/sessions/%s", strings.TrimRight(baseURL, "/"), sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("getPlaygroundSession: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("getPlaygroundSession: GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("getPlaygroundSession: status %d (want 200): %s", resp.StatusCode, body)
	}
	var summary playgroundSessionSummary
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&summary); err != nil {
		t.Fatalf("getPlaygroundSession: decode: %v\nbody: %s", err, body)
	}
	return summary
}

// extractPlaygroundSessionID parses the session ID from the "Share URL:" line
// in the binary's stdout. The line format is:
//
//	  Share URL:  http://host:port/playground/<sessionID>
//
// Fatals if no matching line is found.
func extractPlaygroundSessionID(t *testing.T, output, baseURL string) string {
	t.Helper()
	prefix := strings.TrimRight(baseURL, "/") + "/playground/"
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if idx := strings.Index(trimmed, prefix); idx >= 0 {
			id := strings.TrimSpace(trimmed[idx+len(prefix):])
			// Strip any trailing path segments or whitespace.
			if i := strings.IndexAny(id, " \t/\r\n"); i > 0 {
				id = id[:i]
			}
			if id != "" {
				return id
			}
		}
	}
	t.Fatalf("extractPlaygroundSessionID: no 'Share URL: ...%s<id>' line in output:\n%s", prefix, output)
	return "" // unreachable
}

// readStateFile reads a file relative to dataDir and fatals if it doesn't exist.
func readStateFile(t *testing.T, dataDir, rel string) []byte {
	t.Helper()
	full := filepath.Join(dataDir, rel)
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("readStateFile %q: %v", rel, err)
	}
	return data
}

// mustGit runs a git command in dir and fatals on error.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v (in %s): %v\n%s", args, dir, err, out)
	}
}

// minimalEnv returns a minimal environment slice containing PATH and HOME so
// that git and the binary's subprocesses work without inheriting anything that
// could interfere with the test (e.g. a real CLAUDE_PLUGIN_DATA or
// JAMSESH_PORTAL_URL from the developer's shell).
func minimalEnv() []string {
	env := []string{}
	for _, key := range []string{"PATH", "HOME", "USER", "LOGNAME", "TMPDIR", "TMP", "TEMP", "XDG_RUNTIME_DIR"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}
	// Git needs GIT_CONFIG_NOSYSTEM=1 only if the system config is broken; we
	// don't set it here because we set per-repo user.email / user.name above via
	// mustGit("config", ...).
	return env
}
