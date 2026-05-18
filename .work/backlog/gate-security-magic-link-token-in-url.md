---
id: gate-security-magic-link-token-in-url
kind: story
stage: backlog
tags: [security, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# Magic-link / invite-accept token in URL query string leaks via Referer / history / proxy logs

## Severity
Low

## Domain
Data Protection

## Location
`internal/portal/auth/magic_link.go:107`,
`internal/portal/accounts/orgs.go:191`,
`internal/portal/sessions/invites.go:115`

## Evidence
```go
magicURL := h.portalURL + "/auth/magic-link?token=" + raw
```

Same pattern in `accounts/orgs.go:191`
(`/orgs/.../invites/.../accept?token=...`) and `sessions/invites.go:115`.
URLs with tokens persist in browser history, get forwarded as `Referer:`
when the magic-link landing page links anywhere offsite, and appear in
upstream access logs. Tokens are single-use and short-lived, so impact
is bounded.

## Remediation direction
Land the user on a token-less landing page and POST the token from JS to
the exchange endpoint instead of putting it in the query, or accept the
token in a hash fragment (`#token=...`) which is not sent to the server
or logged.
