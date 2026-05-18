// Invariant: when the portal cannot persist objects to object storage because
// the configured bucket does not exist, a git push fails with a non-2xx
// response — the portal must NOT silently accept the write and return success
// while the object is lost.
//
// Mechanism: MinIO is started normally (accessible), but the cluster is
// configured with JAMSESH_OBJECT_STORAGE_URL pointing at a bucket name that
// was never created. MinIO returns NoSuchBucket on any PutObject call.
//
// Two possible outcomes, both satisfying the RPO=0 invariant:
//
//   - Startup failure: the portal validates bucket existence at boot time and
//     exits non-zero. No push is attempted. The invariant is satisfied because
//     no write was silently accepted.
//
//   - Runtime failure: the portal boots and the first write attempt (on push)
//     returns a non-2xx error to the git client.
//
// Design-flaw escape hatch: if the push returns 2xx but the bucket is
// empty (silent acceptance), park as a Critical durability bug. See the test
// body for the skip+comment if that path is hit.
package failure_test

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"time"

	"github.com/google/uuid"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestObjectStorageWriteRejected verifies that a git push to a clustered
// portal fails loudly when the configured S3 bucket does not exist.
//
// Infrastructure layout:
//   - MinIO is reachable (the container starts fine).
//   - A real bucket is created by minio.Start (mn.BucketName).
//   - The cluster is pointed at a DIFFERENT bucket name that is never created.
//   - On PutObject, MinIO returns NoSuchBucket → the portal must surface this
//     as a non-2xx push error (or exit at startup).
func TestObjectStorageWriteRejected(t *testing.T) {
	requireDockerLocal(t)
	requirePortalImageLocal(t)

	ctx := context.Background()

	// ── Infrastructure ─────────────────────────────────────────────────────────

	// MinIO with a real bucket (the fixture pre-creates mn.BucketName).
	mn := minio.Start(ctx, t, minio.Options{})

	// The portal is configured to use a DIFFERENT bucket that was never created.
	// MinIO returns NoSuchBucket on PutObject when the bucket doesn't exist.
	nonExistentBucket := "does-not-exist-" + writeRejectedRandHex(4)
	objectStorageURL := "s3://" + nonExistentBucket + "/"

	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)

	// Attempt to start the cluster with the missing-bucket URL.
	// portalcluster.Start will t.Fatal if the pods do not boot — but the
	// portal may fail at startup (bucket validation) before /healthz responds,
	// causing Start to hang or fail. We handle both paths below.
	//
	// Strategy: we need to know if pods booted or exited. Build a raw
	// startFailingPortal-style approach for this cluster scenario: if the portal
	// exits at boot, containerIsRunning returns false and we assert startup failure.
	// If it boots (AWS S3 is lazy — bucket existence not checked at startup),
	// proceed to attempt a push.
	//
	// Because portalcluster.Start calls t.Fatal on startup failure (which would
	// abort the test with the wrong message), we instead start one portal pod
	// manually with the missing-bucket env, mirroring the startFailingPortal
	// pattern but for clustered mode.
	clusteredEnv := map[string]string{
		"JAMSESH_BIND":     ":8443",
		"JAMSESH_TLS_MODE": "behind_proxy",

		"JAMSESH_DEPLOY_MODE": "clustered",

		// Missing-bucket URL — MinIO is reachable, bucket is not.
		"JAMSESH_OBJECT_STORAGE_URL":          objectStorageURL,
		"JAMSESH_OBJECT_STORAGE_ENDPOINT_URL": mn.ContainerEndpoint,
		"JAMSESH_OBJECT_STORAGE_PATH_STYLE":   "true",
		"JAMSESH_OBJECT_STORAGE_REGION":       "us-east-1",

		"AWS_ACCESS_KEY_ID":     mn.AccessKey,
		"AWS_SECRET_ACCESS_KEY": mn.SecretKey,

		// Postgres — required for clustered mode.
		"JAMSESH_DB_DRIVER": "postgres",
		"JAMSESH_DB_DSN":    pg.ContainerDSN,

		// Email — required by the portal's config validation.
		"JAMSESH_EMAIL_FROM":      "noreply@example.com",
		"JAMSESH_EMAIL_PROVIDER":  "smtp",
		"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
		"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
		"JAMSESH_EMAIL_SMTP_TLS":  "none",
	}

	// Start one portal pod without a health-check wait — the portal may exit
	// at boot if it validates bucket existence, or may boot lazily (AWS SDK).
	// startFailingPortal registers t.Cleanup and waits up to 15s for exit.
	portalContainer, logs := startFailingPortal(ctx, t, clusteredEnv)

	if !containerIsRunning(ctx, portalContainer) {
		// ── PATH A: startup failure ─────────────────────────────────────────────
		// The portal validated bucket existence at boot and exited non-zero.
		// This IS the loud failure the invariant requires.
		t.Logf("write_rejected: portal exited at startup (bucket validation at boot) — invariant satisfied")
		t.Logf("write_rejected: startup logs:\n%s", logs)

		state, err := portalContainer.State(ctx)
		if err != nil {
			t.Fatalf("write_rejected: inspect container state after startup exit: %v", err)
		}
		if state.ExitCode == 0 {
			t.Errorf("write_rejected: portal exited 0 on missing bucket (want non-zero)")
		}

		// Bucket must be empty: no objects were written before the portal exited.
		writeRejectedAssertBucketEmpty(ctx, t, mn, "sessions/")

		return
	}

	// ── PATH B: portal booted (lazy AWS SDK — bucket not probed at startup) ────
	// The AWS S3 client does not check bucket existence at construction time.
	// The portal is running. Attempt a real git push; the first write to S3
	// must fail with NoSuchBucket and surface as a non-2xx response.
	t.Logf("write_rejected: portal booted (AWS S3 client is lazy — bucket existence deferred to first write)")

	// Get the host-side URL of the running pod so we can talk to it.
	host, err := portalContainer.Host(ctx)
	if err != nil {
		t.Fatalf("write_rejected: get container host: %v", err)
	}
	mappedPort, err := portalContainer.MappedPort(ctx, "8443/tcp")
	if err != nil {
		t.Fatalf("write_rejected: get mapped port: %v", err)
	}
	podURL := fmt.Sprintf("http://%s:%d", host, mappedPort.Num())

	// Wait for /healthz — the portal is running, it should answer quickly.
	writeRejectedWaitHealthz(ctx, t, podURL, 30*time.Second)

	// Auth + onboarding: sign in, create org, create session.
	userEmail := "write-rejected-" + writeRejectedRandHex(4) + "@example.com"

	// Use authflow.RequestMagicLink and authflow.ExtractMagicLinkToken but
	// against our raw pod URL (not a portal.Portal struct — we have a raw container).
	// Build a minimal portal.Portal-like target using the raw URL.
	pair := writeRejectedSignIn(ctx, t, podURL, mh, userEmail)
	userID := writeRejectedGetUserID(ctx, t, podURL, pair.AccessToken)
	orgID := writeRejectedCreateOrg(ctx, t, podURL, pair.AccessToken, "write-rejected-org")
	sessionID := writeRejectedCreateSession(ctx, t, podURL, pair.AccessToken, orgID)

	// Clone the repository. The clone itself does not write to object storage
	// (it is a read operation — git clone runs upload-pack on the server).
	// The push is the first write operation.
	repoDir := t.TempDir()
	repoURL := writeRejectedBasicAuthURL(podURL, "x-access-token", pair.AccessToken) +
		"/git/" + orgID + "/" + sessionID + ".git"

	writeRejectedGitRun(ctx, t, "", "git", "clone", repoURL, repoDir)
	writeRejectedGitRun(ctx, t, repoDir, "git", "config", "user.email", userEmail)
	writeRejectedGitRun(ctx, t, repoDir, "git", "config", "user.name", "Write Rejected Test")

	// Write a file and commit it.
	testFile := filepath.Join(repoDir, "rejected.md")
	if err := os.WriteFile(testFile, []byte("object storage write rejection test"), 0o644); err != nil {
		t.Fatalf("write_rejected: write test file: %v", err)
	}
	writeRejectedGitRun(ctx, t, repoDir, "git", "add", "rejected.md")

	turnID := uuid.New().String()
	commitMsg := fmt.Sprintf("write-rejected: test commit\n\nJam-Session: %s\nJam-Turn: %s\nJam-Author: %s\n",
		sessionID, turnID, userID)
	msgFile := filepath.Join(t.TempDir(), "commit-msg")
	if err := os.WriteFile(msgFile, []byte(commitMsg), 0o644); err != nil {
		t.Fatalf("write_rejected: write commit message file: %v", err)
	}
	writeRejectedGitRun(ctx, t, repoDir, "git", "commit", "-F", msgFile)

	// Push — this is where the S3 write happens. We expect it to FAIL.
	// Do NOT use gitclient.Push (it calls t.Fatal on non-zero exit). Instead
	// execute git push directly and capture the exit status.
	ref := "jam/" + sessionID + "/" + userID + "/main"
	pushCmd := exec.CommandContext(ctx, "git", "push", "origin", "HEAD:refs/heads/"+ref)
	pushCmd.Dir = repoDir
	pushOut, pushErr := pushCmd.CombinedOutput()

	t.Logf("write_rejected: push exit=%v output=%s", pushErr, pushOut)

	// ── Assert: no silent acceptance ──────────────────────────────────────────
	// If pushErr is nil, git exited 0 — which means the portal returned 2xx.
	// But the push cannot have reached the non-existent bucket, so the object
	// must be silently lost. This is an RPO=0 violation.
	//
	// First, verify the real bucket (which the portal can reach) has no objects
	// written by this test session. We check the prefix used for session objects.
	sessionPrefix := "sessions/" + sessionID + "/"
	keysInRealBucket, listErr := mn.ListObjects(ctx, sessionPrefix)
	if listErr != nil {
		t.Logf("write_rejected: ListObjects(%q) error: %v (bucket=%q)", sessionPrefix, listErr, mn.BucketName)
		// ListObjects on an unreachable bucket is not a portal bug — log and continue.
	} else if len(keysInRealBucket) > 0 {
		t.Logf("write_rejected: unexpected: objects in real bucket under %q: %v", sessionPrefix, keysInRealBucket)
	}

	if pushErr == nil {
		// Push succeeded (2xx) on a missing S3 bucket — this is an RPO=0
		// violation. The portal accepted the write but the object cannot have
		// been persisted to the non-existent bucket: the NoSuchBucket error
		// from PutObject was silently swallowed instead of surfaced to the
		// git client as a non-2xx response.
		//
		// The fix (object-storage-write-rejected-silent-acceptance) ensures
		// that EmitForUpdates errors are propagated as HTTP 500 before any
		// response bytes are committed. If this assertion fires, the fix has
		// regressed and must be investigated immediately — do NOT re-add a
		// t.Skip here.
		t.Fatalf(
			"write_rejected: git push exited 0 (2xx) with a missing S3 bucket — "+
				"RPO=0 violation: the portal silently accepted a write it could not "+
				"persist. The receive-pack handler must return a non-2xx response "+
				"when the object-storage write fails. "+
				"Push output: %s", pushOut,
		)
	}

	// Push failed (non-zero exit) — the invariant is satisfied.
	// The portal did NOT silently accept the write.
	t.Logf("write_rejected: push failed as expected (non-2xx from portal); invariant satisfied")

	// Assert the real MinIO bucket has no objects from this session — no partial
	// write leaked to the reachable bucket. (Objects could theoretically land if
	// the portal tried the real bucket as a fallback, which would be a different bug.)
	writeRejectedAssertBucketEmpty(ctx, t, mn, sessionPrefix)
}

// ---------------------------------------------------------------------------
// Local helpers
// ---------------------------------------------------------------------------

// writeRejectedRandHex returns n*2 hex characters of random data.
func writeRejectedRandHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("writeRejectedRandHex: rand.Read: %v", err))
	}
	return hex.EncodeToString(b)
}

// writeRejectedWaitHealthz polls GET /healthz until it returns 200 or timeout.
func writeRejectedWaitHealthz(ctx context.Context, t *testing.T, baseURL string, timeout time.Duration) {
	t.Helper()
	client := httpClientWithTimeout(5 * time.Second)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("write_rejected: portal /healthz did not return 200 within %v", timeout)
}

// writeRejectedSignIn performs a full magic-link sign-in flow against a raw
// base URL (without a portal.Portal struct).
func writeRejectedSignIn(ctx context.Context, t *testing.T, baseURL string, mh *mailhog.MailHog, email string) authflow.TokenPair {
	t.Helper()

	// POST /api/auth/magic-link/request
	reqBody, _ := json.Marshal(map[string]string{"email": email})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/auth/magic-link/request", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("writeRejectedSignIn: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClientWithTimeout(10 * time.Second).Do(req)
	if err != nil {
		t.Fatalf("writeRejectedSignIn: POST magic-link/request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("writeRejectedSignIn: POST magic-link/request: status %d (want 204)", resp.StatusCode)
	}

	// Extract token from MailHog.
	rawToken := authflow.ExtractMagicLinkToken(ctx, t, mh, email)

	// POST /api/auth/magic-link/exchange
	exchBody, _ := json.Marshal(map[string]string{"token": rawToken})
	req2, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/auth/magic-link/exchange", bytes.NewReader(exchBody))
	if err != nil {
		t.Fatalf("writeRejectedSignIn: build exchange request: %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := httpClientWithTimeout(10 * time.Second).Do(req2)
	if err != nil {
		t.Fatalf("writeRejectedSignIn: POST magic-link/exchange: %v", err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("writeRejectedSignIn: POST magic-link/exchange: status %d (want 200): %s", resp2.StatusCode, body2)
	}
	var pair authflow.TokenPair
	if err := json.Unmarshal(body2, &pair); err != nil {
		t.Fatalf("writeRejectedSignIn: decode token pair: %v\nbody: %s", err, body2)
	}
	if pair.AccessToken == "" {
		t.Fatalf("writeRejectedSignIn: empty access_token in exchange response: %s", body2)
	}
	return pair
}

// writeRejectedGetUserID calls GET /api/me and returns the authenticated user's ID.
func writeRejectedGetUserID(ctx context.Context, t *testing.T, baseURL, accessToken string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/me", nil)
	if err != nil {
		t.Fatalf("writeRejectedGetUserID: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := httpClientWithTimeout(10 * time.Second).Do(req)
	if err != nil {
		t.Fatalf("writeRejectedGetUserID: GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("writeRejectedGetUserID: GET /api/me: status %d: %s", resp.StatusCode, body)
	}
	var me struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("writeRejectedGetUserID: decode /api/me: %v\nbody: %s", err, body)
	}
	if me.ID == "" {
		t.Fatalf("writeRejectedGetUserID: empty id in /api/me response: %s", body)
	}
	return me.ID
}

// writeRejectedCreateOrg calls POST /api/orgs and returns the new org ID.
func writeRejectedCreateOrg(ctx context.Context, t *testing.T, baseURL, accessToken, name string) string {
	t.Helper()
	reqBody, _ := json.Marshal(map[string]string{"name": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/orgs", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("writeRejectedCreateOrg: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := httpClientWithTimeout(10 * time.Second).Do(req)
	if err != nil {
		t.Fatalf("writeRejectedCreateOrg: POST /api/orgs: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("writeRejectedCreateOrg: POST /api/orgs: status %d (want 201): %s", resp.StatusCode, body)
	}
	var org struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &org); err != nil {
		t.Fatalf("writeRejectedCreateOrg: decode response: %v\nbody: %s", err, body)
	}
	if org.ID == "" {
		t.Fatalf("writeRejectedCreateOrg: empty id in response: %s", body)
	}
	return org.ID
}

// writeRejectedCreateSession calls POST /api/orgs/{orgID}/sessions and returns
// the new session ID.
func writeRejectedCreateSession(ctx context.Context, t *testing.T, baseURL, accessToken, orgID string) string {
	t.Helper()
	reqBody, _ := json.Marshal(map[string]string{
		"name":         "write-rejected-test-session",
		"goal":         "test object storage write rejection",
		"scope":        `["**"]`,
		"default_mode": "sync",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", baseURL, orgID),
		bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("writeRejectedCreateSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := httpClientWithTimeout(10 * time.Second).Do(req)
	if err != nil {
		t.Fatalf("writeRejectedCreateSession: POST /api/orgs/%s/sessions: %v", orgID, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("writeRejectedCreateSession: POST sessions: status %d (want 201): %s", resp.StatusCode, body)
	}
	var sess struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &sess); err != nil {
		t.Fatalf("writeRejectedCreateSession: decode response: %v\nbody: %s", err, body)
	}
	if sess.ID == "" {
		t.Fatalf("writeRejectedCreateSession: empty id in response: %s", body)
	}
	return sess.ID
}

// writeRejectedAssertBucketEmpty asserts that mn.ListObjects returns no keys
// under the given prefix. A non-empty result indicates an object leaked to the
// reachable bucket despite the configured URL pointing elsewhere — a different bug.
func writeRejectedAssertBucketEmpty(ctx context.Context, t *testing.T, mn *minio.MinIO, prefix string) {
	t.Helper()
	keys, err := mn.ListObjects(ctx, prefix)
	if err != nil {
		// If the bucket doesn't exist (e.g. minio fixture bucket was not created
		// for some reason), that's not a portal bug — skip the assertion.
		t.Logf("write_rejected: assertBucketEmpty(%q): ListObjects error (not a portal bug): %v", prefix, err)
		return
	}
	if len(keys) > 0 {
		t.Errorf("write_rejected: expected zero objects in bucket %q under prefix %q after rejected write, got %d: %v",
			mn.BucketName, prefix, len(keys), keys)
	}
}

// writeRejectedBasicAuthURL injects user:pass into the base URL.
func writeRejectedBasicAuthURL(baseURL, user, pass string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		panic(fmt.Sprintf("writeRejectedBasicAuthURL: parse %q: %v", baseURL, err))
	}
	u.User = url.UserPassword(user, pass)
	return u.String()
}

// writeRejectedGitRun executes a git command in dir, failing the test on error.
// Pass dir="" to use the current working directory.
func writeRejectedGitRun(ctx context.Context, t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("writeRejectedGitRun: %s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}
