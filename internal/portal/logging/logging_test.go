package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jamsesh/internal/portal/logging"
)

func TestSetupReturnsLogger(t *testing.T) {
	// Setup must return a non-nil logger and not panic.
	l := logging.Setup("json", slog.LevelInfo)
	if l == nil {
		t.Fatal("Setup returned nil logger")
	}
}

func TestSetupTextFormat(t *testing.T) {
	l := logging.Setup("text", slog.LevelDebug)
	if l == nil {
		t.Fatal("Setup returned nil logger for text format")
	}
}

func TestAccessMiddlewareCapturesStatus(t *testing.T) {
	// Wire a JSON slog handler writing to a buffer so we can inspect the log.
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test/path", nil)

	logging.Access(nil)(inner).ServeHTTP(w, r)

	// The inner handler wrote 201.
	if w.Code != http.StatusCreated {
		t.Errorf("want 201, got %d", w.Code)
	}

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected at least one log line")
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("decode log line: %v\nline: %s", err, line)
	}

	check := func(field string, want any) {
		t.Helper()
		got, ok := entry[field]
		if !ok {
			t.Errorf("log missing field %q", field)
			return
		}
		// JSON numbers decode as float64; normalise for comparison.
		switch w := want.(type) {
		case int:
			if got != float64(w) {
				t.Errorf("field %q: want %v, got %v", field, want, got)
			}
		default:
			if got != want {
				t.Errorf("field %q: want %v, got %v", field, want, got)
			}
		}
	}

	check("method", "POST")
	check("path", "/test/path")
	check("status", 201)
}

func TestAccessMiddlewareDefaultStatus(t *testing.T) {
	// Handler that never calls WriteHeader — recorder defaults to 200.
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	logging.Access(nil)(inner).ServeHTTP(w, r)

	line := strings.TrimSpace(buf.String())
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("decode log line: %v", err)
	}

	got, ok := entry["status"]
	if !ok {
		t.Fatal("log line missing 'status' field")
	}
	if got != float64(200) {
		t.Errorf("want status=200, got %v", got)
	}
}

// TestAccessLogNoWSBearerLeak verifies that the access-log middleware does NOT
// include the Sec-WebSocket-Protocol header (which carries bearer tokens in the
// jamsesh.bearer.<token> subprotocol scheme) in any logged field. This pins the
// invariant that the middleware only logs path/method/status/duration/bytes/route
// so a future regression that adds header logging is caught immediately.
func TestAccessLogNoWSBearerLeak(t *testing.T) {
	const secretToken = "SECRET_TOKEN_123"

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws/session/abc", nil)
	r.Header.Set("Sec-WebSocket-Protocol", "jamsesh.bearer."+secretToken)

	logging.Access(nil)(inner).ServeHTTP(w, r)

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected at least one log line")
	}

	// The raw log line must not contain the token in any form.
	if strings.Contains(line, secretToken) {
		t.Errorf("access log line leaks bearer token %q: %s", secretToken, line)
	}

	// Also verify the log line has the expected safe fields.
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("decode log line: %v\nline: %s", err, line)
	}
	for _, field := range []string{"method", "path", "status"} {
		if _, ok := entry[field]; !ok {
			t.Errorf("log missing expected field %q", field)
		}
	}
	if entry["path"] != "/ws/session/abc" {
		t.Errorf("want path=/ws/session/abc, got %v", entry["path"])
	}
}

func TestAccessMiddlewareDurationAndBytes(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello world"))
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	logging.Access(nil)(inner).ServeHTTP(w, r)

	line := strings.TrimSpace(buf.String())
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("decode log line: %v", err)
	}

	if _, ok := entry["duration_ms"]; !ok {
		t.Error("log missing 'duration_ms' field")
	}
	if _, ok := entry["bytes"]; !ok {
		t.Error("log missing 'bytes' field")
	}
}
