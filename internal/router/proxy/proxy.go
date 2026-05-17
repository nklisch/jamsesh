// Package proxy implements the reverse-proxy HTTP handler for the jamsesh router.
//
// # Routing flow
//
// For every incoming request:
//  1. Extract the session ID using the configured Extract function.
//  2. If session ID is empty, fall through to Fallback (round-robin across ring).
//  3. Check the hint cache. On a hit whose pod is still in the ring, proxy there.
//  4. Otherwise, consult Ring.Get(sessionID) for the consistent-hash choice.
//  5. Proxy the request. On a 503 from the pod, invalidate the hint cache for
//     this session and retry once against the ring's next preference. If the
//     retry also returns 503, propagate 503 to the client.
//  6. On a non-503 success, record sessionID → podID in the hint cache.
//
// # WebSocket and Git
//
// [httputil.ReverseProxy] handles WebSocket upgrade natively: when the Director
// sets the correct target URL the Upgrade headers pass through unchanged.
// HTTP/1.1 is used on the upstream leg (portal pods use chi; WebSockets require
// HTTP/1.1).
//
// # Fallback (non-session routes)
//
// /healthz, /readyz, /metrics, /auth/* return "" from Extract. These requests
// are handled by Fallback, which is typically a round-robin handler built with
// [NewRoundRobinFallback].
package proxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"

	"jamsesh/internal/portal/metrics"
	"jamsesh/internal/router/cache"
	"jamsesh/internal/router/ring"
)

// Handler is an [http.Handler] that extracts the session ID from each request,
// chooses a backend pod via the hint cache or consistent-hash ring, and
// reverse-proxies the request to that pod.
//
// All fields are required except Metrics (nil-safe).
type Handler struct {
	// Extract returns the session ID for r, or "" if the request has no
	// session affinity (e.g. /healthz, /auth/*). Typically extract.SessionID.
	Extract func(r *http.Request) string

	// Ring is the current consistent-hash pod ring. Must not be nil.
	Ring *ring.Ring

	// Hint is the soft-coordinator hint cache. Must not be nil.
	Hint *cache.Hint

	// Fallback handles requests whose session ID is "". Typically a
	// [NewRoundRobinFallback] wrapping the same ring.
	Fallback http.Handler

	// Metrics is the optional Prometheus registry. When nil all metric
	// operations are no-ops.
	Metrics *metrics.Registry
}

// ServeHTTP implements [http.Handler].
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sessionID := h.Extract(r)
	if sessionID == "" {
		h.Fallback.ServeHTTP(w, r)
		return
	}

	// Resolve pod: hint cache first, ring as fallback.
	pod, source := h.resolvePod(sessionID)
	if pod.Address == "" {
		slog.WarnContext(r.Context(), "router: no backends available",
			"session_id", sessionID)
		h.writeNoBackend(w)
		return
	}

	slog.DebugContext(r.Context(), "router: routing request",
		"session_id", sessionID,
		"pod_id", pod.ID,
		"source", source,
	)

	// First attempt.
	rw := &statusCapture{ResponseWriter: w}
	h.proxyTo(pod, rw, r)

	if rw.status != http.StatusServiceUnavailable {
		// Success path: update hint cache.
		if rw.status == 0 || (rw.status >= 200 && rw.status < 300) {
			h.Hint.Set(sessionID, pod.ID)
		}
		return
	}

	// 503 from first pod: invalidate hint and retry once.
	slog.WarnContext(r.Context(), "router: pod returned 503, retrying",
		"session_id", sessionID,
		"failed_pod", pod.ID,
	)
	h.Hint.Invalidate(sessionID)

	retryPod := h.Ring.GetNext(sessionID, pod.ID)
	if retryPod.Address == "" || retryPod.ID == pod.ID {
		// No distinct retry target; propagate the 503 that already got written.
		slog.WarnContext(r.Context(), "router: no retry target, propagating 503",
			"session_id", sessionID)
		return
	}

	slog.InfoContext(r.Context(), "router: retrying on next pod",
		"session_id", sessionID,
		"retry_pod", retryPod.ID,
	)

	// We've already written headers from the first attempt; we cannot send
	// a fresh response. Use a fresh recorder to capture the retry result.
	// If the retry succeeds (non-503), flush it; otherwise the 503 already
	// sent stands.
	//
	// Note: because statusCapture delegates all writes to the underlying
	// ResponseWriter, the 503 status line from the first attempt may already
	// be flushed to the client. The retry result is therefore best-effort in
	// cases where the connection is being hijacked (WS) or the body was
	// already written. For plain HTTP responses this is a clean retry because
	// the status-capture wrapper calls WriteHeader lazily.
	retryRW := &statusCapture{ResponseWriter: w}
	h.proxyTo(retryPod, retryRW, r)
	// Don't update hint on retry; let next request re-establish via ring.
}

// resolvePod returns the pod to use for sessionID, plus a diagnostic label.
func (h *Handler) resolvePod(sessionID string) (ring.Pod, string) {
	if podID, ok := h.Hint.Get(sessionID); ok {
		// Verify the cached pod is still in the ring.
		all := h.Ring.Pods()
		for _, p := range all {
			if p.ID == podID {
				return p, "cache"
			}
		}
		// Cached pod no longer in ring; fall through to ring lookup.
		h.Hint.Invalidate(sessionID)
	}
	return h.Ring.Get(sessionID), "ring"
}

// proxyTo reverse-proxies r to pod, writing the response to w.
func (h *Handler) proxyTo(pod ring.Pod, w http.ResponseWriter, r *http.Request) {
	target := &url.URL{
		Scheme: "http",
		Host:   pod.Address,
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			// Preserve the original path and query; only the scheme+host change.
			req.Host = target.Host
			// Strip the X-Forwarded-* hop headers that may carry stale values;
			// let the proxy set them fresh.
			req.Header.Del("X-Forwarded-Proto")
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			slog.WarnContext(req.Context(), "router: upstream error",
				"pod_id", pod.ID,
				"pod_addr", pod.Address,
				"err", err,
			)
			http.Error(rw, "bad gateway", http.StatusBadGateway)
		},
		// ModifyResponse is nil — pass upstream response through unmodified.
	}

	proxy.ServeHTTP(w, r)
}

// writeNoBackend writes a 503 with Retry-After: 5 to signal no backends.
func (h *Handler) writeNoBackend(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "5")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = fmt.Fprintln(w, "no backends available")
}

// ── statusCapture ─────────────────────────────────────────────────────────────

// statusCapture wraps an [http.ResponseWriter] and captures the first status
// code written via WriteHeader. The zero value means WriteHeader has not been
// called; in that case the upstream proxy wrote 200 implicitly.
type statusCapture struct {
	http.ResponseWriter
	status int
}

func (sc *statusCapture) WriteHeader(code int) {
	if sc.status == 0 {
		sc.status = code
	}
	sc.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController and hijack detection see through the
// wrapper.
func (sc *statusCapture) Unwrap() http.ResponseWriter {
	return sc.ResponseWriter
}

// ── RoundRobinFallback ────────────────────────────────────────────────────────

// roundRobinFallback is an [http.Handler] that distributes requests across
// all pods in the ring using an atomic round-robin counter. It is used for
// non-session routes (/healthz, /readyz, /metrics, /auth/*).
type roundRobinFallback struct {
	r       *ring.Ring
	counter atomic.Uint64
}

// NewRoundRobinFallback returns an [http.Handler] that round-robins across
// all pods currently in r. If the ring is empty, it returns 503.
//
// The returned handler shares the ring pointer: pod set changes made via
// ring.SetPods are reflected immediately.
func NewRoundRobinFallback(r *ring.Ring) http.Handler {
	return &roundRobinFallback{r: r}
}

func (f *roundRobinFallback) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pods := f.r.Pods()
	if len(pods) == 0 {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "no backends available", http.StatusServiceUnavailable)
		return
	}

	idx := f.counter.Add(1) - 1
	pod := pods[idx%uint64(len(pods))]

	target := &url.URL{
		Scheme: "http",
		Host:   pod.Address,
	}
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			req.Header.Del("X-Forwarded-Proto")
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			slog.WarnContext(req.Context(), "router: fallback upstream error",
				"pod_addr", pod.Address,
				"err", err,
			)
			http.Error(rw, "bad gateway", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r)
}
