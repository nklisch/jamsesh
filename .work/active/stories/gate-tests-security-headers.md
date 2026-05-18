---
id: gate-tests-security-headers
kind: story
stage: implementing
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
