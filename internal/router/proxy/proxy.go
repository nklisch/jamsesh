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
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
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
		h.incDecision("fallback")
		h.Fallback.ServeHTTP(w, r)
		return
	}

	// Resolve pod: hint cache first, ring as fallback.
	pod, source := h.resolvePod(sessionID)
	if pod.Address == "" {
		slog.WarnContext(r.Context(), "router: no backends available",
			"session_id", sessionID)
		h.incDecision("empty_ring")
		h.writeNoBackend(w)
		return
	}

	slog.DebugContext(r.Context(), "router: routing request",
		"session_id", sessionID,
		"pod_id", pod.ID,
		"source", source,
	)

	// Record the routing source (hit_cache or hit_ring).
	switch source {
	case "cache":
		h.incDecision("hit_cache")
	default:
		h.incDecision("hit_ring")
	}

	// Prepare the request body for a possible re-dispatch. The first upstream
	// attempt consumes r.Body, so retrying a body-bearing request (POST/PUT/PATCH
	// session creation routes through here) would resend an empty body. rewind
	// buffers the body up to maxRetryBodyBytes so each attempt gets a fresh
	// reader; retrySafe reports whether a retry can faithfully replay the body.
	// A body larger than the cap is left streamed (not buffered) and retrySafe is
	// false — we don't redispatch oversized uploads, we surface the 503 instead.
	rewind, retrySafe := prepareRetryBody(r)

	// First attempt.
	//
	// The first attempt is buffered rather than streamed straight to the
	// client: if the chosen pod returns 503 (lease held elsewhere) we must be
	// able to discard that response and transparently re-dispatch to another
	// pod, presenting only the successful response to the client. Streaming the
	// first attempt directly to w (the old behaviour) flushed the 503 status
	// line before we knew a retry was warranted, so the retry's response could
	// never reach the client — the client always saw the leaked 503.
	//
	// A bufferedResponse delegates Hijack/Flush to w so WebSocket upgrades and
	// streaming still work: once the connection is hijacked or the upstream
	// flushes (streaming download), the response is "committed" and we never
	// retry or replay it — only fully-buffered, non-streamed responses are held
	// in memory.
	first := newBufferedResponse(w)
	h.proxyTo(pod, first, r)

	if first.committed {
		// The connection was hijacked (WebSocket upgrade) or the response was
		// streamed (flushed) directly to w. Nothing left to flush or retry.
		return
	}

	if first.status != http.StatusServiceUnavailable {
		// Not a 503: commit this response to the client.
		first.flush(w)
		// Success path: update hint cache on 2xx (or an implicit 200).
		if first.status == 0 || (first.status >= 200 && first.status < 300) {
			h.Hint.Set(sessionID, pod.ID)
		}
		return
	}

	// 503 from first pod: invalidate hint and retry once on a distinct pod.
	slog.WarnContext(r.Context(), "router: pod returned 503, retrying",
		"session_id", sessionID,
		"failed_pod", pod.ID,
	)
	h.Hint.Invalidate(sessionID)

	retryPod := h.Ring.GetNext(sessionID, pod.ID)
	// Surface the 503 without retrying when there is no distinct retry target,
	// or when the request body is too large to replay faithfully.
	if retryPod.Address == "" || retryPod.ID == pod.ID || !retrySafe {
		reason := "no retry target"
		if !retrySafe && retryPod.Address != "" && retryPod.ID != pod.ID {
			reason = "request body not replayable"
		}
		slog.WarnContext(r.Context(), "router: propagating 503 without retry",
			"session_id", sessionID, "reason", reason)
		h.incDecision("error_503")
		first.flush(w)
		return
	}

	slog.InfoContext(r.Context(), "router: retrying on next pod",
		"session_id", sessionID,
		"retry_pod", retryPod.ID,
	)
	h.incDecision("retry")

	// Rewind the request body so the retry pod receives the full payload.
	rewind()

	// Retry against the distinct pod, buffered as well so a successful retry
	// fully replaces the discarded 503. This is the bounded retry: exactly one
	// additional pod attempt. Whatever the retry produces (2xx on success, or
	// another 503 when the second pod is also unavailable) is what the client
	// sees — we never fall back to the discarded first-attempt 503.
	retry := newBufferedResponse(w)
	h.proxyTo(retryPod, retry, r)
	if retry.committed {
		return
	}
	retry.flush(w)
	// Don't update hint on retry; let the next request re-establish via ring.
}

// maxRetryBodyBytes caps how much request body the router buffers to enable a
// re-dispatch retry. Session API requests (JSON) are tiny; large uploads (git
// packfiles) exceed this and are streamed without retry — on a 503 the original
// 503 is surfaced rather than buffering an unbounded payload in memory.
const maxRetryBodyBytes = 1 << 20 // 1 MiB

// prepareRetryBody makes r.Body replayable for a single re-dispatch attempt.
//
// It returns rewind, which resets r.Body to a fresh reader over the buffered
// bytes, and retrySafe, which reports whether a faithful replay is possible:
//   - No body (nil or http.NoBody): retrySafe = true, rewind is a no-op.
//   - Body ≤ maxRetryBodyBytes: fully buffered; retrySafe = true.
//   - Body > maxRetryBodyBytes: left as a combined reader over the bytes read so
//     far plus the rest of the stream (so the FIRST attempt still sees the whole
//     body); retrySafe = false (we will not retry an oversized body).
func prepareRetryBody(r *http.Request) (rewind func(), retrySafe bool) {
	if r.Body == nil || r.Body == http.NoBody {
		return func() {}, true
	}

	limited := io.LimitReader(r.Body, maxRetryBodyBytes+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		// Could not read the body for buffering; leave whatever remains and do
		// not retry. The first attempt will surface the read error itself.
		// Preserve the original body's Close so the connection is not leaked.
		r.Body = &multiReadCloser{
			r: io.MultiReader(bytes.NewReader(buf), r.Body),
			c: r.Body,
		}
		return func() {}, false
	}

	if len(buf) > maxRetryBodyBytes {
		// Oversized: don't buffer the rest. Reconstruct a single stream of the
		// already-read prefix followed by the unread remainder for the first
		// attempt, and disable retry. The original body's Close is preserved so
		// the upstream request body is drained/closed correctly.
		orig := r.Body
		r.Body = &multiReadCloser{
			r: io.MultiReader(bytes.NewReader(buf), orig),
			c: orig,
		}
		return func() {}, false
	}

	// Fully buffered. Close the original body and hand out fresh readers.
	_ = r.Body.Close()
	r.ContentLength = int64(len(buf))
	reset := func() {
		r.Body = io.NopCloser(bytes.NewReader(buf))
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(buf)), nil
		}
	}
	reset()
	return reset, true
}

// multiReadCloser adapts an io.Reader (typically an io.MultiReader over a
// buffered prefix plus the unread remainder of a request body) into an
// io.ReadCloser that closes the underlying body, so reconstructing a request
// body for the first attempt does not leak the original body's Close.
type multiReadCloser struct {
	r io.Reader
	c io.Closer
}

func (m *multiReadCloser) Read(p []byte) (int, error) { return m.r.Read(p) }
func (m *multiReadCloser) Close() error               { return m.c.Close() }

// incDecision increments RouterDecisionsTotal for the given result label. It
// is a no-op when Metrics is nil or RouterDecisionsTotal is nil.
func (h *Handler) incDecision(result string) {
	if h.Metrics == nil || h.Metrics.RouterDecisionsTotal == nil {
		return
	}
	h.Metrics.RouterDecisionsTotal.WithLabelValues(result).Inc()
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

// ── bufferedResponse ──────────────────────────────────────────────────────────

// bufferedResponse is an [http.ResponseWriter] that buffers the upstream
// response (status code, headers, body) instead of streaming it to the client.
// This lets the router inspect the status code and decide whether to commit the
// response or discard it and re-dispatch to another pod.
//
// Hijack and Flush delegate to the real underlying writer so WebSocket upgrades
// and streaming responses still work. The first time the underlying connection
// is taken over directly (a successful Hijack, or a Flush after headers are
// sent), committed is set: from that point the response has reached the client
// directly and must not be retried or replayed.
type bufferedResponse struct {
	real      http.ResponseWriter // underlying writer (for Hijack/Flush delegation)
	header    http.Header         // buffered response headers
	body      bytes.Buffer        // buffered response body
	status    int                 // captured status code (0 = WriteHeader not yet called)
	wrote     bool                // WriteHeader has been called
	committed bool                // response went directly to real (hijack/flush) — do not retry/replay
	hijacked  bool                // connection was hijacked — caller owns the raw conn
}

// newBufferedResponse returns a bufferedResponse wrapping real. Its header map
// is independent of real.Header() until flush copies the buffered headers over.
func newBufferedResponse(real http.ResponseWriter) *bufferedResponse {
	return &bufferedResponse{
		real:   real,
		header: make(http.Header),
	}
}

// Header returns the response header map. Before the response is committed this
// is the buffered map (copied to the real writer by flush). After a Flush
// switched to direct streaming it returns the real writer's header so later
// mutations (e.g. trailer values the upstream proxy writes after the first
// flush) land on the actual response. After a Hijack the caller owns the raw
// connection; per the net/http hijack contract Header() must not touch the real
// writer, so the detached buffered map is returned (writes to it are inert).
func (b *bufferedResponse) Header() http.Header {
	if b.committed && !b.hijacked {
		return b.real.Header()
	}
	return b.header
}

// WriteHeader records the upstream status code without touching the client.
func (b *bufferedResponse) WriteHeader(code int) {
	if b.wrote {
		return
	}
	b.wrote = true
	b.status = code
}

// Write buffers the response body. An implicit 200 is recorded if WriteHeader
// was not called first, mirroring http.ResponseWriter semantics. After the
// response has been committed (the first Flush switched to direct streaming),
// writes go straight to the real writer.
func (b *bufferedResponse) Write(p []byte) (int, error) {
	if !b.wrote {
		b.WriteHeader(http.StatusOK)
	}
	if b.committed {
		return b.real.Write(p)
	}
	return b.body.Write(p)
}

// Flush delegates to the real writer. Buffering and flushing are mutually
// exclusive: a streamed (flushed) response is committed directly to the client
// and is never retried. Any data buffered before the first Flush is replayed to
// the real writer first so the client receives a coherent stream.
//
// It uses http.ResponseController so a real writer wrapped by other middleware
// (exposing Flush via the Unwrap chain rather than implementing http.Flusher
// directly) is still flushed.
func (b *bufferedResponse) Flush() {
	if !b.committed {
		// Replay anything buffered so far, then switch to direct streaming.
		b.flush(b.real)
		b.committed = true
	}
	_ = http.NewResponseController(b.real).Flush()
}

// Hijack delegates to the real writer (WebSocket upgrades). Once hijacked the
// response is committed: the caller owns the raw connection and the router must
// not retry or replay.
//
// It uses http.ResponseController so a real writer wrapped by other middleware
// (exposing Hijack via the Unwrap chain rather than implementing http.Hijacker
// directly) can still be hijacked.
func (b *bufferedResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(b.real).Hijack()
	if err == nil {
		b.committed = true
		b.hijacked = true
	}
	return conn, rw, err
}

// flush copies the buffered status, headers, and body to dst. It is a no-op
// once the response has been committed directly (hijacked or already streamed),
// to avoid double-writing. flush must be called at most once per buffered
// response for a non-committed response.
func (b *bufferedResponse) flush(dst http.ResponseWriter) {
	if b.committed {
		return
	}
	dstHeader := dst.Header()
	for k, vs := range b.header {
		dstHeader[k] = vs
	}
	status := b.status
	if status == 0 {
		status = http.StatusOK
	}
	dst.WriteHeader(status)
	_, _ = dst.Write(b.body.Bytes())
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
