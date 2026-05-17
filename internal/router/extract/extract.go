// Package extract provides session-ID extraction from HTTP requests.
//
// The router needs to know which session a request belongs to so it can
// forward the request to the correct portal pod. This package covers all
// URL shapes used by the portal:
//
//   - REST:  /api/orgs/{orgID}/sessions/{sessionID}/...
//   - Git:   /git/{orgID}/{sessionID}.git/...
//   - WS:    /ws/sessions/{sessionID}
//   - MCP:   Jam-Session-Id header (any path)
//
// Requests for /healthz, /readyz, /metrics, and /auth/* do not belong to a
// session; SessionID returns "" for those so the caller can use its fallback
// (round-robin) routing strategy.
package extract

import (
	"net/http"
	"strings"
)

// jamSessionHeader is the header MCP clients emit to identify their session.
const jamSessionHeader = "Jam-Session-Id"

// SessionID returns the session id encoded in r, or "" if none could be
// extracted. The extraction order is:
//
//  1. Path-based: REST, Git, WS — each has a fixed URL shape.
//  2. Header-based: Jam-Session-Id (for MCP connections).
//
// Non-session paths (/healthz, /readyz, /metrics, /auth/*) return "".
func SessionID(r *http.Request) string {
	path := r.URL.Path

	// Strip a single trailing slash so /ws/sessions/abc/ is treated the
	// same as /ws/sessions/abc.  We don't use path.Clean here because that
	// would also collapse //, which we don't need.
	if len(path) > 1 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}

	switch {
	// ── Non-session system routes ──────────────────────────────────────────
	case path == "/healthz",
		path == "/readyz",
		path == "/metrics",
		strings.HasPrefix(path, "/metrics/"),
		path == "/auth",
		strings.HasPrefix(path, "/auth/"):
		return ""

	// ── REST: /api/orgs/{orgID}/sessions/{sessionID}[/...] ────────────────
	case strings.HasPrefix(path, "/api/"):
		return extractRESTSessionID(path)

	// ── WebSocket: /ws/sessions/{sessionID} ───────────────────────────────
	case strings.HasPrefix(path, "/ws/sessions/"):
		return extractWSSessionID(path)

	// ── Git smart-HTTP: /git/{orgID}/{sessionID}.git[/...] ────────────────
	case strings.HasPrefix(path, "/git/"):
		return extractGitSessionID(path)
	}

	// ── MCP: any path, identified by header ───────────────────────────────
	if v := r.Header.Get(jamSessionHeader); v != "" {
		return v
	}

	return ""
}

// extractRESTSessionID parses /api/orgs/{orgID}/sessions/{sessionID}[/...].
// Returns "" when the path doesn't match the expected shape.
func extractRESTSessionID(path string) string {
	// Expected segments after stripping leading slash:
	//   api / orgs / {orgID} / sessions / {sessionID} [/ ...]
	// Index:  0      1          2           3              4
	const prefix = "/api/orgs/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):] // "{orgID}/sessions/{sessionID}[/...]"

	// Find the orgID segment.
	slashAfterOrg := strings.IndexByte(rest, '/')
	if slashAfterOrg < 0 {
		return ""
	}
	rest = rest[slashAfterOrg+1:] // "sessions/{sessionID}[/...]"

	const sessionsKey = "sessions/"
	if !strings.HasPrefix(rest, sessionsKey) {
		return ""
	}
	rest = rest[len(sessionsKey):] // "{sessionID}[/...]"

	if rest == "" {
		return ""
	}

	// sessionID ends at the next slash (if any).
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		return rest[:idx]
	}
	return rest
}

// extractWSSessionID parses /ws/sessions/{sessionID}.
// Returns "" when the segment after /ws/sessions/ is empty.
func extractWSSessionID(path string) string {
	const prefix = "/ws/sessions/"
	id := path[len(prefix):]
	if id == "" {
		return ""
	}
	// There should not be further path segments, but guard anyway.
	if idx := strings.IndexByte(id, '/'); idx >= 0 {
		return id[:idx]
	}
	return id
}

// extractGitSessionID parses /git/{orgID}/{sessionID}.git[/...].
// Returns "" when the path doesn't have enough segments or the session
// segment doesn't end in ".git".
func extractGitSessionID(path string) string {
	const prefix = "/git/"
	rest := path[len(prefix):] // "{orgID}/{sessionID}.git[/...]"

	// Skip the orgID segment.
	slashAfterOrg := strings.IndexByte(rest, '/')
	if slashAfterOrg < 0 {
		return ""
	}
	rest = rest[slashAfterOrg+1:] // "{sessionID}.git[/...]"

	// The sessionID segment ends before the next slash or end-of-string.
	var rawSeg string
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		rawSeg = rest[:idx]
	} else {
		rawSeg = rest
	}

	// Segment must end in ".git".
	const dotGit = ".git"
	if !strings.HasSuffix(rawSeg, dotGit) {
		return ""
	}
	id := rawSeg[:len(rawSeg)-len(dotGit)]
	if id == "" {
		return ""
	}
	return id
}
