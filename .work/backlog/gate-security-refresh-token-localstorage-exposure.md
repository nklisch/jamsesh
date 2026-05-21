---
id: gate-security-refresh-token-localstorage-exposure
kind: story
stage: drafting
tags: [security]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: security
created: 2026-05-20
updated: 2026-05-20
---

# Refresh token persisted in localStorage, exposed to any XSS

## Severity
Medium

## Domain
Data Protection (token storage)

## Location
`frontend/src/lib/auth.svelte.ts:49-50`

## Evidence
```ts
localStorage.setItem(TOKEN_KEY, access);
localStorage.setItem(REFRESH_KEY, refreshTok);
```

## Remediation direction
Refresh tokens are long-lived credentials. Storing them in `localStorage`
makes them readable by any script with execution context, so any single
XSS — including from a future third-party dep, a Markdown render path, or
a user-controlled-string sink — exfiltrates a refresh token that can be
replayed indefinitely until the backend rotates/revokes it.

Industry guidance (OWASP, IETF OAuth 2.0 for Browser-Based Apps BCP) is
to keep refresh tokens out of JS-accessible storage entirely: deliver
them via an HttpOnly, Secure, SameSite cookie scoped to the refresh
endpoint, or use a Backend-for-Frontend that holds the refresh token
server-side and hands the SPA only short-lived access tokens.

The access token is also exposed but its blast radius is bounded by TTL;
the refresh token is the load-bearing concern.
