---
id: gate-tests-security-headers
kind: story
stage: review
tags: [testing, security, portal]
parent: null
depends_on: [gate-security-security-headers-middleware]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# Security-headers middleware entirely unspecified-by-test

## Priority
Critical

## Spec reference
Item: `gate-security-security-headers-middleware`
Acceptance criterion: middleware sets CSP, X-Content-Type-Options=nosniff,
X-Frame-Options=DENY, HSTS (when HTTPS), Referrer-Policy=no-referrer.

## Gap type
missing test for valid partition. `grep -rn 'Content-Security-Policy\|X-Frame-Options\|Strict-Transport-Security' internal/portal/` returns zero hits.

## Suggested test
```go
// TestRouter_SecurityHeadersOnAllResponses
// Issue GET /healthz, GET /api/orgs/me, GET / (SPA).
// Assert each response has CSP, X-Content-Type-Options=nosniff, X-Frame-Options=DENY,
// Referrer-Policy=no-referrer. Assert HSTS present when tls.mode in {native, proxy-https}.
```

## Test location (suggested)
`internal/portal/router/router_test.go`

## Implementation notes

New file: `internal/portal/router/security_headers_test.go` (package `router_test`).

Two top-level test functions, eight subtests total:

**`TestSecurityHeaders_Middleware`** — exercises `SecurityHeaders(...)` directly via a stub handler, without the full router:
- `baseline headers present when HSTS disabled`: asserts `X-Content-Type-Options=nosniff`, `X-Frame-Options=DENY`, `Referrer-Policy=no-referrer`, non-empty CSP, absent `Strict-Transport-Security`.
- `HSTS enabled sets correct HSTS header`: asserts `Strict-Transport-Security: max-age=31536000; includeSubDomains`.
- `default CSP contains critical directives`: spot-checks `default-src 'self'`, `frame-ancestors 'none'`, `object-src 'none'`, `base-uri 'none'` via `strings.Contains` (not literal-string match).
- `caller can override CSP`: passes a custom CSP string; asserts it is used verbatim.

**`TestSecurityHeaders_RouterIntegration`** — constructs `router.New(Deps{...})` and hits `GET /healthz` with `httptest.NewRecorder`, covering all four HSTS-gating cases:
- `TLSMode ""` → no HSTS
- `TLSMode "native"` → HSTS present
- `TLSMode "behind_proxy" + TrustProxyHeaders=true` → HSTS present
- `TLSMode "behind_proxy" + TrustProxyHeaders=false` → no HSTS

All production middleware was found correct; no production bugs surfaced.
