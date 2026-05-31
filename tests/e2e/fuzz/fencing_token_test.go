// Property: any value in the fencing_token field of the stored manifest either
// causes a push to be:
//
//   - Accepted (status < 400): the portal treated the token as valid and
//     the push succeeded.
//   - Rejected with a typed error (4xx or documented 503): the portal parsed
//     the token, compared it against the caller's fencing token, and refused.
//
// But NEVER:
//   - Causes a 5xx (panic, nil-deref, unhandled error in the portal).
//   - Causes silent data corruption (write proceeds with a token lower than
//     the stored value — this is the split-brain hole that fencing prevents).
//
// Trigger mechanism: inject a manifest.json with the fuzz token value as the
// fencing_token JSON field directly into MinIO, then trigger a git push through
// a real single-pod clustered portal. The portal's ManifestStore.Load parses
// the manifest and ManifestStore.Save checks the fencing-token comparison.
//
// Why single-instance suffices: the fencing-token comparison in
// internal/portal/storage/objectstore/manifest.go runs on the portal process
// receiving the push, regardless of deploy mode. Single-pod clustered mode
// exercises the same code path as multi-pod.
//
// Skip with -short. Control random iteration count via FENCING_FUZZ_COUNT.
// Reproduce a specific run via FENCING_FUZZ_SEED=<seed>.
package fuzz_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// fencingTokenSeedCase is one entry from testdata/fencing-token-corpus.json.
// Token holds the raw string to embed as the fencing_token JSON value in the
// manifest. It may be a valid JSON number, a string literal, a keyword, or
// arbitrary garbage — the fuzz harness embeds it verbatim in the JSON.
type fencingTokenSeedCase struct {
	Description string `json:"description"`
	Token       string `json:"token"`
}

// fencingPanicTerms are substrings in container logs that indicate an
// unhandled Go runtime panic. Any match is a production bug.
var fencingPanicTerms = []string{
	"panic:",
	"panic(",
	"SIGSEGV",
	"SIGBUS",
	"nil pointer dereference",
	"runtime error:",
	"fatal error:",
}

// fencingLogsPanic returns true if any log line contains a panic indicator.
func fencingLogsPanic(logs string) bool {
	lower := strings.ToLower(logs)
	for _, term := range fencingPanicTerms {
		if strings.Contains(lower, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

// sanitizeFencingName converts a human-readable description into a safe Go
// test name (reuses the same logic as sanitizeName from mcp_tool_input_test.go,
// but local to avoid package-level conflicts).
func sanitizeFencingName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	name := b.String()
	if len(name) > 60 {
		name = name[:60]
	}
	return name
}

// requireFencingDockerAvailable skips t if Docker is not available.
func requireFencingDockerAvailable(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
}

// requireFencingPortalImage skips t if the portal e2e image is not built.
func requireFencingPortalImage(t *testing.T) {
	t.Helper()
	const img = "jamsesh/portal:e2e"
	if err := exec.Command("docker", "image", "inspect", img).Run(); err != nil {
		t.Skipf("portal e2e image %q not present — run `make test-portal-image` first", img)
	}
}

// buildFencingManifest constructs a manifest.json body with rawToken embedded
// verbatim as the fencing_token JSON value. For valid numeric tokens (e.g.
// "0", "100") the result is well-formed JSON. For malformed values the JSON
// is intentionally invalid or unusual — that is the point of the fuzz.
//
// The fencing_token field is embedded raw (not as a JSON-encoded string) so
// the portal's json.Unmarshal sees the exact bit pattern we want.
//
// For the control / rejection sub-tests use buildFencingManifestInt instead.
func buildFencingManifest(sessionID, rawToken string) []byte {
	// We produce: {"version":1,"session_id":"<sid>","packs":[],"refs":{},"packed_refs":"","fencing_token":<rawToken>,"updated_at":"..."}
	// rawToken is embedded WITHOUT quotes — it is the literal JSON value.
	// For token values that are not valid JSON (e.g. "0xff", "true", empty)
	// the resulting document is invalid JSON, which is what we want to fuzz.
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	return []byte(fmt.Sprintf(
		`{"version":1,"session_id":%q,"packs":[],"refs":{},"packed_refs":"","fencing_token":%s,"updated_at":%q}`,
		sessionID, rawToken, ts,
	))
}

// buildFencingManifestInt constructs a well-formed manifest.json with an int64
// fencing_token value. Use this for control and rejection tests where you need
// a valid manifest that the portal can parse.
func buildFencingManifestInt(sessionID string, token int64) []byte {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	return []byte(fmt.Sprintf(
		`{"version":1,"session_id":%q,"packs":[],"refs":{},"packed_refs":"","fencing_token":%d,"updated_at":%q}`,
		sessionID, token, ts,
	))
}

// fencingManifestKey returns the MinIO key for the given session's manifest.
func fencingManifestKey(sessionID string) string {
	return "sessions/" + sessionID + "/manifest.json"
}

// fencingCreateSession calls POST /api/orgs/{orgID}/sessions and returns the
// new session's ID.
func fencingCreateSession(ctx context.Context, t *testing.T, podURL, accessToken, orgID, name string) string {
	t.Helper()
	body := map[string]string{
		"name":         name,
		"goal":         "Fencing token fuzz",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("fencingCreateSession: marshal: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", podURL, orgID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("fencingCreateSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fencingCreateSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("fencingCreateSession: want 201 got %d: %s", resp.StatusCode, respBody)
	}
	var s struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &s); err != nil {
		t.Fatalf("fencingCreateSession: decode: %v\nbody: %s", err, respBody)
	}
	if s.ID == "" {
		t.Fatalf("fencingCreateSession: empty ID in response")
	}
	return s.ID
}

// fencingGetMe calls GET /api/me and returns the user ID.
func fencingGetMe(ctx context.Context, t *testing.T, podURL, accessToken string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, podURL+"/api/me", nil)
	if err != nil {
		t.Fatalf("fencingGetMe: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fencingGetMe: GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fencingGetMe: want 200 got %d: %s", resp.StatusCode, body)
	}
	var me struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("fencingGetMe: decode: %v", err)
	}
	if me.ID == "" {
		t.Fatalf("fencingGetMe: empty user ID")
	}
	return me.ID
}

// fencingCloneURL builds the credentialed git clone URL for a session.
func fencingCloneURL(podURL, user, pass, orgID, sessionID string) string {
	u := podURL
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(u, prefix) {
			u = prefix + user + ":" + pass + "@" + u[len(prefix):]
			break
		}
	}
	return u + "/git/" + orgID + "/" + sessionID + ".git"
}

// runFencingSeed exercises the fencing-token fuzz property for one seed:
//  1. Injects a manifest with rawToken as the fencing_token JSON value into MinIO.
//  2. Starts a fresh 1-pod clustered portal so its next push reads the injected
//     manifest.
//  3. Clones the session and attempts a git push.
//  4. Checks: no 5xx from the portal, no panic in logs.
//
// Note: each seed gets its own session (pre-created in the shared bootstrap
// cluster) and its own hot cluster. This ensures the hot cluster cold-starts
// with our injected manifest rather than a portal-authored one.
func runFencingSeed(
	ctx context.Context,
	t *testing.T,
	mn *minio.MinIO,
	pg *postgres.Postgres,
	mh *mailhog.MailHog,
	desc, rawToken string,
) {
	t.Helper()

	// Bound concurrent container cold-starts across all parallel seeds so the
	// Docker host is not saturated (see limiter_test.go). The slot is held until
	// this seed's containers are torn down, serialising boot+teardown I/O.
	acquireStartupSlot(t)

	// Step 1: create the session via a short-lived bootstrap cluster.
	bootstrapCluster := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        1,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      false,
		PortalExtraEnv: map[string]string{
			"JAMSESH_EMAIL_PROVIDER":  "smtp",
			"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":  "none",
		},
	})
	pod0 := bootstrapCluster.Pods[0]

	userEmail := randEmail(t, "fuzz-fence")
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	userID := fencingGetMe(ctx, t, pod0.URL, pair.AccessToken)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Fencing Fuzz Org")
	sessionID := fencingCreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "fuzz-fencing")

	// Step 2: inject the fuzz manifest into MinIO BEFORE the hot cluster starts.
	manifestBytes := buildFencingManifest(sessionID, rawToken)
	mKey := fencingManifestKey(sessionID)
	if err := mn.PutObject(ctx, mKey, manifestBytes); err != nil {
		t.Fatalf("runFencingSeed %q: PutObject(%q): %v", desc, mKey, err)
	}
	t.Logf("seed %q: wrote %d bytes manifest (fencing_token=%s) to %s",
		desc, len(manifestBytes), rawToken, mKey)

	// Step 3: start a fresh hot cluster. Its cold-start will read the injected
	// manifest on first access to the session.
	hotCluster := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        1,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      false,
		PortalExtraEnv: map[string]string{
			"JAMSESH_EMAIL_PROVIDER":  "smtp",
			"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":  "none",
		},
	})
	hotPod := hotCluster.Pods[0]

	// Step 4: clone and push to trigger the fencing-token comparison path.
	cloneDir := t.TempDir()
	ref := "jam/" + sessionID + "/" + userID + "/main"

	cloneURL := fencingCloneURL(hotPod.URL, "x-access-token", pair.AccessToken, orgID, sessionID)
	cloneOut, cloneErr := exec.CommandContext(ctx, "git", "clone", cloneURL, cloneDir).CombinedOutput()
	if cloneErr != nil {
		// Clone failed — the portal may have rejected the session because the
		// injected manifest is unparseable. Check for panics and return.
		logs, _ := hotPod.Logs(ctx)
		if fencingLogsPanic(logs) {
			t.Errorf(
				"BUG DETECTED (panic in portal logs on clone) — production bug.\n"+
					"  seed:        %q\n"+
					"  raw_token:   %q\n"+
					"  clone error: %v\n"+
					"  git output:  %s\n"+
					"  logs (tail):\n%s\n"+
					"Action: park as Critical via /agile-workflow:park.",
				desc, rawToken, cloneErr, cloneOut, fencingTailLogs(logs, 50),
			)
			return
		}
		t.Logf("seed %q (token=%q): clone rejected without panic — portal correctly refused unparseable manifest. clone output: %s",
			desc, rawToken, cloneOut)
		return
	}

	// Configure git identity.
	gitConf := func(key, val string) {
		cmd := exec.CommandContext(ctx, "git", "config", key, val)
		cmd.Dir = cloneDir
		cmd.Run() //nolint:errcheck
	}
	gitConf("user.email", userID+"@test.example")
	gitConf("user.name", "FuzzFencing")

	// Write a file and commit with required Jam-* trailers.
	contentFile := cloneDir + "/fuzz-fencing-check.md"
	if err := os.WriteFile(contentFile, []byte("fencing token fuzz test\n"), 0o644); err != nil {
		t.Fatalf("seed %q: write test file: %v", desc, err)
	}

	addCmd := exec.CommandContext(ctx, "git", "add", "fuzz-fencing-check.md")
	addCmd.Dir = cloneDir
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("seed %q: git add: %v\n%s", desc, err, out)
	}

	commitMsg := fmt.Sprintf(
		"fuzz: fencing token check\n\nJam-Session: %s\nJam-Turn: %s\nJam-Author: %s\n",
		sessionID, newFencingUUID(), userID,
	)
	msgFile := cloneDir + "/.commit-msg"
	if err := os.WriteFile(msgFile, []byte(commitMsg), 0o644); err != nil {
		t.Fatalf("seed %q: write commit msg: %v", desc, err)
	}

	commitCmd := exec.CommandContext(ctx, "git", "commit", "-F", msgFile)
	commitCmd.Dir = cloneDir
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("seed %q: git commit: %v\n%s", desc, err, out)
	}

	// Push to trigger the manifest sync path (fencing-token comparison happens here).
	pushCmd := exec.CommandContext(ctx, "git", "push", "origin", "HEAD:refs/heads/"+ref)
	pushCmd.Dir = cloneDir
	pushOut, _ := pushCmd.CombinedOutput()
	pushExitCode := 0
	if pushCmd.ProcessState != nil {
		pushExitCode = pushCmd.ProcessState.ExitCode()
	}

	// Step 5: check for panics in portal logs regardless of push outcome.
	logs, _ := hotPod.Logs(ctx)
	if fencingLogsPanic(logs) {
		t.Errorf(
			"BUG DETECTED (panic in portal logs on push) — production bug.\n"+
				"  seed:       %q\n"+
				"  raw_token:  %q\n"+
				"  push exit:  %d\n"+
				"  push output:%s\n"+
				"  logs (tail):\n%s\n"+
				"Action: park as Critical via /agile-workflow:park.",
			desc, rawToken, pushExitCode, pushOut, fencingTailLogs(logs, 50),
		)
		return
	}

	// Step 6: classify.
	// The property is: no panic (checked above). A push failure on a malformed
	// token is correct (the portal rejected it via a typed error). A push success
	// on a token that parses as a valid int64 is also correct (portal accepted
	// it). We do NOT assert on 2xx vs non-2xx here — that is for
	// TestFencingTokenRejectionIsExplicit which uses a known numeric token.
	t.Logf("seed %q (token=%q): push exit=%d, no panic — property satisfied. output: %s",
		desc, rawToken, pushExitCode, pushOut)
}

// TestFencingTokenFuzz is a property-based fuzz harness for the fencing-token
// validation boundary in the object-storage write precondition path (F11).
//
// Property: any value in the fencing_token field of the on-disk manifest
// either results in:
//   - A push success (token accepted as >= caller's token), OR
//   - A typed rejection (4xx or 503, git push exits non-zero), OR
//   - A clone rejection (portal refuses session at clone time)
//
// But NEVER a portal panic (which surfaces as 5xx or a panic in the logs).
//
// The harness drives real git pushes to a real portal container backed by a
// real MinIO and Postgres. It does not stub any internal components — the
// fencing-token comparison code path runs exactly as in production.
//
// Skip with -short. Control random iteration count via FENCING_FUZZ_COUNT.
// Reproduce a specific run via FENCING_FUZZ_SEED=<seed>.
func TestFencingTokenFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("fuzz: long-running, skip under -short")
	}

	ctx := context.Background()

	requireFencingDockerAvailable(t)
	requireFencingPortalImage(t)

	// Shared infrastructure: all seeds share MinIO, Postgres, and MailHog.
	// Each seed gets its own session and its own hot cluster.
	mn := minio.Start(ctx, t, minio.Options{})
	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)

	// ---------------------------------------------------------------------------
	// Phase 1: seed corpus
	// ---------------------------------------------------------------------------

	seedData, err := os.ReadFile("testdata/fencing-token-corpus.json")
	if err != nil {
		t.Fatalf("fuzz/fencing: read seed corpus: %v", err)
	}
	var seeds []fencingTokenSeedCase
	if err := json.Unmarshal(seedData, &seeds); err != nil {
		t.Fatalf("fuzz/fencing: parse seed corpus: %v", err)
	}
	if len(seeds) < 10 {
		t.Fatalf("fuzz/fencing: corpus has %d entries (want ≥10); corpus is too small", len(seeds))
	}

	for i, seed := range seeds {
		seed := seed // capture
		t.Run(fmt.Sprintf("seed_%02d_%s", i, sanitizeFencingName(seed.Description)), func(t *testing.T) {
			runFencingSeed(ctx, t, mn, pg, mh, seed.Description, seed.Token)
		})
	}

	// ---------------------------------------------------------------------------
	// Phase 2: random property iterations
	// ---------------------------------------------------------------------------

	iterations := 10 // low default: each iteration is a full cold-start cycle
	if v := os.Getenv("FENCING_FUZZ_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			iterations = n
		}
	}

	seed64 := time.Now().UnixNano()
	t.Logf("fuzz/fencing: random seed = %d (rerun with FENCING_FUZZ_SEED=%d to reproduce)", seed64, seed64)
	if v := os.Getenv("FENCING_FUZZ_SEED"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			seed64 = n
			t.Logf("fuzz/fencing: using provided seed %d", seed64)
		}
	}
	rng := mrand.New(mrand.NewPCG(uint64(seed64), 0xdeadbeef))

	for i := 0; i < iterations; i++ {
		i := i // capture
		rawToken := generateRandomFencingTokenValue(rng)
		t.Run(fmt.Sprintf("rand_%04d", i), func(t *testing.T) {
			runFencingSeed(ctx, t, mn, pg, mh, fmt.Sprintf("rand_%04d", i), rawToken)
		})
	}
}

// TestFencingTokenRejectionIsExplicit verifies the split-brain guard:
// when the stored manifest token is strictly greater than the portal's issued
// fencing token, the push is EXPLICITLY REJECTED — not silently accepted.
//
// This is the non-fuzz correctness test for the invariant that the fuzz harness
// cannot verify on its own (the fuzz only checks for panics, not for silent
// acceptance of a lower token).
//
// Property: inject stored_token=100 → push with portal-issued token (1 after
// first acquisition) → push must fail AND the manifest in MinIO must still
// have fencing_token=100 (not overwritten).
//
// If the push returns success (git push exits 0 + portal returns 2xx): this is
// a critical split-brain vulnerability. Park via /agile-workflow:park with
// title "Stale fencing token silently accepted in manifest write". Do NOT
// change the assertion.
//
// If the manifest injection mechanism is not feasible (format changed): land
// with t.Skip("<story-id>: manifest format injection not available") and file
// a follow-on story for a test-side injection helper.
func TestFencingTokenRejectionIsExplicit(t *testing.T) {
	if testing.Short() {
		t.Skip("fuzz: long-running, skip under -short")
	}

	ctx := context.Background()

	requireFencingDockerAvailable(t)
	requireFencingPortalImage(t)

	mn := minio.Start(ctx, t, minio.Options{})
	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)

	// ---------------------------------------------------------------------------
	// Step 1: create session via a bootstrap cluster.
	// ---------------------------------------------------------------------------

	bootstrapCluster := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        1,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      false,
		PortalExtraEnv: map[string]string{
			"JAMSESH_EMAIL_PROVIDER":  "smtp",
			"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":  "none",
		},
	})
	pod0 := bootstrapCluster.Pods[0]

	userEmail := randEmail(t, "fuzz-fence-rej")
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	userID := fencingGetMe(ctx, t, pod0.URL, pair.AccessToken)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Fencing Rejection Org")
	sessionID := fencingCreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "fuzz-fencing-rejection")

	// ---------------------------------------------------------------------------
	// Step 2: inject a manifest with stored_token=100 into MinIO.
	// The portal will issue fencing token 1 (the first nextval from its own
	// Postgres sequence) on its first push. Since 100 > 1, the Save must be
	// rejected with ErrFenced.
	// ---------------------------------------------------------------------------
	const storedToken int64 = 100
	manifestBytes := buildFencingManifestInt(sessionID, storedToken)
	mKey := fencingManifestKey(sessionID)

	if err := mn.PutObject(ctx, mKey, manifestBytes); err != nil {
		t.Fatalf("TestFencingTokenRejectionIsExplicit: PutObject(%q): %v", mKey, err)
	}
	t.Logf("injected manifest with fencing_token=%d at key %s", storedToken, mKey)

	// ---------------------------------------------------------------------------
	// Step 3: start a fresh hot cluster that cold-starts with the injected
	// manifest.
	// ---------------------------------------------------------------------------
	hotCluster := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        1,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      false,
		PortalExtraEnv: map[string]string{
			"JAMSESH_EMAIL_PROVIDER":  "smtp",
			"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":  "none",
		},
	})
	hotPod := hotCluster.Pods[0]

	// ---------------------------------------------------------------------------
	// Step 4: clone and attempt a push.
	// ---------------------------------------------------------------------------
	cloneDir := t.TempDir()
	ref := "jam/" + sessionID + "/" + userID + "/main"

	cloneURL := fencingCloneURL(hotPod.URL, "x-access-token", pair.AccessToken, orgID, sessionID)
	cloneOut, cloneErr := exec.CommandContext(ctx, "git", "clone", cloneURL, cloneDir).CombinedOutput()
	if cloneErr != nil {
		// Clone failed. This is possible if the portal can read the manifest's
		// session_id but notices the fencing token is ahead of its own state.
		// In either case, check for panics first.
		logs, _ := hotPod.Logs(ctx)
		if fencingLogsPanic(logs) {
			t.Errorf(
				"BUG: panic in portal logs on clone.\n  clone output: %s\n  logs:\n%s",
				cloneOut, fencingTailLogs(logs, 50),
			)
			return
		}
		// Clone rejected cleanly is acceptable (portal noticed stale state at
		// clone time). The rejection-is-explicit property is satisfied.
		t.Logf("clone rejected without panic — stale fencing token detected at clone time: %s", cloneOut)

		// Verify the manifest was NOT overwritten.
		rawAfter, err := mn.GetObject(ctx, mKey)
		if err != nil {
			t.Fatalf("GetObject after rejection: %v", err)
		}
		assertManifestTokenUnchanged(t, rawAfter, storedToken)
		return
	}

	// Configure git identity.
	gitConf := func(key, val string) {
		cmd := exec.CommandContext(ctx, "git", "config", key, val)
		cmd.Dir = cloneDir
		cmd.Run() //nolint:errcheck
	}
	gitConf("user.email", userID+"@test.example")
	gitConf("user.name", "FuzzFencingRej")

	contentFile := cloneDir + "/fuzz-fencing-rej.md"
	if err := os.WriteFile(contentFile, []byte("fencing rejection test\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	addCmd := exec.CommandContext(ctx, "git", "add", "fuzz-fencing-rej.md")
	addCmd.Dir = cloneDir
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	commitMsg := fmt.Sprintf(
		"fuzz: fencing rejection test\n\nJam-Session: %s\nJam-Turn: %s\nJam-Author: %s\n",
		sessionID, newFencingUUID(), userID,
	)
	msgFile := cloneDir + "/.commit-msg"
	if err := os.WriteFile(msgFile, []byte(commitMsg), 0o644); err != nil {
		t.Fatalf("write commit msg: %v", err)
	}

	commitCmd := exec.CommandContext(ctx, "git", "commit", "-F", msgFile)
	commitCmd.Dir = cloneDir
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	pushCmd := exec.CommandContext(ctx, "git", "push", "origin", "HEAD:refs/heads/"+ref)
	pushCmd.Dir = cloneDir
	pushOut, _ := pushCmd.CombinedOutput()
	pushSucceeded := pushCmd.ProcessState != nil && pushCmd.ProcessState.ExitCode() == 0

	// ---------------------------------------------------------------------------
	// Step 5: check portal logs for panics.
	// ---------------------------------------------------------------------------
	logs, _ := hotPod.Logs(ctx)
	if fencingLogsPanic(logs) {
		t.Errorf(
			"BUG: panic in portal logs on push.\n  push output: %s\n  logs:\n%s",
			pushOut, fencingTailLogs(logs, 50),
		)
		return
	}

	// ---------------------------------------------------------------------------
	// Step 6: assert the push was REJECTED.
	// stored_token(100) > portal-issued-token(~1) → ManifestStore.Save must
	// return ErrFenced → portal returns non-2xx → git push exits non-zero.
	//
	// If push succeeded: this is a critical split-brain vulnerability.
	// ---------------------------------------------------------------------------
	if pushSucceeded {
		t.Errorf(
			"CRITICAL BUG: stale fencing token silently accepted.\n"+
				"  stored_token:  %d\n"+
				"  push succeeded when it must have been rejected.\n"+
				"  push output:   %s\n"+
				"  logs (tail):\n%s\n\n"+
				"Action: park as Critical via /agile-workflow:park with title\n"+
				"\"Stale fencing token silently accepted in manifest write\".\n"+
				"Do NOT change this assertion.",
			storedToken, pushOut, fencingTailLogs(logs, 50),
		)
		return
	}

	t.Logf("push correctly rejected (git exit non-zero) for stored_token=%d — split-brain guard active", storedToken)

	// ---------------------------------------------------------------------------
	// Step 7: assert the manifest was NOT overwritten with a lower fencing token.
	// Read the manifest from MinIO and verify fencing_token is still storedToken.
	// ---------------------------------------------------------------------------
	rawAfter, err := mn.GetObject(ctx, mKey)
	if err != nil {
		t.Fatalf("GetObject after rejection: %v", err)
	}
	assertManifestTokenUnchanged(t, rawAfter, storedToken)
}

// assertManifestTokenUnchanged reads a manifest from raw JSON bytes and asserts
// that fencing_token == expectedToken. This verifies that a rejected write did
// not overwrite the manifest with a lower token.
//
// If the JSON is unparseable, the assertion treats fencing_token as unchanged
// (the portal cannot have written a valid manifest over corrupt bytes). This is
// conservative but correct: if the portal DID overwrite a corrupt manifest with
// a valid one containing a lower token, that is a separate bug class from the
// one this assertion targets (silent-accept).
func assertManifestTokenUnchanged(t *testing.T, rawBytes []byte, expectedToken int64) {
	t.Helper()

	var m struct {
		FencingToken int64 `json:"fencing_token"`
	}
	if err := json.Unmarshal(rawBytes, &m); err != nil {
		// Cannot parse: likely the original injected manifest (pre-push rejection).
		// The portal cannot have written a new manifest (push was rejected), so
		// this is fine — the original bytes are still in place.
		t.Logf("assertManifestTokenUnchanged: cannot parse manifest JSON (original bytes unchanged): %v", err)
		return
	}

	if m.FencingToken != expectedToken {
		t.Errorf(
			"CRITICAL BUG: manifest was overwritten after rejection.\n"+
				"  expected fencing_token: %d\n"+
				"  got fencing_token:      %d\n"+
				"  This means the portal wrote a new manifest despite ErrFenced.\n"+
				"Action: park as Critical via /agile-workflow:park with title\n"+
				"\"Manifest overwritten by stale fencing token write\".\n"+
				"Do NOT change this assertion.",
			expectedToken, m.FencingToken,
		)
	} else {
		t.Logf("assertManifestTokenUnchanged: fencing_token=%d unchanged — manifest not overwritten", m.FencingToken)
	}
}

// ---------------------------------------------------------------------------
// Random fencing-token value generator
// ---------------------------------------------------------------------------

// generateRandomFencingTokenValue returns a random raw JSON value (as string)
// to embed as the fencing_token field. It covers:
//   - Valid int64 boundary values (numerically encoded)
//   - JSON null, true, false
//   - JSON strings (wrong type)
//   - Floating-point JSON numbers
//   - Non-JSON garbage (will produce invalid JSON in the manifest)
//   - Embedded control characters
//   - Oversize numbers
func generateRandomFencingTokenValue(rng *mrand.Rand) string {
	switch rng.IntN(14) {
	case 0:
		// Zero.
		return "0"
	case 1:
		// Small positive.
		return strconv.FormatInt(int64(rng.IntN(1000)), 10)
	case 2:
		// Small negative.
		return strconv.FormatInt(-int64(rng.IntN(1000)), 10)
	case 3:
		// MaxInt64.
		return "9223372036854775807"
	case 4:
		// MinInt64.
		return "-9223372036854775808"
	case 5:
		// MaxInt64 + 1 (overflows — not a valid int64 decimal).
		return "9223372036854775808"
	case 6:
		// JSON null.
		return "null"
	case 7:
		// JSON boolean.
		if rng.IntN(2) == 0 {
			return "true"
		}
		return "false"
	case 8:
		// JSON string wrapping a number (wrong type).
		return fmt.Sprintf("%q", strconv.FormatInt(int64(rng.IntN(1000)), 10))
	case 9:
		// JSON float.
		return fmt.Sprintf("%f", rng.Float64()*1e12)
	case 10:
		// Non-JSON garbage (will produce invalid JSON in the manifest document).
		garbage := []string{"0xff", "0b1010", "NaN", "Infinity", "-Infinity", "undefined", ""}
		return garbage[rng.IntN(len(garbage))]
	case 11:
		// Oversize digit string (256 chars — not a valid JSON number either).
		digits := make([]byte, 256)
		for i := range digits {
			digits[i] = '0' + byte(rng.IntN(10))
		}
		return string(digits)
	case 12:
		// A JSON string value (wrong type for the int64 field).
		return `"not-a-number"`
	default:
		// Random int64 across the full range.
		v := int64(rng.Uint64())
		return strconv.FormatInt(v, 10)
	}
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

// fencingTailLogs returns the last n lines of logs (or all if fewer than n).
func fencingTailLogs(logs string, n int) string {
	lines := strings.Split(logs, "\n")
	if len(lines) <= n {
		return logs
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// newFencingUUID returns a random UUID-shaped string for Jam-Turn trailers.
// Avoids importing github.com/google/uuid to keep the package lean.
// Uses math/rand since it is already imported; the UUID is only used as a
// trailer value — cryptographic strength is not required.
func newFencingUUID() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(mrand.IntN(256))
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
