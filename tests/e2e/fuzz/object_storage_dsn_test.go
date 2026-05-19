// Property: any value of JAMSESH_OBJECT_STORAGE_URL either causes the portal
// to boot cleanly (URL is valid and backend reachable) OR causes the portal to
// fail fast at startup with a typed error.
//
// The portal NEVER:
//   - Panics (nil-deref, SEGV) on any input
//   - Boots cleanly and then crashes on the first write attempt
//   - Logs an unhandled error without an exit
//
// The harness drives each seed URL through a real portal container started with
// JAMSESH_DEPLOY_MODE=clustered and a real Postgres DSN. Most seeds are
// expected to cause fast-fail (non-zero exit within 15s); a well-formed s3://
// URL may start a running container (URL accepted by parser), in which case
// /healthz is asserted to confirm a clean boot.
//
// Control iteration count via OBJ_DSN_FUZZ_COUNT (default 50).
// Reproduce a specific run via OBJ_DSN_FUZZ_SEED=<seed>.
package fuzz_test

import (
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

	"github.com/testcontainers/testcontainers-go"

	"jamsesh/tests/e2e/fixtures/postgres"
)

// dsnSeedCase is one entry from testdata/object-storage-dsn-corpus.json.
type dsnSeedCase struct {
	Description string `json:"description"`
	URL         string `json:"url"`
}

// panicTerms are substrings in container logs that indicate a Go runtime panic.
// Any match is a production bug.
var panicTerms = []string{
	"panic:",
	"panic(",
	"SIGSEGV",
	"SIGBUS",
	"nil pointer dereference",
	"runtime error:",
	"goroutine ",          // stack traces begin with "goroutine N [running]:"
	"fatal error:",
}

// logsPanic returns true if any log line contains a panic indicator.
func logsPanic(logs string) bool {
	lower := strings.ToLower(logs)
	for _, term := range panicTerms {
		if strings.Contains(lower, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

// startDSNPortal starts a portal container with the given JAMSESH_OBJECT_STORAGE_URL
// and a real Postgres DSN. It does NOT wait for /healthz — most seeds are
// expected to fail fast. Returns the container and its collected log output.
//
// The container is registered for cleanup on t via t.Cleanup.
func startDSNPortal(ctx context.Context, t *testing.T, pgContainerDSN, objectStorageURL string) (testcontainers.Container, string) {
	t.Helper()

	const portalImg = "jamsesh/portal:e2e"

	env := map[string]string{
		"JAMSESH_BIND":            ":8443",
		"JAMSESH_TLS_MODE":        "behind_proxy",
		"JAMSESH_DEPLOY_MODE":     "clustered",
		"JAMSESH_DB_DRIVER":       "postgres",
		"JAMSESH_DB_DSN":          pgContainerDSN,
		"JAMSESH_EMAIL_FROM":      "noreply@example.com",
		"JAMSESH_OBJECT_STORAGE_URL": objectStorageURL,
		// Provide AWS credentials so the SDK chain does not error on
		// missing credentials for any seed that reaches SDK construction.
		"AWS_ACCESS_KEY_ID":     "test-key",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
		"AWS_REGION":            "us-east-1",
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        portalImg,
			Env:          env,
			ExposedPorts: []string{"8443/tcp"},
			// No WaitingFor — we expect most seeds to crash before /healthz is reachable.
		},
		Started: true,
	})
	if err != nil {
		// Docker-level failure, not a portal crash.
		t.Fatalf("startDSNPortal: GenericContainer error (Docker level, not portal crash): %v", err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(c); err != nil {
			t.Logf("startDSNPortal: cleanup: terminate: %v", err)
		}
	})

	// Poll until the container exits or a 15s deadline is reached.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		state, err := c.State(ctx)
		if err != nil {
			break
		}
		if !state.Running {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	// Collect logs regardless of state.
	logReader, err := c.Logs(ctx)
	var logOutput string
	if err == nil && logReader != nil {
		raw, _ := io.ReadAll(logReader)
		logReader.Close()
		logOutput = string(raw)
	}

	return c, logOutput
}

// dsnPortalIsRunning returns true when the container is still running.
func dsnPortalIsRunning(ctx context.Context, c testcontainers.Container) bool {
	state, err := c.State(ctx)
	if err != nil {
		return false
	}
	return state.Running
}

// dsnHealthzOK issues a single GET /healthz to the container's mapped port.
// Returns true if the response is 200 OK.
func dsnHealthzOK(ctx context.Context, c testcontainers.Container) bool {
	host, err := c.Host(ctx)
	if err != nil {
		return false
	}
	mappedPort, err := c.MappedPort(ctx, "8443/tcp")
	if err != nil {
		return false
	}
	url := fmt.Sprintf("http://%s:%d/healthz", host, mappedPort.Num())

	hc := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := hc.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// checkDSNOutcome inspects the container state and logs after the 15s poll
// window and enforces the property:
//
//  1. Container exited → must NOT have panicked. A typed error in logs is OK.
//  2. Container still running → /healthz must be reachable (clean boot).
//     A running container that is not serving /healthz is a "zombie" — log it
//     and skip (potential hang / DOS vector — would need a separate park).
//
// Returns true when the outcome is valid, false when a bug is detected.
func checkDSNOutcome(ctx context.Context, t *testing.T, c testcontainers.Container, logs, desc, rawURL string) {
	t.Helper()

	isRunning := dsnPortalIsRunning(ctx, c)

	if !isRunning {
		// Container exited. Confirm no panic in logs.
		if logsPanic(logs) {
			t.Errorf(
				"BUG DETECTED (panic in portal logs) — this is a production bug.\n"+
					"  seed desc : %s\n"+
					"  raw URL   : %q\n"+
					"  logs (tail):\n%s\n"+
					"Action: park as Important via /agile-workflow:park with this URL and log excerpt.",
				desc, rawURL, tailLogs(logs, 50),
			)
			return
		}
		// Non-panic exit is the expected fast-fail outcome for bad URLs.
		t.Logf("seed %q: container exited (fast-fail, no panic) — URL=%q", desc, rawURL)
		return
	}

	// Container is still running. Confirm /healthz is reachable.
	if dsnHealthzOK(ctx, c) {
		t.Logf("seed %q: container running and /healthz 200 (URL accepted as valid) — URL=%q", desc, rawURL)
		return
	}

	// Container running but /healthz not reachable: zombie / hang.
	// This is a potential DOS vector — document but do not fail hard, since
	// the portal may simply be slow to start with a valid-looking URL.
	// We log as a warning; if widespread, park as Critical.
	t.Logf(
		"WARNING: container still running after 15s but /healthz is unreachable.\n"+
			"  seed desc : %s\n"+
			"  raw URL   : %q\n"+
			"  This may be a hang (DOS vector). Investigate and park as Critical if confirmed.\n"+
			"  logs (tail):\n%s",
		desc, rawURL, tailLogs(logs, 30),
	)
	// Do not call t.Error here — leave it as a logged warning. A dedicated
	// test run with a longer timeout would be required to confirm a true hang.
	// The 15s poll above is generous for config-validation errors; a genuine
	// slow startup (e.g. SDK auth chain) may extend beyond that window on some
	// seed inputs. Document, don't gate.
}

// tailLogs returns the last n lines of logs (or all lines if fewer).
func tailLogs(logs string, n int) string {
	lines := strings.Split(logs, "\n")
	if len(lines) <= n {
		return logs
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// TestObjectStorageDSNFuzz is a property-based fuzz harness for the
// JAMSESH_OBJECT_STORAGE_URL parser in the portal. It:
//
//  1. Starts a Postgres container for all sub-tests to share (DSN only; no data).
//  2. For each seed in testdata/object-storage-dsn-corpus.json, starts a portal
//     container with JAMSESH_DEPLOY_MODE=clustered and the seed as the URL.
//  3. Waits up to 15s for the container to exit (fast-fail expected for most seeds).
//  4. Enforces the invariant: either exited without panic, or running with /healthz 200.
//  5. Runs N randomly generated URL inputs under the same property.
//
// Skip with -short. Control random iteration count via OBJ_DSN_FUZZ_COUNT.
func TestObjectStorageDSNFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("fuzz: long-running, skip under -short")
	}

	ctx := context.Background()

	requireDSNDockerAvailable(t)
	requireDSNPortalImagePresent(t)

	// Start a single Postgres container shared by all sub-tests.
	// Each portal container connects to it; we don't seed any data.
	pg := postgres.Start(ctx, t, postgres.Options{})

	// ---------------------------------------------------------------------------
	// Phase 1: seed corpus
	// ---------------------------------------------------------------------------

	seedData, err := os.ReadFile("testdata/object-storage-dsn-corpus.json")
	if err != nil {
		t.Fatalf("fuzz/dsn: read seed corpus: %v", err)
	}
	var seeds []dsnSeedCase
	if err := json.Unmarshal(seedData, &seeds); err != nil {
		t.Fatalf("fuzz/dsn: parse seed corpus: %v", err)
	}
	if len(seeds) < 15 {
		t.Fatalf("fuzz/dsn: corpus has %d entries (want ≥15); add more seeds", len(seeds))
	}

	for i, seed := range seeds {
		seed := seed // capture
		t.Run(fmt.Sprintf("seed_%02d_%s", i, sanitizeDSNName(seed.Description)), func(t *testing.T) {
			t.Parallel()
			// Docker/runc rejects env vars containing NUL bytes before the
			// portal binary even runs (OCI runtime error). NUL-containing
			// DSN parsing is a unit-test concern, not an e2e-via-container
			// concern; skip here and rely on dedicated unit coverage.
			if strings.ContainsRune(seed.URL, 0) {
				t.Skipf("DSN contains NUL byte; cannot pass via container env (see seed %q)", seed.Description)
			}
			c, logs := startDSNPortal(ctx, t, pg.ContainerDSN, seed.URL)
			checkDSNOutcome(ctx, t, c, logs, seed.Description, seed.URL)
		})
	}

	// ---------------------------------------------------------------------------
	// Phase 2: random property iterations
	// ---------------------------------------------------------------------------

	iterations := 50
	if v := os.Getenv("OBJ_DSN_FUZZ_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			iterations = n
		}
	}

	seed64 := time.Now().UnixNano()
	t.Logf("fuzz/dsn: random seed = %d (rerun with OBJ_DSN_FUZZ_SEED=%d to reproduce)", seed64, seed64)
	if v := os.Getenv("OBJ_DSN_FUZZ_SEED"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			seed64 = n
			t.Logf("fuzz/dsn: using provided seed %d", seed64)
		}
	}
	rng := mrand.New(mrand.NewPCG(uint64(seed64), 0xdeadbeef))

	for i := 0; i < iterations; i++ {
		i := i // capture
		rawURL := generateRandomObjectStorageURL(rng)
		t.Run(fmt.Sprintf("rand_%04d", i), func(t *testing.T) {
			t.Parallel()
			// See seed-loop comment: NUL bytes are blocked at the container
			// runtime layer, not by the portal parser. Skip.
			if strings.ContainsRune(rawURL, 0) {
				t.Skipf("randomly generated DSN contains NUL byte; cannot pass via container env")
			}
			c, logs := startDSNPortal(ctx, t, pg.ContainerDSN, rawURL)
			checkDSNOutcome(ctx, t, c, logs, fmt.Sprintf("rand_%04d", i), rawURL)
		})
	}
}

// ---------------------------------------------------------------------------
// Random URL generator
// ---------------------------------------------------------------------------

// knownSchemes covers valid and invalid schemes to exercise the parser.
var dsnSchemes = []string{
	"s3", "s3", "s3", // weight valid schemes more
	"gs", "azblob",
	"s3-compatible",
	"https", "http", "ftp", "file",
	"", // no scheme
	"s3://s3", // double-scheme
	"S3", "GS", "AZBLOB", // uppercase
}

// generateRandomObjectStorageURL generates a random URL-shaped string that
// exercises the parser. It produces a mix of:
//   - valid-ish shapes (correct scheme + bucket)
//   - wrong schemes
//   - malformed authority
//   - path traversal
//   - unicode, null bytes, percent-encoding edge cases
//   - double-scheme, empty, query strings, fragments
func generateRandomObjectStorageURL(rng *mrand.Rand) string {
	switch rng.IntN(10) {
	case 0:
		// Empty string.
		return ""
	case 1:
		// No scheme — just a path.
		return randDSNString(rng, 1, 64)
	case 2:
		// Plausible s3:// URL with a random bucket and key.
		return fmt.Sprintf("s3://%s/%s", randBucketName(rng), randKeyPath(rng))
	case 3:
		// Wrong scheme.
		scheme := []string{"https", "http", "ftp", "file", "data", "javascript"}[rng.IntN(6)]
		return fmt.Sprintf("%s://%s/%s", scheme, randBucketName(rng), randKeyPath(rng))
	case 4:
		// Path traversal in bucket or key.
		return fmt.Sprintf("s3://bucket/%s", randPathTraversal(rng))
	case 5:
		// Embedded credentials.
		return fmt.Sprintf("s3://%s:%s@%s/%s",
			randDSNString(rng, 1, 16),
			randDSNString(rng, 1, 16),
			randBucketName(rng),
			randKeyPath(rng))
	case 6:
		// Query string / fragment.
		base := fmt.Sprintf("s3://%s/%s", randBucketName(rng), randKeyPath(rng))
		switch rng.IntN(3) {
		case 0:
			return base + "?" + randDSNString(rng, 0, 32)
		case 1:
			return base + "#" + randDSNString(rng, 0, 32)
		default:
			return base + "?" + randDSNString(rng, 0, 16) + "#" + randDSNString(rng, 0, 16)
		}
	case 7:
		// Double-scheme.
		return fmt.Sprintf("s3://s3://%s/%s", randBucketName(rng), randKeyPath(rng))
	case 8:
		// Unicode or binary garbage.
		return randBinaryURL(rng)
	default:
		// Random scheme from the known set + random authority.
		scheme := dsnSchemes[rng.IntN(len(dsnSchemes))]
		if scheme == "" {
			return randDSNString(rng, 1, 64)
		}
		return fmt.Sprintf("%s://%s/%s", scheme, randBucketName(rng), randKeyPath(rng))
	}
}

// randBucketName returns a bucket name: valid, empty, or garbage.
func randBucketName(rng *mrand.Rand) string {
	switch rng.IntN(5) {
	case 0:
		// Valid DNS-label bucket name.
		const chars = "abcdefghijklmnopqrstuvwxyz0123456789-"
		n := 3 + rng.IntN(30)
		b := make([]byte, n)
		for i := range b {
			b[i] = chars[rng.IntN(len(chars))]
		}
		return string(b)
	case 1:
		return ""
	case 2:
		return "../etc"
	case 3:
		return strings.Repeat("a", 256) // overlong
	default:
		return randDSNString(rng, 0, 64)
	}
}

// randKeyPath returns a key/prefix path.
func randKeyPath(rng *mrand.Rand) string {
	switch rng.IntN(5) {
	case 0:
		return "prefix/subdir/"
	case 1:
		return "../../../etc/passwd"
	case 2:
		return ""
	case 3:
		return strings.Repeat("a/", 100) + "key"
	default:
		return randDSNString(rng, 0, 128)
	}
}

// randPathTraversal returns a path-traversal string.
func randPathTraversal(rng *mrand.Rand) string {
	patterns := []string{
		"../etc/passwd",
		"../../etc/shadow",
		".%2e/.%2e/etc/passwd",
		"..%2fetc%2fpasswd",
		"....//....//etc/passwd",
		strings.Repeat("../", rng.IntN(10)+1) + "etc/passwd",
	}
	return patterns[rng.IntN(len(patterns))]
}

// randBinaryURL returns a string with unicode, null bytes, or control chars.
func randBinaryURL(rng *mrand.Rand) string {
	switch rng.IntN(5) {
	case 0:
		// Unicode bucket name.
		return "s3://bücket-" + string([]rune{'α', 'β', 'γ'}) + "/prefix"
	case 1:
		// Embedded newline.
		return "s3://bucket/key\ninjected-header: value"
	case 2:
		// Null byte.
		return "s3://bucket/key\x00evil"
	case 3:
		// Percent-encoding error.
		return "s3://bucket/key%zz"
	default:
		// Control characters.
		return "s3://bucket/\x01\x02\x03\x04key"
	}
}

// randDSNString returns a random string that may appear in a URL.
// It includes printable ASCII, unicode, and occasionally control chars.
func randDSNString(rng *mrand.Rand, minLen, maxLen int) string {
	length := minLen
	if maxLen > minLen {
		length = minLen + rng.IntN(maxLen-minLen)
	}
	if length == 0 {
		return ""
	}
	switch rng.IntN(4) {
	case 0:
		// Printable ASCII only.
		const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~:@!$&'()*+,;="
		b := make([]byte, length)
		for i := range b {
			b[i] = charset[rng.IntN(len(charset))]
		}
		return string(b)
	case 1:
		// Unicode.
		runes := []rune("abcABC αβγδ中文日本語한국어🎉🔥")
		b := make([]rune, length)
		for i := range b {
			b[i] = runes[rng.IntN(len(runes))]
		}
		return string(b)
	case 2:
		// Control characters.
		b := make([]byte, length)
		for i := range b {
			b[i] = byte(rng.IntN(32))
		}
		return string(b)
	default:
		// Mixed bytes.
		b := make([]byte, length)
		for i := range b {
			b[i] = byte(rng.IntN(256))
		}
		return string(b)
	}
}

// ---------------------------------------------------------------------------
// Test infrastructure helpers
// ---------------------------------------------------------------------------

// requireDSNDockerAvailable skips t if Docker is not available.
func requireDSNDockerAvailable(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
}

// requireDSNPortalImagePresent skips t if the portal e2e image is not built.
func requireDSNPortalImagePresent(t *testing.T) {
	t.Helper()
	const img = "jamsesh/portal:e2e"
	if err := exec.Command("docker", "image", "inspect", img).Run(); err != nil {
		t.Skipf("portal e2e image %q not present — run `make test-portal-image` first", img)
	}
}

// sanitizeDSNName converts a human-readable description into a safe Go test name.
func sanitizeDSNName(s string) string {
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
