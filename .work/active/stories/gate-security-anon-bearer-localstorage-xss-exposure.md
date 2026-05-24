---
id: gate-security-anon-bearer-localstorage-xss-exposure
kind: story
stage: drafting
tags: [security, ui, tokens, data-protection]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: security
created: 2026-05-24
updated: 2026-05-24
---

# Anonymous bearer tokens persisted to localStorage have no per-tab isolation and no XSS protection beyond CSP `script-src 'self'`

## Severity
Medium

## Domain
Data Protection

## Location
`frontend/src/lib/auth.svelte.ts:23-31, 67-76`

## Evidence
```ts
const TOKEN_KEY = 'jamsesh.token';
const REFRESH_KEY = 'jamsesh.refresh';

let _token = $state<string | null>(
  typeof localStorage !== 'undefined' ? localStorage.getItem(TOKEN_KEY) : null,
);
// ...
setTokens(access: string, refreshTok: string): void {
    localStorage.setItem(TOKEN_KEY, access);
    localStorage.setItem(REFRESH_KEY, refreshTok);
},
```

The `PlaygroundContext` (`auth.svelte.ts:17-21`) holds the anonymous bearer
that grants session-scoped git-push and REST access. Although the current
bundle stores it only in-memory (`_playgroundContext`), durable tokens
(access + 30-day refresh) live in `localStorage`, where any XSS — including a
future inline-script regression in the Svelte bundle — exfiltrates both halves
of the token pair. `docs/SECURITY.md:162-180` acknowledges DB-resident refresh
tokens as a breach exposure, but does not document the additional client-side
reach of `localStorage`. The default CSP in
`internal/portal/router/security_headers.go:30-41` allows
`style-src 'unsafe-inline'`, increasing the XSS-mitigation surface area.

## Remediation direction
Move refresh tokens to an httpOnly Secure cookie (or document the
localStorage decision explicitly in `docs/SECURITY.md` and accept the
residual XSS-exfil risk); add a CSP `report-uri` / `report-to` so
inline-script regressions surface.
