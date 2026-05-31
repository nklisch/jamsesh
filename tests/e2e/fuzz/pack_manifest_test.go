// Property: any bytes at sessions/<sessionID>/manifest.json in MinIO either:
//   - Parse as a valid objectstore.Manifest (version=1, session_id non-empty,
//     all PackEntry keys exist in bucket) and the portal proceeds normally, OR
//   - Cause the portal to fail fast with a typed error on first hydration.
//
// The portal NEVER:
//   - Panics or nil-derefs on manifest parse
//   - Silently truncates a corrupt manifest and proceeds as if it were valid
//   - Accepts a manifest with dangling pack references and silently produces an
//     inconsistent state
//
// Trigger mechanism: cold-start hydration. For each seed:
//  1. Write the seed bytes to MinIO under sessions/<sessionID>/manifest.json.
//  2. Start a 1-pod cluster that cold-starts with that bucket, triggering hydration.
//  3. Attempt a git push to the session.
//  4. Classify the outcome:
//     - push non-zero: correct (portal rejected or failed on corrupt manifest).
//       Assert: no panic in portal logs.
//     - push 2xx:
//       - For the control seed (valid manifest, no packs): expected and required.
//       - For invalid seeds: silent truncation — a production bug to park.
//  5. Assert: portal did not log "panic" or nil-pointer dereference.
//
// Skip with -short. Control iteration count via MANIFEST_FUZZ_COUNT.
// Reproduce a specific run via MANIFEST_FUZZ_SEED=<seed>.
package fuzz_test

import (
	"bytes"
	"context"
	crand "crypto/rand"
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

// manifestSeedCase is one entry from testdata/pack-manifest-corpus.json.
type manifestSeedCase struct {
	Description string `json:"description"`
	// Manifest holds the raw bytes to write as manifest.json in the bucket.
	// PLACEHOLDER is substituted with the real session ID at test time.
	Manifest string `json:"manifest"`

	// isControl marks the control seed (valid manifest, no packs).
	// A 2xx push with this seed must succeed; it verifies the harness itself works.
	isControl bool
}

// controlSeedDescription identifies the control seed in the corpus.
// This seed must produce a 2xx from git push; if it fails, the harness is broken.
const controlSeedDescription = "valid manifest no packs"

// validManifestSeedDescriptions lists corpus seeds that LOOK corrupt by name but
// actually decode to a legitimate, readable schema-version-1 manifest for the
// session — so a 2xx push is the CORRECT outcome, not silent truncation.
//
// In particular, Go's encoding/json marshals a nil slice/map as JSON `null`, so
// a manifest the portal itself writes with no packs and no refs serialises as
// `"packs":null,"refs":null`. A pre-seeded `"packs":null` is therefore
// indistinguishable from a valid empty manifest and must NOT be treated as
// corruption. These seeds are asserted to succeed, exactly like the control.
var validManifestSeedDescriptions = map[string]bool{
	"packs is null instead of array": true,
}

// manifestPanicTerms are substrings in container logs that indicate a Go runtime panic.
var manifestPanicTerms = []string{
	"panic:",
	"panic(",
	"SIGSEGV",
	"SIGBUS",
	"nil pointer dereference",
	"runtime error:",
	"fatal error:",
}

// manifestLogsPanic returns true if any log line contains a panic indicator.
func manifestLogsPanic(logs string) bool {
	lower := strings.ToLower(logs)
	for _, term := range manifestPanicTerms {
		if strings.Contains(lower, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

// sanitizeManifestName converts a human-readable description into a safe Go test name.
func sanitizeManifestName(s string) string {
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

// substituteManifestPlaceholder replaces all occurrences of PLACEHOLDER with
// the real sessionID in seed bytes.
func substituteManifestPlaceholder(raw, sessionID string) []byte {
	return []byte(strings.ReplaceAll(raw, "PLACEHOLDER", sessionID))
}

// requireManifestDockerAvailable skips t if Docker is not available.
func requireManifestDockerAvailable(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
}

// requireManifestPortalImage skips t if the portal e2e image is not built.
func requireManifestPortalImage(t *testing.T) {
	t.Helper()
	const img = "jamsesh/portal:e2e"
	if err := exec.Command("docker", "image", "inspect", img).Run(); err != nil {
		t.Skipf("portal e2e image %q not present — run `make test-portal-image` first", img)
	}
}

// manifestGetMe calls GET /api/me and returns the user ID.
func manifestGetMe(ctx context.Context, t *testing.T, baseURL, accessToken string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/me", nil)
	if err != nil {
		t.Fatalf("manifestGetMe: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("manifestGetMe: GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("manifestGetMe: want 200 got %d: %s", resp.StatusCode, body)
	}
	var me struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("manifestGetMe: decode: %v", err)
	}
	if me.ID == "" {
		t.Fatalf("manifestGetMe: empty user ID")
	}
	return me.ID
}

// manifestCreateSession calls POST /api/orgs/{orgID}/sessions and returns the new session ID.
func manifestCreateSession(ctx context.Context, t *testing.T, baseURL, accessToken, orgID, name string) string {
	t.Helper()
	body := map[string]string{
		"name":         name,
		"goal":         "Pack manifest fuzz test",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("manifestCreateSession: marshal: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", baseURL, orgID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("manifestCreateSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("manifestCreateSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("manifestCreateSession: want 201 got %d: %s", resp.StatusCode, respBody)
	}
	var s struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &s); err != nil {
		t.Fatalf("manifestCreateSession: decode: %v\nbody: %s", err, respBody)
	}
	if s.ID == "" {
		t.Fatalf("manifestCreateSession: empty ID in response")
	}
	return s.ID
}

// gitPushResult represents the outcome of a git push attempt.
type gitPushResult struct {
	// Success is true if git push returned exit code 0 (server returned 2xx).
	Success bool
	// Output is the combined stdout+stderr from git push.
	Output string
}

// tryGitPush attempts a git push and returns a gitPushResult without failing
// the test on non-zero exit. This allows the caller to inspect the outcome.
func tryGitPush(ctx context.Context, dir, ref string) gitPushResult {
	cmd := exec.CommandContext(ctx, "git", "push", "origin", "HEAD:refs/heads/"+ref)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return gitPushResult{
		Success: err == nil,
		Output:  string(out),
	}
}

// manifestKey returns the object-storage key for the given session's manifest.
func manifestKeyPath(sessionID string) string {
	return "sessions/" + sessionID + "/manifest.json"
}

// runManifestSeed exercises the fuzz property for one seed:
//  1. Writes the seed bytes to MinIO under sessions/<sessionID>/manifest.json.
//  2. Starts a 1-pod cluster (cold-start triggers hydration which reads the manifest).
//  3. Attempts a git push to the session.
//  4. Checks the outcome against the property invariant.
func runManifestSeed(
	ctx context.Context,
	t *testing.T,
	mn *minio.MinIO,
	pg *postgres.Postgres,
	mh *mailhog.MailHog,
	seed manifestSeedCase,
) {
	t.Helper()

	// Bound concurrent container cold-starts across all parallel seeds so the
	// Docker host is not saturated (see limiter_test.go). The slot is held until
	// this seed's containers are torn down, serialising boot+teardown I/O.
	acquireStartupSlot(t)

	// Step 1: create the session via the portal (using a short-lived bootstrap cluster).
	// We need a real session ID before we can write the manifest to the bucket.
	// Bootstrap with a 1-pod cluster just for session creation; we then tear it
	// down and create a fresh cluster that cold-starts with the seeded manifest.
	//
	// Note: portalcluster.Start registers t.Cleanup to tear down the cluster, so
	// the bootstrap cluster's containers are cleaned up when the subtest exits.
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

	userEmail := randEmail(t, "fuzz-manifest")
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	userID := manifestGetMe(ctx, t, pod0.URL, pair.AccessToken)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Fuzz Manifest Org")
	sessionID := manifestCreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "fuzz-manifest")

	// Step 2: inject the seed manifest into MinIO under the real session's key,
	// BEFORE the cold-start cluster reads it.
	seedBytes := substituteManifestPlaceholder(seed.Manifest, sessionID)
	mKey := manifestKeyPath(sessionID)

	if err := mn.PutObject(ctx, mKey, seedBytes); err != nil {
		t.Fatalf("runManifestSeed %q: PutObject(%q): %v", seed.Description, mKey, err)
	}
	t.Logf("seed %q: wrote %d bytes to %s", seed.Description, len(seedBytes), mKey)

	// Step 3: start a fresh 1-pod cluster. The cold-start will read the seeded
	// manifest from the bucket during hydration. We use a separate cluster so the
	// hydration path sees the pre-seeded manifest rather than a manifest written
	// by the portal itself.
	//
	// This cluster starts with JAMSESH_DEPLOY_MODE=clustered, which triggers the
	// manifest-based hydration path on first access to the session.
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

	// Step 4: attempt a git push to trigger manifest hydration on the hot pod.
	// The push URL uses the hot pod so it reads the seeded manifest.
	//
	// We construct the git working tree manually via os/exec rather than via
	// gitclient.Clone (which calls t.Fatal on errors) so that we can inspect
	// clone failures (e.g. if the portal refuses the session due to a corrupt
	// manifest on the initial clone that triggers hydration).
	cloneDir := t.TempDir()
	ref := "jam/" + sessionID + "/" + userID + "/main"

	// Credentials are the same token from the bootstrap session.
	cloneURL := manifestCloneURL(hotPod.URL, "x-access-token", pair.AccessToken, orgID, sessionID)

	// Clone — this may fail if the portal rejects the session at clone time.
	cloneOut, cloneErr := exec.CommandContext(ctx, "git", "clone", cloneURL, cloneDir).CombinedOutput()
	if cloneErr != nil {
		// Clone failed. This is a valid outcome for corrupt manifests (the portal
		// may refuse to serve the session). Check for panics in logs.
		logs, _ := hotPod.Logs(ctx)
		if manifestLogsPanic(logs) {
			t.Errorf(
				"BUG DETECTED (panic in portal logs on clone) — production bug.\n"+
					"  seed:  %q\n"+
					"  clone error: %v\n"+
					"  git output: %s\n"+
					"  logs (tail):\n%s\n"+
					"Action: park as Important via /agile-workflow:park.",
				seed.Description, cloneErr, cloneOut, tailLogs(logs, 50),
			)
			return
		}
		if seed.isControl {
			t.Errorf(
				"HARNESS BUG: control seed %q failed at clone — harness setup is broken.\n"+
					"  clone error: %v\n"+
					"  git output: %s\n"+
					"Fix the harness setup before running any other seeds.",
				seed.Description, cloneErr, cloneOut,
			)
			return
		}
		t.Logf("seed %q: clone rejected (no panic) — portal correctly refused corrupt session at clone time. clone output: %s",
			seed.Description, cloneOut)
		return
	}

	// Configure git identity inside the cloned repo.
	exec.CommandContext(ctx, "git", "config", "user.email", userID+"@test.example").Run() //nolint:errcheck
	exec.CommandContext(ctx, "git", "config", "user.name", "Test").Run()                  //nolint:errcheck
	gitConf := func(key, val string) {
		cmd := exec.CommandContext(ctx, "git", "config", key, val)
		cmd.Dir = cloneDir
		cmd.Run() //nolint:errcheck
	}
	gitConf("user.email", userID+"@test.example")
	gitConf("user.name", "FuzzManifest")

	// Create a commit with required Jam-* trailers.
	contentFile := cloneDir + "/fuzz-manifest-check.md"
	if err := os.WriteFile(contentFile, []byte("fuzz manifest test\n"), 0o644); err != nil {
		t.Fatalf("seed %q: write test file: %v", seed.Description, err)
	}

	addCmd := exec.CommandContext(ctx, "git", "add", "fuzz-manifest-check.md")
	addCmd.Dir = cloneDir
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("seed %q: git add: %v\n%s", seed.Description, err, out)
	}

	// Compose commit message with required jamsesh trailers.
	commitMsg := fmt.Sprintf(
		"fuzz: manifest check\n\nJam-Session: %s\nJam-Turn: %s\nJam-Author: %s\n",
		sessionID, newUUID(), userID,
	)
	msgFile := cloneDir + "/.commit-msg"
	if err := os.WriteFile(msgFile, []byte(commitMsg), 0o644); err != nil {
		t.Fatalf("seed %q: write commit msg: %v", seed.Description, err)
	}

	commitCmd := exec.CommandContext(ctx, "git", "commit", "-F", msgFile)
	commitCmd.Dir = cloneDir
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("seed %q: git commit: %v\n%s", seed.Description, err, out)
	}

	// Step 5: attempt the push. This triggers manifest hydration on the hot pod.
	pushResult := tryGitPush(ctx, cloneDir, ref)

	// Step 6: check portal logs for panics regardless of push outcome.
	logs, _ := hotPod.Logs(ctx)
	if manifestLogsPanic(logs) {
		t.Errorf(
			"BUG DETECTED (panic in portal logs) — production bug.\n"+
				"  seed: %q\n"+
				"  push success: %v\n"+
				"  push output: %s\n"+
				"  logs (tail):\n%s\n"+
				"Action: park as Important via /agile-workflow:park.",
			seed.Description, pushResult.Success, pushResult.Output, tailLogs(logs, 50),
		)
		return
	}

	// Step 7: classify the push outcome against the invariant.
	if seed.isControl {
		// The control seed (valid manifest, no packs) MUST produce a successful push.
		// If it fails, the harness setup is broken.
		if !pushResult.Success {
			t.Errorf(
				"HARNESS BUG: control seed %q must succeed (valid manifest + no packs).\n"+
					"  push output: %s\n"+
					"  logs (tail):\n%s\n"+
					"Fix the harness setup before running other seeds.",
				seed.Description, pushResult.Output, tailLogs(logs, 50),
			)
		} else {
			t.Logf("seed %q (control): push succeeded as expected", seed.Description)
		}
		return
	}

	// Some seeds look corrupt by name but actually decode to a valid, readable
	// manifest (e.g. "packs":null == no packs). For those, a successful push is
	// the CORRECT outcome — the portal read a legitimate manifest. Assert success
	// like the control rather than flagging silent truncation.
	if validManifestSeedDescriptions[seed.Description] {
		if !pushResult.Success {
			t.Errorf(
				"seed %q is a VALID manifest (decodes to a readable v1 manifest) and must push successfully, "+
					"but the push was rejected.\n  push output: %s\n  logs (tail):\n%s",
				seed.Description, pushResult.Output, tailLogs(logs, 50),
			)
		} else {
			t.Logf("seed %q: push succeeded as expected (valid manifest variant)", seed.Description)
		}
		return
	}

	// For invalid seeds: a successful push (2xx) is silent truncation — a production bug.
	// A failed push (non-2xx) is the correct outcome.
	if pushResult.Success {
		t.Errorf(
			"BUG DETECTED (silent truncation) — portal accepted a corrupt manifest and "+
				"returned 2xx from git push. This is a durability / correctness violation.\n"+
				"  seed: %q\n"+
				"  seed bytes (first 256): %q\n"+
				"  push output: %s\n"+
				"  logs (tail):\n%s\n"+
				"Action: park as Critical via /agile-workflow:park with this seed description and log excerpt.\n"+
				"Do NOT change the assertion to accept 2xx on clearly invalid seeds.",
			seed.Description, truncateBytes(seedBytes, 256),
			pushResult.Output, tailLogs(logs, 50),
		)
	} else {
		t.Logf("seed %q: push rejected (no panic) — portal correctly refused corrupt manifest. output: %s",
			seed.Description, pushResult.Output)
	}
}

// TestPackManifestFuzz is a property-based fuzz harness for the pack manifest
// reader in the portal. It:
//
//  1. Starts MinIO, Postgres, and MailHog containers shared by all sub-tests.
//  2. For each seed in testdata/pack-manifest-corpus.json:
//     a. Creates a session via a bootstrap portal cluster.
//     b. Injects the seed bytes directly into MinIO as manifest.json.
//     c. Starts a fresh 1-pod cluster (cold-start hydration reads the manifest).
//     d. Attempts a git push to trigger manifest use.
//     e. Enforces the invariant: no panic, no silent truncation.
//  3. Runs N randomly generated malformed manifests under the same property.
//
// Skip with -short. Control random count via MANIFEST_FUZZ_COUNT.
// Each seed uses a fresh cluster (cold-start) — this test is intentionally slow.
func TestPackManifestFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("fuzz: long-running, skip under -short")
	}

	ctx := context.Background()

	requireManifestDockerAvailable(t)
	requireManifestPortalImage(t)

	// Shared infrastructure: all seeds share one MinIO, Postgres, and MailHog.
	// Each seed gets its own cluster (cold-start) and its own session.
	mn := minio.Start(ctx, t, minio.Options{})
	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)

	// ---------------------------------------------------------------------------
	// Phase 1: seed corpus
	// ---------------------------------------------------------------------------

	seedData, err := os.ReadFile("testdata/pack-manifest-corpus.json")
	if err != nil {
		t.Fatalf("fuzz/manifest: read seed corpus: %v", err)
	}
	var seeds []manifestSeedCase
	if err := json.Unmarshal(seedData, &seeds); err != nil {
		t.Fatalf("fuzz/manifest: parse seed corpus: %v", err)
	}
	if len(seeds) < 10 {
		t.Fatalf("fuzz/manifest: corpus has %d entries (want ≥10); add more seeds", len(seeds))
	}

	// Mark the control seed.
	for i := range seeds {
		if seeds[i].Description == controlSeedDescription {
			seeds[i].isControl = true
		}
	}

	// The control seed must exist — without it, the harness has no ground-truth.
	hasControl := false
	for _, s := range seeds {
		if s.isControl {
			hasControl = true
			break
		}
	}
	if !hasControl {
		t.Fatalf("fuzz/manifest: corpus is missing the control seed %q — add it before running", controlSeedDescription)
	}

	// Run the control seed first (sequential) so we know the harness works
	// before exercising invalid seeds.
	var controlSeed manifestSeedCase
	var otherSeeds []manifestSeedCase
	for _, s := range seeds {
		if s.isControl {
			controlSeed = s
		} else {
			otherSeeds = append(otherSeeds, s)
		}
	}

	t.Run("seed_control_"+sanitizeManifestName(controlSeed.Description), func(t *testing.T) {
		runManifestSeed(ctx, t, mn, pg, mh, controlSeed)
	})

	// Run remaining seeds as parallel sub-tests. Each acquires its own cluster
	// so they do not share pod state; MinIO and Postgres are shared read/write
	// but each seed operates in a distinct session namespace.
	for i, seed := range otherSeeds {
		seed := seed // capture
		t.Run(fmt.Sprintf("seed_%02d_%s", i, sanitizeManifestName(seed.Description)), func(t *testing.T) {
			t.Parallel()
			runManifestSeed(ctx, t, mn, pg, mh, seed)
		})
	}

	// ---------------------------------------------------------------------------
	// Phase 2: random malformed manifest iterations
	// ---------------------------------------------------------------------------

	iterations := 5 // low default: each iteration is a full cold-start cycle
	if v := os.Getenv("MANIFEST_FUZZ_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			iterations = n
		}
	}

	seed64 := time.Now().UnixNano()
	t.Logf("fuzz/manifest: random seed = %d (rerun with MANIFEST_FUZZ_SEED=%d to reproduce)", seed64, seed64)
	if v := os.Getenv("MANIFEST_FUZZ_SEED"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			seed64 = n
			t.Logf("fuzz/manifest: using provided seed %d", seed64)
		}
	}
	rng := mrand.New(mrand.NewPCG(uint64(seed64), 0xdeadbeef))

	for i := 0; i < iterations; i++ {
		i := i // capture
		randomManifest := generateRandomManifest(rng)
		randSeed := manifestSeedCase{
			Description: fmt.Sprintf("rand_%04d", i),
			Manifest:    randomManifest,
			isControl:   false,
		}
		t.Run(fmt.Sprintf("rand_%04d", i), func(t *testing.T) {
			t.Parallel()
			runManifestSeed(ctx, t, mn, pg, mh, randSeed)
		})
	}
}

// ---------------------------------------------------------------------------
// Random manifest generator
// ---------------------------------------------------------------------------

// generateRandomManifest returns a random byte sequence intended to exercise
// the manifest parser: valid-ish shapes, wrong types, huge payloads, binary.
func generateRandomManifest(rng *mrand.Rand) string {
	switch rng.IntN(12) {
	case 0:
		// Empty.
		return ""
	case 1:
		// Plain "null".
		return "null"
	case 2:
		// Truncated mid-key.
		return `{"version":1,"session_id":"x","pac`
	case 3:
		// Wrong top-level type: JSON array.
		return `[1,2,3]`
	case 4:
		// Wrong top-level type: JSON string.
		return `"not-an-object"`
	case 5:
		// Oversize manifest — 5 MB of nested garbage.
		return generateOversizeManifest(rng)
	case 6:
		// Valid shape but version=0.
		return `{"version":0,"session_id":"PLACEHOLDER","packs":[],"refs":{}}`
	case 7:
		// Valid shape but session_id is a number.
		return `{"version":1,"session_id":12345,"packs":[],"refs":{}}`
	case 8:
		// Packs with entry missing sha.
		return `{"version":1,"session_id":"PLACEHOLDER","packs":[{"pack_key":"k","idx_key":"i"}],"refs":{}}`
	case 9:
		// Refs with non-string values.
		return `{"version":1,"session_id":"PLACEHOLDER","packs":[],"refs":{"HEAD":1234}}`
	case 10:
		// Binary garbage.
		b := make([]byte, 64+rng.IntN(512))
		for i := range b {
			b[i] = byte(rng.IntN(256))
		}
		return string(b)
	default:
		// Random valid-looking JSON fields in random order.
		return generateRandomJSONManifest(rng)
	}
}

// generateOversizeManifest returns a JSON document large enough to stress the
// portal's manifest size limit (targeting ~5 MB).
func generateOversizeManifest(rng *mrand.Rand) string {
	// Build a packs array with many entries to hit ~5 MB.
	var b strings.Builder
	b.WriteString(`{"version":1,"session_id":"PLACEHOLDER","packs":[`)
	target := 5 * 1024 * 1024
	entry := `{"pack_key":"sessions/PLACEHOLDER/packs/` + strings.Repeat("x", 60) + `.pack","idx_key":"sessions/PLACEHOLDER/packs/` + strings.Repeat("x", 60) + `.idx","sha":"` + strings.Repeat("a", 40) + `"},`
	for b.Len() < target {
		b.WriteString(entry)
	}
	// Remove trailing comma and close.
	s := b.String()
	if s[len(s)-1] == ',' {
		s = s[:len(s)-1]
	}
	s += `],"refs":{}}`
	return s
}

// generateRandomJSONManifest returns a JSON object with randomised fields.
func generateRandomJSONManifest(rng *mrand.Rand) string {
	fields := []string{
		fmt.Sprintf(`"version":%d`, rng.IntN(200)-50),
		fmt.Sprintf(`"session_id":%q`, randManifestString(rng, 0, 128)),
		`"packs":[]`,
		`"refs":{}`,
	}
	// Shuffle field order.
	for i := len(fields) - 1; i > 0; i-- {
		j := rng.IntN(i + 1)
		fields[i], fields[j] = fields[j], fields[i]
	}
	// Optionally omit some fields.
	keep := fields[:1+rng.IntN(len(fields))]
	return "{" + strings.Join(keep, ",") + "}"
}

// randManifestString returns a random string of length in [minLen, maxLen).
func randManifestString(rng *mrand.Rand, minLen, maxLen int) string {
	length := minLen
	if maxLen > minLen {
		length = minLen + rng.IntN(maxLen-minLen)
	}
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_/"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rng.IntN(len(charset))]
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

// manifestCloneURL builds the credentialed git clone URL for a session.
func manifestCloneURL(portalURL, user, pass, orgID, sessionID string) string {
	// Insert user:pass into the URL, e.g. http://user:pass@host:port/git/org/session.git
	u := portalURL
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(u, prefix) {
			u = prefix + user + ":" + pass + "@" + u[len(prefix):]
			break
		}
	}
	return u + "/git/" + orgID + "/" + sessionID + ".git"
}

// truncateBytes returns the first n bytes of b as a string (for logging).
func truncateBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + fmt.Sprintf("…[%d more bytes]", len(b)-n)
}

// newUUID returns a random UUID-shaped string sufficient for Jam-Turn trailers.
// Avoids importing github.com/google/uuid to keep the fuzz package lean.
func newUUID() string {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		// Fall back to time-based if crypto/rand is unavailable (should never happen).
		return fmt.Sprintf("%x-%x-%x-%x-%x",
			time.Now().UnixNano(), time.Now().UnixNano()>>32,
			time.Now().UnixNano()>>16, time.Now().UnixNano()>>8,
			time.Now().UnixNano())
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
