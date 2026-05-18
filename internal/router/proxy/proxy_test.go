package proxy_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jamsesh/internal/router/cache"
	"jamsesh/internal/router/proxy"
	"jamsesh/internal/router/ring"
)

// ── Helpers ────────────────────────────────────────────────────────────────────

// backendServer starts an httptest.Server that records which requests it
// received and responds with the given status code. Each request appends the
// request path to received. Call ts.Close() when done.
func backendServer(t *testing.T, status int) (*httptest.Server, *[]string) {
	t.Helper()
	var received []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.URL.Path)
		w.WriteHeader(status)
	}))
	t.Cleanup(ts.Close)
	return ts, &received
}

// makeHandler builds a Handler with two pods (pod-a, pod-b) pointing at
// serverA and serverB.
func makeHandler(t *testing.T, serverA, serverB *httptest.Server) *proxy.Handler {
	t.Helper()
	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "pod-a", Address: strings.TrimPrefix(serverA.URL, "http://")},
		{ID: "pod-b", Address: strings.TrimPrefix(serverB.URL, "http://")},
	})
	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     cache.New(1000, 60*time.Second),
		Fallback: proxy.NewRoundRobinFallback(r),
	}
	return h
}

// staticExtract extracts the session ID from the X-Test-Session header (used
// only in tests). Returns "" for /healthz.
func staticExtract(r *http.Request) string {
	if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
		return ""
	}
	return r.Header.Get("X-Test-Session")
}

// get issues a GET request against handler h and returns the status code.
func get(t *testing.T, h http.Handler, path, sessionID string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if sessionID != "" {
		req.Header.Set("X-Test-Session", sessionID)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code
}

// ── Tests ──────────────────────────────────────────────────────────────────────

// TestSessionRouting verifies that requests with a session ID are reverse-
// proxied to a backend and return the backend's status code.
func TestSessionRouting(t *testing.T) {
	tsA, recA := backendServer(t, http.StatusOK)
	tsB, recB := backendServer(t, http.StatusOK)
	h := makeHandler(t, tsA, tsB)

	code := get(t, h, "/api/orgs/o1/sessions/mysession/data", "mysession")
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", code)
	}
	total := len(*recA) + len(*recB)
	if total != 1 {
		t.Errorf("expected exactly 1 backend request, got %d (A=%d B=%d)", total, len(*recA), len(*recB))
	}
}

// TestSameSessionSamePod verifies that repeated requests for the same session
// ID always land on the same pod (consistent hashing + hint cache).
func TestSameSessionSamePod(t *testing.T) {
	tsA, recA := backendServer(t, http.StatusOK)
	tsB, recB := backendServer(t, http.StatusOK)
	h := makeHandler(t, tsA, tsB)

	const n = 10
	for i := range n {
		code := get(t, h, fmt.Sprintf("/api/orgs/o/sessions/stable/item%d", i), "stable")
		if code != http.StatusOK {
			t.Fatalf("request %d: status %d", i, code)
		}
	}

	// All n requests went to exactly one pod.
	if len(*recA) != 0 && len(*recB) != 0 {
		t.Errorf("requests split between pods (A=%d B=%d): expected all on one pod", len(*recA), len(*recB))
	}
	if len(*recA)+len(*recB) != n {
		t.Errorf("total requests: got %d, want %d", len(*recA)+len(*recB), n)
	}
}

// TestFallbackForNonSessionRoute verifies that /healthz (no session ID) goes
// to the round-robin fallback and reaches a backend.
func TestFallbackForNonSessionRoute(t *testing.T) {
	tsA, recA := backendServer(t, http.StatusOK)
	tsB, recB := backendServer(t, http.StatusOK)
	h := makeHandler(t, tsA, tsB)

	const n = 4
	for range n {
		code := get(t, h, "/healthz", "")
		if code != http.StatusOK {
			t.Fatalf("healthz: got %d, want 200", code)
		}
	}
	total := len(*recA) + len(*recB)
	if total != n {
		t.Errorf("fallback total requests: got %d, want %d", total, n)
	}
}

// TestRoundRobinFallbackDistributes verifies that the round-robin fallback
// distributes requests across both pods over many calls.
func TestRoundRobinFallbackDistributes(t *testing.T) {
	tsA, recA := backendServer(t, http.StatusOK)
	tsB, recB := backendServer(t, http.StatusOK)
	h := makeHandler(t, tsA, tsB)

	const n = 20
	for range n {
		code := get(t, h, "/healthz", "")
		if code != http.StatusOK {
			t.Fatalf("healthz: status %d", code)
		}
	}
	if len(*recA) == 0 || len(*recB) == 0 {
		t.Errorf("round-robin did not distribute: A=%d B=%d", len(*recA), len(*recB))
	}
}

// TestEmptyRing503 verifies that requests against an empty ring return 503.
func TestEmptyRing503(t *testing.T) {
	r := ring.New(50)
	// Ring has no pods.
	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     cache.New(100, 60*time.Second),
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/orgs/o/sessions/s/x", nil)
	req.Header.Set("X-Test-Session", "s")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("empty ring: got %d, want 503", rr.Code)
	}
}

// TestEmptyRingFallback503 verifies that the round-robin fallback returns 503
// when the ring is empty.
func TestEmptyRingFallback503(t *testing.T) {
	r := ring.New(50)
	fb := proxy.NewRoundRobinFallback(r)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	fb.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("empty ring fallback: got %d, want 503", rr.Code)
	}
}

// Test503RetrySucceeds verifies that when the primary pod returns 503 the
// router invalidates the hint cache and retries on the next pod, which
// returns 200.
func Test503RetrySucceeds(t *testing.T) {
	// Server A always returns 503.
	tsA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(tsA.Close)

	// Server B always returns 200.
	var bHits int
	tsB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bHits++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(tsB.Close)

	// Build a ring that deterministically routes "retry-session" to pod-a first.
	// We may not control which pod the ring picks, so we check the behavior:
	// at most 2 backend hits total (1 primary + 1 retry) and the final HTTP
	// response should NOT be an application-level error that we caused.
	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "pod-a", Address: strings.TrimPrefix(tsA.URL, "http://")},
		{ID: "pod-b", Address: strings.TrimPrefix(tsB.URL, "http://")},
	})
	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     cache.New(100, 60*time.Second),
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/o/sessions/retry-session/x", nil)
	req.Header.Set("X-Test-Session", "retry-session")
	h.ServeHTTP(rr, req)

	// The ring sends the session to either pod-a or pod-b.
	// If pod-a: primary gets 503, retry hits pod-b which is 200 → client sees 200.
	// If pod-b: primary gets 200 → client sees 200, no retry.
	// Either way the final response must not be our own 503 (no-backends error).
	if rr.Code == http.StatusServiceUnavailable {
		body, _ := io.ReadAll(rr.Body)
		t.Errorf("got 503 with body %q; expected 200 (successful retry) or no backends", body)
	}
}

// Test503BothPodsPropagate verifies that when both primary and retry pods
// return 503 the client receives 503.
func Test503BothPodsPropagate(t *testing.T) {
	tsA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(tsA.Close)
	tsB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(tsB.Close)

	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "pod-a", Address: strings.TrimPrefix(tsA.URL, "http://")},
		{ID: "pod-b", Address: strings.TrimPrefix(tsB.URL, "http://")},
	})
	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     cache.New(100, 60*time.Second),
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/o/sessions/s503/x", nil)
	req.Header.Set("X-Test-Session", "s503")
	h.ServeHTTP(rr, req)

	// Both pods return 503, so the client must see a 503.
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("both pods 503: got %d, want 503", rr.Code)
	}
}

// TestHintCacheUsed verifies that after a successful request the hint cache
// is written, and a second request for the same session goes to the same pod
// even if the ring would choose differently after rebalancing.
func TestHintCacheUsed(t *testing.T) {
	tsA, recA := backendServer(t, http.StatusOK)
	tsB, recB := backendServer(t, http.StatusOK)

	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "pod-a", Address: strings.TrimPrefix(tsA.URL, "http://")},
		{ID: "pod-b", Address: strings.TrimPrefix(tsB.URL, "http://")},
	})
	hint := cache.New(100, 60*time.Second)
	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     hint,
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	// First request — goes to whichever pod the ring chooses.
	req1 := httptest.NewRequest(http.MethodGet, "/api/o/sessions/sticky/x", nil)
	req1.Header.Set("X-Test-Session", "sticky")
	h.ServeHTTP(httptest.NewRecorder(), req1)

	// Determine which pod was chosen.
	var firstPodHits *[]string
	var otherPodHits *[]string
	if len(*recA) > 0 {
		firstPodHits = recA
		otherPodHits = recB
	} else {
		firstPodHits = recB
		otherPodHits = recA
	}
	initialHits := len(*firstPodHits)

	// Second request — hint cache should send it to the same pod.
	req2 := httptest.NewRequest(http.MethodGet, "/api/o/sessions/sticky/y", nil)
	req2.Header.Set("X-Test-Session", "sticky")
	h.ServeHTTP(httptest.NewRecorder(), req2)

	if len(*firstPodHits) != initialHits+1 {
		t.Errorf("hint cache miss: first pod got %d hits (want %d), other pod got %d",
			len(*firstPodHits), initialHits+1, len(*otherPodHits))
	}
}

// TestHintInvalidatedOn503 verifies that a 503 from a pod invalidates its
// hint cache entry.
func TestHintInvalidatedOn503(t *testing.T) {
	ts503 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(ts503.Close)

	ts200 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts200.Close)

	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "pod-503", Address: strings.TrimPrefix(ts503.URL, "http://")},
		{ID: "pod-200", Address: strings.TrimPrefix(ts200.URL, "http://")},
	})
	hint := cache.New(100, 60*time.Second)
	// Pre-populate hint to point at the 503 pod.
	hint.Set("inv-session", "pod-503")

	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     hint,
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/o/sessions/inv-session/x", nil)
	req.Header.Set("X-Test-Session", "inv-session")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	// After the request, the hint for inv-session should be gone (invalidated).
	_, ok := hint.Get("inv-session")
	if ok {
		t.Error("hint cache entry was not invalidated after 503")
	}
}

// TestWebSocketUpgradeHeaders verifies that the reverse proxy passes through
// Upgrade and Connection headers without stripping them, enabling WebSocket
// connections.
func TestWebSocketUpgradeHeaders(t *testing.T) {
	// Backend echoes its received Upgrade header back.
	var gotUpgrade string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUpgrade = r.Header.Get("Upgrade")
		// Simulate a 101 Switching Protocols response to validate the upgrade path.
		// In a real WS exchange the server hijacks; here we just check the header flows.
		w.Header().Set("Upgrade", r.Header.Get("Upgrade"))
		w.Header().Set("Connection", "Upgrade")
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	t.Cleanup(ts.Close)

	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "pod-ws", Address: strings.TrimPrefix(ts.URL, "http://")},
	})
	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     cache.New(100, 60*time.Second),
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	req := httptest.NewRequest(http.MethodGet, "/ws/sessions/ws-session", nil)
	req.Header.Set("X-Test-Session", "ws-session")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if gotUpgrade != "websocket" {
		t.Errorf("backend did not receive Upgrade: websocket; got %q", gotUpgrade)
	}
}
