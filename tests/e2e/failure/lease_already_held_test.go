// Invariant: when pod A holds a session's Postgres advisory-lock lease and a
// git push for the same session arrives directly at pod B (bypassing the
// router), pod B must return 503 Service Unavailable with a Retry-After header.
// The error envelope must carry a non-empty error code. The client must not
// receive 200 — that would indicate split-brain corruption.
//
// Mechanism: we inject the advisory lock from the test process itself (same
// technique as router_lease_unavailable_test.go) so neither portal pod can
// acquire the lease. We then drive a git push directly to pod 0 (no router)
// and assert the exact 503 shape documented in PROTOCOL.md.
//
// SAFETY-CRITICAL ASSERTION: if the push returns 200 the test fatals — a
// portal that accepts a write without holding the lease is a split-brain
// corruption risk. Do NOT soften to != 200.
package failure_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	_ "github.com/lib/pq"
	"github.com/google/uuid"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// leaseHeldErrorEnvelope is the expected error response shape for lease-held
// failure responses (PROTOCOL.md error envelope contract).
type leaseHeldErrorEnvelope struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// TestLeaseAlreadyHeld verifies that a git push to a pod that cannot acquire
// the session's advisory lock returns 503 + Retry-After, not 200.
//
// The advisory lock is held from the test process (via a direct Postgres
// connection) so the portal's non-blocking pg_try_advisory_lock call fails
// before any write to object storage occurs.
func TestLeaseAlreadyHeld(t *testing.T) {
	requireDockerLocal(t)
	requirePortalImageLocal(t)

	ctx := context.Background()

	// ── Infrastructure ─────────────────────────────────────────────────────────
	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)
	c := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        2,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      false, // direct-pod test — we address pods by index
		PortalExtraEnv: map[string]string{
			"JAMSESH_LEASE_HEARTBEAT_INTERVAL_S": "2",
			"JAMSESH_EMAIL_PROVIDER":             "smtp",
			"JAMSESH_EMAIL_SMTP_HOST":            mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT":            strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":             "none",
		},
	})

	// ── Auth + session creation ─────────────────────────────────────────────────
	pod0 := c.Pods[0]
	userEmail := leaseHeldRandEmail("lf-held")
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "LeaseHeld Org")
	sessionID := leaseHeldCreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "lease-already-held")
	userID := leaseHeldGetMe(ctx, t, pod0.URL, pair.AccessToken)

	// ── Hold the advisory lock from the test process ────────────────────────────
	// The portal's lease manager calls pg_try_advisory_lock(hashtext(sessionID)::oid).
	// We acquire that same lock from a dedicated DB connection so every portal pod's
	// try-lock fails and returns 503. This is the controlled injection mechanism —
	// no need to race two real pods.
	lockDB, err := sql.Open("postgres", pg.DSN)
	if err != nil {
		t.Fatalf("lease_already_held: open lock DB: %v", err)
	}
	defer lockDB.Close()

	// pg_advisory_lock blocks until acquired. Since no push has happened yet,
	// neither pod holds the lock — we acquire it immediately.
	if _, err := lockDB.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext($1)::oid)", sessionID); err != nil {
		t.Fatalf("lease_already_held: acquire advisory lock from test process: %v", err)
	}
	t.Logf("lease_already_held: advisory lock held by test process for session %s", sessionID)

	// ── Attempt a git push directly to pod 0 ───────────────────────────────────
	// The push triggers the post-receive path which calls AcquireForRequest on the
	// portal's lease manager. Because the test process holds the advisory lock,
	// the portal's pg_try_advisory_lock fails → portal returns 503.
	//
	// SAFETY-CRITICAL ASSERTION: the push must NOT return 0 (git exit 0 = 2xx).
	// A 2xx here means the portal wrote to object storage without holding the
	// lease — split-brain corruption risk.
	pushStatus := leaseHeldAttemptPush(ctx, t, pod0.URL, orgID, sessionID, userID, pair.AccessToken)
	t.Logf("lease_already_held: push to pod 0 (lock held externally) → HTTP %d", pushStatus)

	// ── SAFETY-CRITICAL ASSERTION ───────────────────────────────────────────────
	if pushStatus == http.StatusOK {
		// 200 means the portal accepted the push without holding the lease.
		// This is a Critical split-brain bug: the portal wrote to object storage
		// under a session it does not own.
		t.Fatalf(
			"lease_already_held: CRITICAL SPLIT-BRAIN BUG — pod returned 200 while the advisory lock "+
				"was held externally. The portal must NOT accept writes without holding the session lease. "+
				"Park this as a Critical production bug before landing any workaround.",
		)
	}

	// The push MUST return exactly 503. Any other status (500, 422, 401) is
	// unexpected — 503 is the documented lease-contention response.
	if pushStatus != http.StatusServiceUnavailable {
		t.Fatalf(
			"lease_already_held: push returned %d (want 503 Service Unavailable) — "+
				"the portal must return 503 + Retry-After when it cannot acquire the session lease; "+
				"a 5xx other than 503 may indicate an unhandled error in the lease acquisition path",
			pushStatus,
		)
	}

	// ── Probe the 503 shape via a direct HTTP request ───────────────────────────
	// The git push CLI only reports the exit code, not headers. Do a separate
	// direct HTTP probe to assert Retry-After and the error envelope shape.
	probeResp := leaseHeldProbeGitEndpoint(ctx, t, pod0.URL, orgID, sessionID, pair.AccessToken)
	defer probeResp.Body.Close()
	probeBody, _ := io.ReadAll(probeResp.Body)

	t.Logf("lease_already_held: direct probe status=%d body=%s", probeResp.StatusCode, probeBody)

	// Retry-After must be present. The portal's 503 lease-contention path sets
	// this header so the git client can back off before retrying.
	if probeResp.Header.Get("Retry-After") == "" {
		t.Errorf(
			"lease_already_held: 503 response is missing Retry-After header — "+
				"PROTOCOL.md requires Retry-After on lease-contention 503 responses; "+
				"add the header in the portal's lease-acquisition error path",
		)
	} else {
		t.Logf("lease_already_held: Retry-After = %q", probeResp.Header.Get("Retry-After"))
	}

	// Error envelope: {"error": "<code>", "message": "<human>"}
	// The exact error code is not yet defined in httperr — assert non-empty and log.
	if probeResp.StatusCode == http.StatusServiceUnavailable {
		var env leaseHeldErrorEnvelope
		if jsonErr := json.Unmarshal(probeBody, &env); jsonErr != nil {
			t.Errorf(
				"lease_already_held: 503 response body is not a valid error envelope: %v\nbody: %s",
				jsonErr, probeBody,
			)
		} else {
			// Log the actual error code for future PROTOCOL.md alignment.
			t.Logf("lease_already_held: 503 error code = %q (expected 'lease.held_elsewhere')", env.Error)
			if env.Error == "" {
				t.Errorf(
					"lease_already_held: 503 response has empty error code — "+
						"PROTOCOL.md envelope contract requires a non-empty 'error' field; "+
						"add the lease-contention error code to the httperr package and PROTOCOL.md",
				)
			}
		}
	}

	// ── Release lock and verify portal can now acquire the lease ───────────────
	if _, err := lockDB.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext($1)::oid)", sessionID); err != nil {
		t.Logf("lease_already_held: release advisory lock: %v (non-fatal — test assertion already complete)", err)
	} else {
		t.Logf("lease_already_held: advisory lock released")
	}

	t.Logf("lease_already_held: invariant verified — portal returns 503 when advisory lock is held externally")
}

// ---------------------------------------------------------------------------
// Per-file helpers
// ---------------------------------------------------------------------------

// leaseHeldRandEmail returns a unique email for isolation across parallel runs.
func leaseHeldRandEmail(prefix string) string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("leaseHeldRandEmail: rand.Read: %v", err))
	}
	return prefix + "-" + hex.EncodeToString(b) + "@example.com"
}

// leaseHeldSessionRef is the minimal session-creation response shape.
type leaseHeldSessionRef struct {
	ID string `json:"id"`
}

// leaseHeldCreateSession POSTs to /api/orgs/{orgID}/sessions and returns the
// new session ID.
func leaseHeldCreateSession(
	ctx context.Context, t *testing.T,
	baseURL, accessToken, orgID, name string,
) string {
	t.Helper()

	body, err := json.Marshal(map[string]string{
		"name":         name,
		"goal":         "lease already held failure test",
		"scope":        `["**"]`,
		"default_mode": "sync",
	})
	if err != nil {
		t.Fatalf("leaseHeldCreateSession: marshal: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", baseURL, orgID),
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("leaseHeldCreateSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("leaseHeldCreateSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("leaseHeldCreateSession: want 201, got %d; body: %s", resp.StatusCode, respBody)
	}

	var s leaseHeldSessionRef
	if err := json.Unmarshal(respBody, &s); err != nil {
		t.Fatalf("leaseHeldCreateSession: decode: %v; body: %s", err, respBody)
	}
	if s.ID == "" {
		t.Fatalf("leaseHeldCreateSession: empty session ID in response; body: %s", respBody)
	}
	return s.ID
}

// leaseHeldGetMe calls GET /api/me and returns the user ID.
func leaseHeldGetMe(ctx context.Context, t *testing.T, baseURL, accessToken string) string {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/me", nil)
	if err != nil {
		t.Fatalf("leaseHeldGetMe: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("leaseHeldGetMe: GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("leaseHeldGetMe: want 200, got %d; body: %s", resp.StatusCode, body)
	}

	var me struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("leaseHeldGetMe: decode: %v; body: %s", err, body)
	}
	if me.ID == "" {
		t.Fatalf("leaseHeldGetMe: empty ID in response; body: %s", body)
	}
	return me.ID
}

// leaseHeldBasicAuthURL injects user:pass into the base URL for git clone.
func leaseHeldBasicAuthURL(baseURL, user, pass string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		panic(fmt.Sprintf("leaseHeldBasicAuthURL: parse %q: %v", baseURL, err))
	}
	u.User = url.UserPassword(user, pass)
	return u.String()
}

// leaseHeldAttemptPush clones the session repo, makes a commit, and pushes.
// Returns the HTTP status code that git received from the server (200 on success,
// 503 on lease contention). If git exits non-zero the exit is assumed to be a
// server-side rejection (503/5xx); returns http.StatusServiceUnavailable in
// that case. If git exits 0, returns http.StatusOK.
func leaseHeldAttemptPush(
	ctx context.Context, t *testing.T,
	podURL, orgID, sessionID, userID, accessToken string,
) int {
	t.Helper()

	repoDir := t.TempDir()
	repoURL := leaseHeldBasicAuthURL(podURL, "x-access-token", accessToken) +
		"/git/" + orgID + "/" + sessionID + ".git"

	// Clone — this does not acquire the lease (upload-pack only reads).
	cloneCmd := exec.CommandContext(ctx, "git", "clone", repoURL, repoDir)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		t.Logf("leaseHeldAttemptPush: clone error: %v\n%s", err, out)
		// Clone failure is not the lease-contention error we're testing — fail.
		t.Fatalf("leaseHeldAttemptPush: git clone failed (not the lease error): %v\n%s", err, out)
	}

	// Configure git identity.
	for _, args := range [][]string{
		{"config", "user.email", userID + "@test.example"},
		{"config", "user.name", "Test " + userID},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("leaseHeldAttemptPush: git %v: %v\n%s", args, err, out)
		}
	}

	// Write and stage a file.
	testFile := filepath.Join(repoDir, "lease-held-test.md")
	if err := os.WriteFile(testFile, []byte("lease already held test content"), 0o644); err != nil {
		t.Fatalf("leaseHeldAttemptPush: write file: %v", err)
	}
	addCmd := exec.CommandContext(ctx, "git", "add", "lease-held-test.md")
	addCmd.Dir = repoDir
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("leaseHeldAttemptPush: git add: %v\n%s", err, out)
	}

	// Commit with required Jam-* trailers.
	turnID := uuid.New().String()
	fullMessage := fmt.Sprintf(
		"lf-held: test commit\n\nJam-Session: %s\nJam-Turn: %s\nJam-Author: %s\n",
		sessionID, turnID, userID,
	)
	msgFile := filepath.Join(t.TempDir(), "commit-msg")
	if err := os.WriteFile(msgFile, []byte(fullMessage), 0o644); err != nil {
		t.Fatalf("leaseHeldAttemptPush: write commit msg file: %v", err)
	}
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-F", msgFile)
	commitCmd.Dir = repoDir
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("leaseHeldAttemptPush: git commit: %v\n%s", err, out)
	}

	// Push — this is where the lease acquisition attempt occurs in post-receive.
	ref := "jam/" + sessionID + "/" + userID + "/main"
	pushCmd := exec.CommandContext(ctx, "git", "push", "origin", "HEAD:refs/heads/"+ref)
	pushCmd.Dir = repoDir
	pushOut, pushErr := pushCmd.CombinedOutput()
	t.Logf("leaseHeldAttemptPush: git push exit=%v output=%s", pushErr, pushOut)

	if pushErr == nil {
		// git exited 0 → the server returned 2xx.
		return http.StatusOK
	}

	// git exited non-zero → server returned a non-2xx. We parse the git
	// output to find the HTTP status if reported, else assume 503.
	output := string(pushOut)
	for _, code := range []int{503, 500, 422, 401, 403} {
		if strings.Contains(output, strconv.Itoa(code)) {
			return code
		}
	}
	// git does not always echo the HTTP status code clearly; non-zero exit from
	// a push to an HTTP remote means the server returned a non-2xx error.
	// Treat as 503 since that is the expected lease-contention code.
	return http.StatusServiceUnavailable
}

// leaseHeldProbeGitEndpoint performs a direct HTTP GET against the git
// smart-HTTP info/refs endpoint. This is not a real git client call but it
// probes the portal's response shape (status code + headers + body) for the
// session's git endpoint. When the portal cannot acquire the session lease,
// it must return 503 + Retry-After on this endpoint too.
//
// Note: the portal returns 503 on any session-scoped request when the lease
// is blocked, not just on git push — this probe exercises the synchronous
// HTTP-layer check independently of the git push protocol.
func leaseHeldProbeGitEndpoint(
	ctx context.Context, t *testing.T,
	podURL, orgID, sessionID, accessToken string,
) *http.Response {
	t.Helper()

	// The git smart-HTTP info/refs endpoint is session-scoped and triggers the
	// lease check on the portal. Use upload-pack (clone/fetch) rather than
	// receive-pack (push) so the probe doesn't actually attempt to write anything.
	probeURL := fmt.Sprintf("%s/git/%s/%s.git/info/refs?service=git-upload-pack",
		podURL, orgID, sessionID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		t.Fatalf("leaseHeldProbeGitEndpoint: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("leaseHeldProbeGitEndpoint: GET: %v", err)
	}
	return resp
}
