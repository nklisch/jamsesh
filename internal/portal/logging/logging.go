// Package logging provides structured slog setup and HTTP access-log middleware.
package logging

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/portal/metrics"
)

// Setup configures the default slog logger. format is "text" or "json"
// (default); level is the minimum log level. Returns the configured logger
// so callers can scope attributes via logger.With(...).
func Setup(format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch format {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	l := slog.New(handler)
	slog.SetDefault(l)
	return l
}

// Access returns an HTTP middleware that writes a structured access-log line at
// request completion. If reg is non-nil, it also records http_requests_total and
// http_request_duration_seconds Prometheus metrics.
//
// Route labels are extracted from the chi route pattern
// (chi.RouteContext(r.Context()).RoutePattern()) after the downstream handler
// returns, so the pattern is fully resolved. Unmatched routes (pattern == "")
// use the sentinel label "unknown" to prevent cardinality explosion.
//
// Passing nil for reg makes the metrics recording a no-op, which keeps tests
// that build router.Deps without a registry working correctly.
func Access(reg *metrics.Registry) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sr, r)
			elapsed := time.Since(start)

			// Extract the chi route pattern after routing so it is fully resolved.
			route := "unknown"
			if rctx := chi.RouteContext(r.Context()); rctx != nil {
				if p := rctx.RoutePattern(); p != "" {
					route = p
				}
			}

			slog.InfoContext(r.Context(), "http access",
				"method", r.Method,
				"path", r.URL.Path,
				"query", RedactQueryTokens(r.URL.RawQuery),
				"route", route,
				"status", sr.status,
				"duration_ms", elapsed.Milliseconds(),
				"bytes", sr.bytes,
			)

			if reg != nil {
				statusLabel := fmt.Sprintf("%d", sr.status)
				reg.HTTPRequestsTotal.WithLabelValues(r.Method, route, statusLabel).Inc()
				reg.HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(elapsed.Seconds())
			}
		})
	}
}

// statusRecorder wraps http.ResponseWriter to capture the status code and
// response body byte count written by downstream handlers.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Unwrap returns the underlying http.ResponseWriter. This is required by
// coder/websocket (and the standard net/http ResponseController) to find
// http.Hijacker through the middleware chain. Without Unwrap, WebSocket
// upgrade attempts fail with "does not implement http.Hijacker".
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
