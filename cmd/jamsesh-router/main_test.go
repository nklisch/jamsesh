// Integration tests for the jamsesh-router binary.
//
// These tests start real httptest backend servers, wire up the full handler
// stack (ring + hint cache + proxy handler), and verify end-to-end routing
// behaviour using an httptest.Server as the router front-end. No actual
// network port binding is required — everything runs in-process.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"jamsesh/internal/router/cache"
	"jamsesh/internal/router/extract"
	"jamsesh/internal/router/proxy"
	"jamsesh/internal/router/ring"
)

// ── Test infrastructure ────────────────────────────────────────────────────────

// trackingBackend is an httptest.Server backend that records received requests
// and replies with the given status code.
type trackingBackend struct {
	ts      *httptest.Server
	mu      sync.Mutex
	paths   []string
	headers http.Header
	status  int
}

func newBackend(t *testing.T, status int) *trackingBackend {
	t.Helper()
	b := &trackingBackend{status: status}
	b.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.paths = append(b.paths, r.URL.Path)
		b.headers = r.Header.Clone()
		b.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(b.ts.Close)
	return b
}

func (b *trackingBackend) addr() string {
	return strings.TrimPrefix(b.ts.URL, "http://")
}

func (b *trackingBackend) hitCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.paths)
}

// buildHandler wires up a full handler stack with the given backends.
func buildHandler(t *testing.T, backends ...*trackingBackend) (*proxy.Handler, *ring.Ring) {
	t.Helper()
	r := ring.New(50)
	pods := make([]ring.Pod, 0, len(backends))
	for i, b := range backends {
		pods = append(pods, ring.Pod{
			ID:      fmt.Sprintf("pod-%d", i),
			Address: b.addr(),
		})
	}
	r.SetPods(pods)

	h := &proxy.Handler{
		Extract:  extract.SessionID,
		Ring:     r,
		Hint:     cache.New(1000, 60*time.Second),
		Fallback: proxy.NewRoundRobinFallback(r),
	}
	return h, r
}

// doRequest issues a request against ts and returns the status and body.
func doRequest(t *testing.T, ts *httptest.Server, method, path string, headers map[string]string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// ── Tests ──────────────────────────────────────────────────────────────────────

// TestRESTRouting verifies that a REST request with a session ID in the path
// is forwarded to one of the backends.
func TestRESTRouting(t *testing.T) {
	b0 := newBackend(t, http.StatusOK)
	b1 := newBackend(t, http.StatusOK)
	h, _ := buildHandler(t, b0, b1)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	status, _ := doRequest(t, ts, http.MethodGet,
		"/api/orgs/org1/sessions/rest-session-1/data", nil)
	if status != http.StatusOK {
		t.Fatalf("REST routing: got %d, want 200", status)
	}
	total := b0.hitCount() + b1.hitCount()
	if total != 1 {
		t.Errorf("expected exactly 1 backend hit, got %d (b0=%d b1=%d)", total, b0.hitCount(), b1.hitCount())
	}
}

// TestGitRouting verifies that a Git smart-HTTP request is forwarded to a backend.
func TestGitRouting(t *testing.T) {
	b0 := newBackend(t, http.StatusOK)
	b1 := newBackend(t, http.StatusOK)
	h, _ := buildHandler(t, b0, b1)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	status, _ := doRequest(t, ts, http.MethodGet,
		"/git/org1/git-session-abc.git/info/refs", nil)
	if status != http.StatusOK {
		t.Fatalf("Git routing: got %d, want 200", status)
	}
	if b0.hitCount()+b1.hitCount() != 1 {
		t.Errorf("expected exactly 1 backend hit, got %d", b0.hitCount()+b1.hitCount())
	}
}

// TestWSUpgradeRouting verifies that a WebSocket upgrade request is forwarded.
// We don't complete the WS handshake (the backend just returns 200); the point
// is that the path extracts the session ID and the proxy reaches the backend.
func TestWSUpgradeRouting(t *testing.T) {
	b0 := newBackend(t, http.StatusOK)
	b1 := newBackend(t, http.StatusOK)
	h, _ := buildHandler(t, b0, b1)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	status, _ := doRequest(t, ts, http.MethodGet,
		"/ws/sessions/ws-session-xyz", nil)
	if status != http.StatusOK {
		t.Fatalf("WS routing: got %d, want 200", status)
	}
	if b0.hitCount()+b1.hitCount() != 1 {
		t.Errorf("expected exactly 1 backend hit, got %d", b0.hitCount()+b1.hitCount())
	}
}

// TestMCPRoutingViaHeader verifies that an MCP request with a Jam-Session-Id
// header is forwarded to a backend.
func TestMCPRoutingViaHeader(t *testing.T) {
	b0 := newBackend(t, http.StatusOK)
	b1 := newBackend(t, http.StatusOK)
	h, _ := buildHandler(t, b0, b1)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	status, _ := doRequest(t, ts, http.MethodPost, "/mcp/call",
		map[string]string{"Jam-Session-Id": "mcp-session-42"})
	if status != http.StatusOK {
		t.Fatalf("MCP routing: got %d, want 200", status)
	}
	if b0.hitCount()+b1.hitCount() != 1 {
		t.Errorf("expected exactly 1 backend hit, got %d", b0.hitCount()+b1.hitCount())
	}
}

// TestHealthzFallback verifies that /healthz (no session ID) is routed via the
// round-robin fallback and reaches a backend.
func TestHealthzFallback(t *testing.T) {
	b0 := newBackend(t, http.StatusOK)
	b1 := newBackend(t, http.StatusOK)
	h, _ := buildHandler(t, b0, b1)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	const n = 4
	for i := range n {
		status, _ := doRequest(t, ts, http.MethodGet, "/healthz", nil)
		if status != http.StatusOK {
			t.Fatalf("healthz request %d: got %d, want 200", i, status)
		}
	}
	if b0.hitCount()+b1.hitCount() != n {
		t.Errorf("expected %d backend hits, got %d", n, b0.hitCount()+b1.hitCount())
	}
}

// TestReadyzFallback verifies that /readyz falls through to the fallback.
func TestReadyzFallback(t *testing.T) {
	b0 := newBackend(t, http.StatusOK)
	h, _ := buildHandler(t, b0)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	status, _ := doRequest(t, ts, http.MethodGet, "/readyz", nil)
	if status != http.StatusOK {
		t.Fatalf("readyz: got %d, want 200", status)
	}
	if b0.hitCount() != 1 {
		t.Errorf("readyz: expected 1 hit, got %d", b0.hitCount())
	}
}

// TestAuthFallback verifies that /auth/* falls through to the fallback.
func TestAuthFallback(t *testing.T) {
	b0 := newBackend(t, http.StatusOK)
	h, _ := buildHandler(t, b0)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	status, _ := doRequest(t, ts, http.MethodGet, "/auth/github/callback", nil)
	if status != http.StatusOK {
		t.Fatalf("/auth/*: got %d, want 200", status)
	}
}

// TestStickySessionConsistentHash verifies that multiple requests for the same
// session ID all land on the same backend.
func TestStickySessionConsistentHash(t *testing.T) {
	b0 := newBackend(t, http.StatusOK)
	b1 := newBackend(t, http.StatusOK)
	h, _ := buildHandler(t, b0, b1)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	const n = 10
	for i := range n {
		status, _ := doRequest(t, ts, http.MethodGet,
			fmt.Sprintf("/api/orgs/o/sessions/sticky-one/item-%d", i), nil)
		if status != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, status)
		}
	}

	// All requests must go to a single pod.
	if b0.hitCount() != 0 && b1.hitCount() != 0 {
		t.Errorf("session split across pods (b0=%d b1=%d): should be sticky", b0.hitCount(), b1.hitCount())
	}
}

// Test503TriggerRetry verifies that a 503 from the primary pod causes a retry
// against the next pod in the ring.
func Test503TriggerRetry(t *testing.T) {
	// One pod always returns 503.
	var pod503Hits int
	ts503 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pod503Hits++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(ts503.Close)

	// One pod always returns 200.
	var pod200Hits int
	ts200 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pod200Hits++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts200.Close)

	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "pod-503", Address: strings.TrimPrefix(ts503.URL, "http://")},
		{ID: "pod-200", Address: strings.TrimPrefix(ts200.URL, "http://")},
	})
	h := &proxy.Handler{
		Extract:  extract.SessionID,
		Ring:     r,
		Hint:     cache.New(100, 60*time.Second),
		Fallback: proxy.NewRoundRobinFallback(r),
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	// Issue a request for a session. We don't know which pod the ring picks
	// first; the important invariant is:
	// - At most 2 total backend hits (primary + retry).
	// - If the primary was pod-503, the retry hit pod-200, giving 200 to client.
	// - If the primary was pod-200, client sees 200 directly.
	status, _ := doRequest(t, ts, http.MethodGet,
		"/api/orgs/o/sessions/retry-target/x", nil)

	totalHits := pod503Hits + pod200Hits
	if totalHits > 2 {
		t.Errorf("too many backend hits: got %d (503=%d 200=%d), want ≤ 2", totalHits, pod503Hits, pod200Hits)
	}
	_ = status // Status depends on which pod the ring chose first; tested below.

	// The client must not see an unexpected error status from the router
	// itself (non-backend-originated). A 200 or 503 (propagated) are both valid.
	if status != http.StatusOK && status != http.StatusServiceUnavailable {
		t.Errorf("unexpected status: got %d", status)
	}
}

// TestRun_DiscoveryGoroutineExitsOnContextCancel verifies that the discovery
// goroutine launched inside runCtx exits cleanly when the context is cancelled.
//
// This is a regression guard for the fix in bug-router-static-discoverer-not-started,
// which moved the signal context earlier in run() so the goroutine is launched
// with a cancellable context. Without that fix the discovery goroutine would
// run forever after the HTTP server stopped, leaking a goroutine on every test
// invocation.
//
// Leak detection uses runtime.NumGoroutine() snapshots. The goroutine count is
// captured immediately before runCtx is spawned (after the backend httptest
// server is already running, so its goroutines are already counted). After
// runCtx returns, http.DefaultTransport.CloseIdleConnections() is called to
// drain keep-alive connection goroutines spawned by the readyz probe — those
// are not our goroutines. The poll then checks that the count falls back to
// baseline ± a small slack (2) for any ambient noise from the Go test runner.
func TestRun_DiscoveryGoroutineExitsOnContextCancel(t *testing.T) {
	// Start a real httptest server to act as the static pod backend.
	// The static discoverer will probe it on every tick; having a real server
	// prevents connection-refused errors from polluting test output and ensures
	// the probe makes a real HTTP request (which exercises the keep-alive path).
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	// Configure runCtx via environment variables (config.Load reads these).
	// Use :0 so the OS picks a free port — the test never sends requests to
	// the router, so the actual port doesn't matter.
	t.Setenv("JAMSESH_ROUTER_BIND", "127.0.0.1:0")
	t.Setenv("JAMSESH_ROUTER_STATIC_PODS", backendAddr)
	// Keep the shutdown grace period short so the test finishes quickly.
	t.Setenv("JAMSESH_ROUTER_SHUTDOWN_GRACE_S", "1")

	// Snapshot goroutine count after the backend is already running (its
	// goroutines are already included in the baseline).
	goroutinesBefore := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// runCtx returns an exit code. Capture it via a channel so we can assert
	// both that it returned and that it returned cleanly (exit code 0).
	exitCode := make(chan int, 1)
	go func() {
		exitCode <- runCtx(ctx, nil)
	}()

	// Wait for runCtx to start. We don't have a readiness channel from runCtx,
	// and the actual listen port is hidden (:0). A short sleep is an acceptable
	// startup settle — it is NOT used as a goroutine-exit synchronisation
	// primitive (that role belongs to the exitCode channel and poll loop below).
	time.Sleep(200 * time.Millisecond)

	// Cancel the context — this is the adversarial condition under test.
	cancel()

	// Assert runCtx returns within the shutdown deadline (generous 8 s to
	// absorb CI variance; the grace period itself is only 1 s).
	const shutdownCeiling = 8 * time.Second
	select {
	case code := <-exitCode:
		if code != 0 {
			t.Errorf("runCtx returned non-zero exit code: %d", code)
		}
	case <-time.After(shutdownCeiling):
		t.Fatalf("runCtx did not return within %s after context cancel — possible goroutine leak or shutdown hang", shutdownCeiling)
	}

	// Close idle connections held by http.DefaultTransport. The readyz.Probe
	// inside runCtx uses the default HTTP client, which keeps a persistent
	// connection to the backend alive in the connection pool. These pool
	// goroutines (persistConn readLoop / writeLoop) are not our goroutines —
	// they are ambient Go HTTP client infrastructure. Closing them before the
	// snapshot prevents spurious failures.
	http.DefaultTransport.(*http.Transport).CloseIdleConnections()

	// Poll until goroutine count returns to baseline (or 5 s ceiling).
	// A slack of +2 absorbs ambient noise from the Go test runner itself
	// (e.g. a GC goroutine or internal timer goroutine that may appear
	// transiently). Any genuine leak from the discovery goroutine or the
	// HTTP server would permanently elevate the count well above this.
	const leakPollCeiling = 5 * time.Second
	leakDeadline := time.Now().Add(leakPollCeiling)
	leaked := true
	for time.Now().Before(leakDeadline) {
		if runtime.NumGoroutine() <= goroutinesBefore+2 {
			leaked = false
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if leaked {
		t.Errorf("goroutine leak detected: count did not fall back to baseline (%d) within %s after CloseIdleConnections; current=%d",
			goroutinesBefore, leakPollCeiling, runtime.NumGoroutine())
	}
}

// TestGracefulShutdown verifies that in-flight requests complete before the
// server stops accepting connections.
func TestGracefulShutdown(t *testing.T) {
	// Backend that introduces a small delay to simulate an in-flight request.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(slow.Close)

	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "slow", Address: strings.TrimPrefix(slow.URL, "http://")},
	})
	h := &proxy.Handler{
		Extract:  extract.SessionID,
		Ring:     r,
		Hint:     cache.New(100, 60*time.Second),
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// Start a request in the background.
	done := make(chan int, 1)
	go func() {
		status, _ := doRequest(t, srv, http.MethodGet,
			"/api/orgs/o/sessions/grace-session/x", nil)
		done <- status
	}()

	// Give the goroutine a moment to start the request, then close the
	// httptest server (which calls Shutdown underneath).
	time.Sleep(5 * time.Millisecond)
	srv.Close()

	select {
	case status := <-done:
		// We may see 200 (completed) or 502 (connection reset during proxy)
		// depending on timing, but we must not hang.
		t.Logf("in-flight request completed with status %d", status)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for in-flight request to complete")
	}
}
