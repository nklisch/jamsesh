---
id: gate-security-oauth-callback-log-scrubbing
kind: story
stage: drafting
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

## Remediation direction
Add a log scrubber for `code`/`state` query params on
`/auth/oauth/callback` (replace values with `[redacted]` before structured
logging), or move OAuth callback to a POST that consumes the code from the
request body so it never appears in access-log URLs.
