---
id: review-csp-report-only-header-test-coverage
kind: story
stage: implementing
tags: [testing, security]
parent: null
depends_on: []
release_binding: null
created: 2026-05-24
updated: 2026-05-24
---

# Add test coverage for the new `Content-Security-Policy-Report-Only` header

## Origin
Spawned during review of `gate-security-anon-bearer-localstorage-xss-exposure`.
The fix added a second CSP header but no test asserts its presence or shape.

## Issue
`internal/portal/router/security_headers.go` now emits
`Content-Security-Policy-Report-Only` alongside the enforced
`Content-Security-Policy`. `security_headers_test.go` covers only the
enforced header — a future regression that drops the report-only header
would not fail the test suite.

## Fix
Add at least one test in `security_headers_test.go` asserting:
- the `Content-Security-Policy-Report-Only` header is present on a default
  response,
- it ends with `report-uri /_csp-report`,
- its body otherwise mirrors `defaultCSP()` (or at minimum contains
  `script-src 'self'`).
