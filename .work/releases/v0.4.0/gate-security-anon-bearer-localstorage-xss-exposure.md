---
id: gate-security-anon-bearer-localstorage-xss-exposure
kind: story
stage: done
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

## Implementation notes

**Direction taken:** Direction 2 (document + CSP report-uri), not Direction 1
(move tokens to httpOnly cookies). Reasons: refresh tokens are already in
`localStorage` as a documented design decision; moving to cookies requires
auth-flow rework out of scope for a medium-severity mid-release finding.

**`docs/SECURITY.md` — new section "Client-side token storage and XSS residual risk"**
inserted immediately before "Supply chain and integrity". The section:
- Documents that refresh tokens are persisted to `localStorage` (`jamsesh.token`,
  `jamsesh.refresh` keys) and explicitly calls out the XSS-exfil risk and
  residual-risk acceptance rationale.
- Confirms that the playground anonymous bearer (`_playgroundContext`) is held
  in-memory only — NOT in `localStorage` — and that a page reload intentionally
  drops it.
- References the CSP report endpoint (see below) as the regression-detection
  mechanism.

**`internal/portal/router/security_headers.go` — `Content-Security-Policy-Report-Only` header**
A new `cspReportOnly()` helper mirrors `defaultCSP()` and appends
`report-uri /_csp-report`. The `SecurityHeaders` middleware now emits this as a
second header alongside the enforced `Content-Security-Policy`. The
`Report-Only` mode means violations are reported but never block the page —
appropriate for a regression-detection signal.

**`/_csp-report` endpoint:** The route is a placeholder; browsers will POST
reports there and receive 404 (harmless — the reports still appear in browser
devtools). A backlog item `bug-csp-report-endpoint-not-wired` has been parked
for wiring an actual log-and-204 receiver.

**Verification:** `go build ./...` clean; `go test ./internal/portal/router/...`
clean (existing security-headers tests pass); `npm run check` 0 errors;
`npm test` 693/693 passing.

## Review notes

Spawned `review-csp-report-only-header-test-coverage` (Important) — the new `Content-Security-Policy-Report-Only` header is emitted but `security_headers_test.go` only asserts the enforced `Content-Security-Policy`. A regression that drops the report-only header would not be caught.

CSP `Content-Security-Policy-Report-Only` header confirmed wired in `security_headers.go:74`. SECURITY.md paragraph "Client-side token storage and XSS residual risk" is accurate, covers durable + playground stores, names the keys, and acknowledges the residual XSS risk + the unwired report endpoint (tracked in backlog as `bug-csp-report-endpoint-not-wired`).
