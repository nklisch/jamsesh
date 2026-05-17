package extract_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"jamsesh/internal/router/extract"
)

func req(method, path string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

// TestSessionID_REST covers all REST path variants.
func TestSessionID_REST(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "root path",
			path: "/api/orgs/org1/sessions/sess123",
			want: "sess123",
		},
		{
			name: "with sub-resource",
			path: "/api/orgs/org1/sessions/sess123/comments",
			want: "sess123",
		},
		{
			name: "finalize",
			path: "/api/orgs/org1/sessions/sess123/finalize",
			want: "sess123",
		},
		{
			name: "invite sub-path",
			path: "/api/orgs/org1/sessions/sess123/invites/inv99/accept",
			want: "sess123",
		},
		{
			name: "trailing slash",
			path: "/api/orgs/org1/sessions/sess123/",
			want: "sess123",
		},
		{
			name: "sessions list (no sessionID)",
			path: "/api/orgs/org1/sessions",
			want: "",
		},
		{
			name: "orgs list (no sessions key)",
			path: "/api/orgs/org1",
			want: "",
		},
		{
			name: "non-session API path",
			path: "/api/accounts/me",
			want: "",
		},
		{
			name: "api root",
			path: "/api/",
			want: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extract.SessionID(req(http.MethodGet, tt.path, nil))
			if got != tt.want {
				t.Errorf("SessionID(%q) = %q; want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestSessionID_Git covers the /git/{orgID}/{sessionID}.git/... shape.
func TestSessionID_Git(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "info/refs",
			path: "/git/org1/sess456.git/info/refs",
			want: "sess456",
		},
		{
			name: "upload-pack",
			path: "/git/org1/sess456.git/git-upload-pack",
			want: "sess456",
		},
		{
			name: "receive-pack",
			path: "/git/org1/sess456.git/git-receive-pack",
			want: "sess456",
		},
		{
			name: "no trailing path",
			path: "/git/org1/sess456.git",
			want: "sess456",
		},
		{
			name: "trailing slash after .git",
			path: "/git/org1/sess456.git/",
			want: "sess456",
		},
		{
			name: "missing .git suffix",
			path: "/git/org1/sess456/info/refs",
			want: "",
		},
		{
			name: "no sessionID segment",
			path: "/git/org1/",
			want: "",
		},
		{
			name: "only org segment",
			path: "/git/org1",
			want: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extract.SessionID(req(http.MethodGet, tt.path, nil))
			if got != tt.want {
				t.Errorf("SessionID(%q) = %q; want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestSessionID_WS covers the /ws/sessions/{sessionID} shape.
func TestSessionID_WS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "basic",
			path: "/ws/sessions/wsess789",
			want: "wsess789",
		},
		{
			name: "trailing slash",
			path: "/ws/sessions/wsess789/",
			want: "wsess789",
		},
		{
			name: "empty sessionID",
			path: "/ws/sessions/",
			want: "",
		},
		{
			name: "ws root",
			path: "/ws/",
			want: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extract.SessionID(req(http.MethodGet, tt.path, nil))
			if got != tt.want {
				t.Errorf("SessionID(%q) = %q; want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestSessionID_MCP covers the Jam-Session-Id header path.
func TestSessionID_MCP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		headers map[string]string
		want    string
	}{
		{
			name:    "mcp path with header",
			path:    "/mcp",
			headers: map[string]string{"Jam-Session-Id": "mcpsess001"},
			want:    "mcpsess001",
		},
		{
			name:    "arbitrary path with header",
			path:    "/some/unknown/path",
			headers: map[string]string{"Jam-Session-Id": "mcpsess002"},
			want:    "mcpsess002",
		},
		{
			name:    "mcp path without header",
			path:    "/mcp",
			headers: nil,
			want:    "",
		},
		{
			name:    "header takes precedence on unknown path",
			path:    "/unknown",
			headers: map[string]string{"Jam-Session-Id": "h001"},
			want:    "h001",
		},
		{
			// REST paths are parsed before header inspection; if the path
			// yields a session the header is irrelevant (consistent-hash on
			// the path-extracted ID is correct for REST).
			name:    "REST path wins over header",
			path:    "/api/orgs/org1/sessions/pathsess",
			headers: map[string]string{"Jam-Session-Id": "headersess"},
			want:    "pathsess",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extract.SessionID(req(http.MethodPost, tt.path, tt.headers))
			if got != tt.want {
				t.Errorf("SessionID(%q, headers=%v) = %q; want %q", tt.path, tt.headers, got, tt.want)
			}
		})
	}
}

// TestSessionID_SystemRoutes covers routes that must return "".
func TestSessionID_SystemRoutes(t *testing.T) {
	t.Parallel()

	systemPaths := []string{
		"/healthz",
		"/readyz",
		"/metrics",
		"/metrics/",
		"/metrics/prometheus",
		"/auth/magic-link",
		"/auth/oauth/callback",
		"/auth/",
	}

	for _, path := range systemPaths {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			// Even with a header present, system routes must return "".
			headers := map[string]string{"Jam-Session-Id": "should-not-appear"}
			got := extract.SessionID(req(http.MethodGet, path, headers))
			if got != "" {
				t.Errorf("SessionID(%q) = %q; want \"\" (system route must not extract session)", path, got)
			}
		})
	}
}
