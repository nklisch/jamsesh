---
id: gate-security-oauth-callback-log-scrubbing
kind: story
stage: done
tags: [security, portal, logging, auth]
parent: feature-server-secret-log-hygiene
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-24
updated: 2026-05-25
---

# GitHub OAuth callback may surface provider-error envelopes / log `code`+`state` query params

## Severity
Low

## Domain
Secrets & Configuration

## Location
`internal/portal/auth/oauth.go:162-174`

## Evidence
```go
ghIdentity, err := provider.Exchange(ctx, code, stateRow.RedirectURI)
if err != nil {
    // ...
    return nil, deperr.WrapOAuthProvider(
        fmt.Errorf("oauth callback: exchange: %w", err))
}
```

`deperr.WrapOAuthProvider` ultimately routes through `httperr.WriteFromError`
for the response envelope, but the wrapping error chain is also logged via
slog in the access logger. If the provider response embeds the `code` (some
non-standard providers do for diagnostics) or query-string fragments, that
lands in the structured access log alongside the request URL (which contains
`code=` and `state=` for the OAuth callback). The portal does not currently
strip `code`/`state` from logged URLs.

## Status: already remediated

Investigation during feature design (2026-05-25) confirmed this is already
fixed:

1. `POST /api/auth/oauth/callback` receives `code` and `state` in the JSON
   request body — not in the URL query string. The access logger logs
   `r.URL.RawQuery`, which is empty for this endpoint.
2. `internal/portal/logging/redact.go` already lists `"code"` and `"state"`
   in `sensitiveParams`, and `RedactQueryTokens` is called on every access-log
   line via the `logging.Access` middleware.
3. The error chain from `deperr.WrapOAuthProvider` wraps a Go `error` value
   (from the OAuth client library); it does not include raw provider HTTP
   response bodies.

## Remaining work

Add a targeted regression test that pins the invariant — ensures `code` and
`state` are covered by `RedactQueryTokens` in the access-log middleware — so
a future refactor cannot silently remove them.

**File**: `internal/portal/logging/logging_test.go`

```go
// TestAccessMiddlewareRedactsOAuthQueryParams pins the invariant that the
// access-log middleware redacts OAuth code/state params from the logged
// query field. The OauthCallback endpoint is a POST with JSON body (no query
// params in production), but this test guards against any future path that
// routes code/state through the URL.
func TestAccessMiddlewareRedactsOAuthQueryParams(t *testing.T) {
    const secretCode = "provider_auth_code_secret"
    const secretState = "csrf_nonce_secret"

    var buf bytes.Buffer
    logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
    slog.SetDefault(logger)

    inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusBadRequest) // simulated bad-grant response
    })

    w := httptest.NewRecorder()
    r := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/callback?code="+secretCode+"&state="+secretState, nil)

    logging.Access(nil)(inner).ServeHTTP(w, r)

    line := strings.TrimSpace(buf.String())
    if strings.Contains(line, secretCode) {
        t.Errorf("access log leaks OAuth code %q: %s", secretCode, line)
    }
    if strings.Contains(line, secretState) {
        t.Errorf("access log leaks OAuth state %q: %s", secretState, line)
    }

    var entry map[string]any
    if err := json.Unmarshal([]byte(line), &entry); err != nil {
        t.Fatalf("decode log line: %v", err)
    }
    q, _ := entry["query"].(string)
    if !strings.Contains(q, "<redacted>") {
        t.Errorf("query field %q does not contain <redacted>", q)
    }
}
```

## Acceptance Criteria
- [ ] `TestAccessMiddlewareRedactsOAuthQueryParams` is added to `internal/portal/logging/logging_test.go`
- [ ] Test passes: neither `code` nor `state` values appear in log output
- [ ] `<redacted>` sentinel appears in the logged `query` field for both params

## Implementation notes

- Audit confirmed `code` and `state` are already in `sensitiveParams` in
  `internal/portal/logging/redact.go`; the access middleware already routes
  the query through `RedactQueryTokens`. Today the callback is POST + JSON
  body so `code`/`state` never reach the URL — but a future regression
  (route refactor, mis-route, GET fallback) must not leak.
- Added `TestAccessMiddlewareRedactsOAuthQueryParams` in
  `internal/portal/logging/logging_test.go`. The test:
  - constructs a GET request to `/api/auth/oauth/callback?code=…&state=…`
    with secret-shaped values;
  - runs it through `logging.Access(nil)`;
  - decodes the structured log line;
  - asserts neither `code` nor `state` raw value appears in `query`,
    the key names remain (`code=`, `state=`), and the `<redacted>`
    sentinel is present.

Verified: `go test ./internal/portal/logging/... -count 1` passes; the new
test alone passes via `-run RedactsOAuth`.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Defense-in-depth regression test for an already-remediated finding. Test asserts: (a) raw `code` and `state` values absent from log, (b) key names retained for debug utility, (c) `<redacted>` sentinel present. Sound pattern.
