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

// TestRedactQueryTokens exercises RedactQueryTokens across the documented
// sensitive-param set and edge cases. Assertions are on the actual security
// invariant (the raw token value must not appear) rather than on exact
// URL-encoding output, which may vary between Go versions.
func TestRedactQueryTokens(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		wantContain string   // substring that MUST appear in output
		wantAbsent  []string // values that must NOT appear
	}{
		{
			name:        "no query string passes through unchanged",
			input:       "",
			wantContain: "",
		},
		{
			name:        "token value is redacted, param name is preserved",
			input:       "token=abc123&foo=bar",
			wantContain: "token=",
			wantAbsent:  []string{"abc123"},
		},
		{
			name:        "code param is redacted",
			input:       "code=xyz789",
			wantContain: "code=",
			wantAbsent:  []string{"xyz789"},
		},
		{
			name:        "state param is redacted",
			input:       "state=csrf-secret&next=%2Fdashboard",
			wantContain: "state=",
			wantAbsent:  []string{"csrf-secret"},
		},
		{
			name:        "ticket param is redacted",
			input:       "ticket=tkt-secret",
			wantContain: "ticket=",
			wantAbsent:  []string{"tkt-secret"},
		},
		{
			name:        "case-insensitive: TOKEN is redacted",
			input:       "TOKEN=uppercaseSecret",
			wantContain: "TOKEN=",
			wantAbsent:  []string{"uppercaseSecret"},
		},
		{
			name:        "non-sensitive params are not redacted",
			input:       "foo=bar&baz=qux",
			wantContain: "bar",
		},
		{
			name:        "full URL string: token in query is redacted",
			input:       "/auth/magic-link?token=secretlink&redirect=%2F",
			wantContain: "token=",
			wantAbsent:  []string{"secretlink"},
		},
		{
			name:        "redacted value sentinel is present",
			input:       "token=abc",
			wantContain: "<redacted>",
			wantAbsent:  []string{"abc"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := logging.RedactQueryTokens(tc.input)
			if tc.wantContain != "" && !strings.Contains(got, tc.wantContain) {
				t.Errorf("RedactQueryTokens(%q) = %q; want it to contain %q", tc.input, got, tc.wantContain)
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("RedactQueryTokens(%q) = %q; must not contain %q", tc.input, got, absent)
				}
			}
		})
	}
}

// TestRedactQueryTokensMultipleValues verifies that when the same sensitive
// param appears multiple times (e.g. repeated token= keys), all values are
// redacted — not just the first occurrence.
func TestRedactQueryTokensMultipleValues(t *testing.T) {
	input := "token=first&token=second"
	got := logging.RedactQueryTokens(input)
	for _, raw := range []string{"first", "second"} {
		if strings.Contains(got, raw) {
			t.Errorf("RedactQueryTokens(%q) = %q; still contains raw value %q", input, got, raw)
		}
	}
	if !strings.Contains(got, "token=") {
		t.Errorf("RedactQueryTokens(%q) = %q; param name 'token' should be preserved", input, got)
	}
}

// TestRedactQueryTokensMalformed verifies that a malformed query string never
// returns the raw input unchanged when it contains a sensitive param name.
// Either the value is redacted or the whole string is replaced.
func TestRedactQueryTokensMalformed(t *testing.T) {
	// A query string that url.ParseQuery may struggle with but still contains
	// a token key — we want to ensure the raw value never leaks.
	input := "token=%ZZ" // invalid percent-encoding
	got := logging.RedactQueryTokens(input)
	// The raw sequence "%ZZ" is the (invalid-encoded) value — it should not appear
	// verbatim in the output after the token= key.
	if strings.Contains(got, "token=%ZZ") {
		t.Errorf("RedactQueryTokens(%q) = %q; raw token value leaked through", input, got)
	}
}

func TestAccessMiddlewareLogsRedactedQuery(t *testing.T) {
	// Verify that the access middleware logs a 'query' field and that
	// a token in the query string is redacted in the log output.
	const secretToken = "verysecrettoken"

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/auth/magic-link?token="+secretToken+"&redirect=%2F", nil)

	logging.Access(nil)(inner).ServeHTTP(w, r)

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected at least one log line")
	}

	// The raw secret must not appear anywhere in the log line.
	if strings.Contains(line, secretToken) {
		t.Errorf("access log leaks raw token %q: %s", secretToken, line)
	}

	// A 'query' field must be present and contain the redacted sentinel.
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("decode log line: %v\nline: %s", err, line)
	}
	q, ok := entry["query"]
	if !ok {
		t.Fatal("log line missing 'query' field")
	}
	qs, ok := q.(string)
	if !ok {
		t.Fatalf("'query' field is not a string: %T", q)
	}
	if !strings.Contains(qs, "<redacted>") {
		t.Errorf("'query' field %q does not contain '<redacted>'", qs)
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
