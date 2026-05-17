// Package logging provides structured slog setup and HTTP access-log middleware.
package logging

import (
	"log/slog"
	"net/http"
	"os"
	"time"
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

// Access wraps every request in a structured access-log line written at
// request completion. Reads the request ID injected by chi's RequestID
// middleware via r.Context() (slog carries it automatically when the context
// key is set).
func Access(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)
		slog.InfoContext(r.Context(), "http access",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sr.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes", sr.bytes,
		)
	})
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
