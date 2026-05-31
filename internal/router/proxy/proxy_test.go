package proxy_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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
// router invalidates the hint cache and re-dispatches to the next pod, and the
// client sees the retry pod's 2xx — the leaked 503 must NEVER reach the client.
//
// The first attempt is pinned to the 503 pod via the hint cache so the retry
// path is exercised deterministically. (Routing the session by ring alone is
// not deterministic enough: it can land on pod-b first and skip the retry,
// which is how the original buffer-leak bug hid from this test.) The 503 pod
// returns a distinctive body so we can confirm the client did NOT receive it.
func Test503RetrySucceeds(t *testing.T) {
	const leak503Body = "LEAKED-503-FROM-POD-A"
	const okBody = "OK-FROM-POD-B"

	// Server A always returns 503 with a distinctive body.
	var aHits int
	tsA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aHits++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, leak503Body)
	}))
	t.Cleanup(tsA.Close)

	// Server B always returns 200.
	var bHits int
	tsB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bHits++
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, okBody)
	}))
	t.Cleanup(tsB.Close)

	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "pod-a", Address: strings.TrimPrefix(tsA.URL, "http://")},
		{ID: "pod-b", Address: strings.TrimPrefix(tsB.URL, "http://")},
	})
	hint := cache.New(100, 60*time.Second)
	// Pin the first attempt to the 503 pod so the re-dispatch path always runs.
	hint.Set("retry-session", "pod-a")
	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     hint,
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/o/sessions/retry-session/x", nil)
	req.Header.Set("X-Test-Session", "retry-session")
	h.ServeHTTP(rr, req)

	body, _ := io.ReadAll(rr.Body)

	// Invariant: the client sees the retry pod's 200, never the first pod's 503.
	if rr.Code != http.StatusOK {
		t.Errorf("re-dispatch: got status %d (body %q); want 200 from retry pod — "+
			"the first pod's 503 must not leak to the client", rr.Code, body)
	}
	if string(body) == leak503Body {
		t.Errorf("re-dispatch: client received the discarded 503 body %q — "+
			"the buffered first attempt leaked to the client", leak503Body)
	}
	// Both pods must have been hit exactly once: pod-a (503) then pod-b (retry).
	if aHits != 1 {
		t.Errorf("re-dispatch: pod-a (503) hit %d times; want exactly 1", aHits)
	}
	if bHits != 1 {
		t.Errorf("re-dispatch: pod-b (retry) hit %d times; want exactly 1 "+
			"(the retry must reach a distinct pod)", bHits)
	}
}

// Test1xxInformationalNotCapturedAsFinalStatus verifies that an upstream
// informational 1xx response (100 Continue — which git push drives via
// Expect: 100-continue, forwarded by httputil.ReverseProxy through WriteHeader
// before the final response) is NOT recorded as the final buffered status.
// Before the fix the buffered response captured the 100 and dropped the real
// 200, so a push routed through the router would fail.
func Test1xxInformationalNotCapturedAsFinalStatus(t *testing.T) {
	const okBody = "FINAL-200-BODY"
	var hits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusContinue) // 100 informational, like a reply to Expect: 100-continue
		w.WriteHeader(http.StatusOK)       // 200 final
		_, _ = io.WriteString(w, okBody)
	}))
	t.Cleanup(ts.Close)

	r := ring.New(50)
	r.SetPods([]ring.Pod{{ID: "pod-a", Address: strings.TrimPrefix(ts.URL, "http://")}})
	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     cache.New(100, 60*time.Second),
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/o/sessions/onexx-session/x", nil)
	req.Header.Set("X-Test-Session", "onexx-session")
	h.ServeHTTP(rr, req)

	body, _ := io.ReadAll(rr.Body)
	if rr.Code != http.StatusOK {
		t.Errorf("1xx: client got status %d (body %q); want the final 200 — a forwarded "+
			"100 Continue must not be captured as the final status", rr.Code, body)
	}
	if string(body) != okBody {
		t.Errorf("1xx: client body = %q; want %q (the final response body)", body, okBody)
	}
	if hits != 1 {
		t.Errorf("1xx: backend hit %d times; want 1 (no spurious retry triggered by the 100)", hits)
	}
}

// TestDeadPodTransportErrorFailsOver verifies that when the primary pod is dead
// (the upstream dial fails with connection-refused — the SIGKILLed-pod case, NOT
// a 503 status), the router invalidates the hint, redispatches to a distinct
// live pod, and the client sees that pod's 2xx — never a 502.
//
// This is the dead-pod-502 bug: a killed pod produces a transport error, not a
// 503, so the 503-only redispatch path never fired and the client got a 502.
// The first attempt is pinned to the dead pod via the hint cache so the failover
// path is exercised deterministically.
func TestDeadPodTransportErrorFailsOver(t *testing.T) {
	const okBody = "OK-FROM-LIVE-POD"

	// "Dead" pod: start a server, capture its address, then close it so any dial
	// to that address fails with connection-refused (mirrors a SIGKILLed pod).
	tsDead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadAddr := strings.TrimPrefix(tsDead.URL, "http://")
	tsDead.Close() // now deadAddr refuses connections

	// Live pod: always returns 200.
	var liveHits int32
	tsLive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&liveHits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, okBody)
	}))
	t.Cleanup(tsLive.Close)

	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "pod-dead", Address: deadAddr},
		{ID: "pod-live", Address: strings.TrimPrefix(tsLive.URL, "http://")},
	})
	hint := cache.New(100, 60*time.Second)
	// Pin the first attempt to the dead pod so the failover path always runs.
	hint.Set("dead-session", "pod-dead")
	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     hint,
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/o/sessions/dead-session/x", nil)
	req.Header.Set("X-Test-Session", "dead-session")
	h.ServeHTTP(rr, req)

	body, _ := io.ReadAll(rr.Body)

	// Invariant: the client sees the live pod's 200, never a 502 from the dead pod.
	if rr.Code != http.StatusOK {
		t.Errorf("dead-pod failover: got status %d (body %q); want 200 from the live pod — "+
			"a dial/transport error to a dead pod must fail over, not return 502", rr.Code, body)
	}
	if string(body) != okBody {
		t.Errorf("dead-pod failover: client received body %q; want %q from the live pod", body, okBody)
	}
	if got := atomic.LoadInt32(&liveHits); got != 1 {
		t.Errorf("dead-pod failover: live pod hit %d times; want exactly 1 (the failover must reach it)", got)
	}
	// The dead pod's hint must be invalidated so future requests don't re-pin it.
	if _, ok := hint.Get("dead-session"); ok {
		t.Error("dead-pod failover: hint for dead session was not invalidated after the transport error")
	}
}

// TestBothPodsDeadReturns502 verifies the failover-exhaustion path for transport
// errors: when both the primary and retry pods are dead (dial failures), the
// client receives a 502 after exactly two pod attempts — never an indefinite
// hang and never more than the bounded retry.
func TestBothPodsDeadReturns502(t *testing.T) {
	// Two dead addresses: start then close so both refuse connections.
	mkDead := func() string {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		addr := strings.TrimPrefix(ts.URL, "http://")
		ts.Close()
		return addr
	}
	deadA := mkDead()
	deadB := mkDead()

	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "pod-a", Address: deadA},
		{ID: "pod-b", Address: deadB},
	})
	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     cache.New(100, 60*time.Second),
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/o/sessions/both-dead/x", nil)
	req.Header.Set("X-Test-Session", "both-dead")
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("both pods dead: got %d, want 502 (bad gateway) after bounded failover", rr.Code)
	}
}

// Test503BothPodsPropagate verifies the bounded-retry exhaustion path: when
// both the primary and retry pods return 503, the client receives 503 after
// exactly two pod attempts (initial + one retry) — never more.
func Test503BothPodsPropagate(t *testing.T) {
	var hits int32
	mk503 := func() *httptest.Server {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		t.Cleanup(ts.Close)
		return ts
	}
	tsA := mk503()
	tsB := mk503()

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
	// Bounded retry: exactly two pod attempts total (initial + one retry).
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("bounded retry: got %d pod attempts; want exactly 2 (initial + one retry)", got)
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

// Test503RetryReplaysRequestBody verifies that when the first pod returns 503,
// the re-dispatch to the retry pod resends the FULL request body — the first
// attempt consuming r.Body must not leave the retry pod with an empty body.
func Test503RetryReplaysRequestBody(t *testing.T) {
	const body = "the-full-request-body-payload"

	// Pod A (pinned first) returns 503 after draining the body.
	tsA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(tsA.Close)

	// Pod B (retry) echoes the body it received so we can assert it is intact.
	var gotBody string
	tsB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(tsB.Close)

	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "pod-a", Address: strings.TrimPrefix(tsA.URL, "http://")},
		{ID: "pod-b", Address: strings.TrimPrefix(tsB.URL, "http://")},
	})
	hint := cache.New(100, 60*time.Second)
	hint.Set("body-session", "pod-a") // pin first attempt to the 503 pod
	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     hint,
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/o/sessions/body-session/x",
		strings.NewReader(body))
	req.Header.Set("X-Test-Session", "body-session")
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("re-dispatch with body: got status %d; want 200 from retry pod", rr.Code)
	}
	if gotBody != body {
		t.Errorf("retry pod received body %q; want %q — the request body was not replayed on re-dispatch", gotBody, body)
	}
}

// Test503OversizedBodyNotRetried verifies that a request whose body exceeds the
// retry-buffer cap is NOT re-dispatched: the first pod's 503 is surfaced rather
// than replaying a truncated body to another pod.
func Test503OversizedBodyNotRetried(t *testing.T) {
	// Pod A (pinned) returns 503; pod B must NOT be hit for an oversized body.
	tsA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(tsA.Close)

	var bHit int32
	tsB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&bHit, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(tsB.Close)

	r := ring.New(50)
	r.SetPods([]ring.Pod{
		{ID: "pod-a", Address: strings.TrimPrefix(tsA.URL, "http://")},
		{ID: "pod-b", Address: strings.TrimPrefix(tsB.URL, "http://")},
	})
	hint := cache.New(100, 60*time.Second)
	hint.Set("big-session", "pod-a")
	h := &proxy.Handler{
		Extract:  staticExtract,
		Ring:     r,
		Hint:     hint,
		Fallback: proxy.NewRoundRobinFallback(r),
	}

	// 2 MiB body — exceeds the 1 MiB retry cap.
	big := strings.Repeat("x", 2<<20)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/o/sessions/big-session/x",
		strings.NewReader(big))
	req.Header.Set("X-Test-Session", "big-session")
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("oversized-body 503: got %d; want 503 surfaced without retry", rr.Code)
	}
	if got := atomic.LoadInt32(&bHit); got != 0 {
		t.Errorf("oversized-body 503: retry pod was hit %d times; want 0 (no replay of an oversized body)", got)
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
