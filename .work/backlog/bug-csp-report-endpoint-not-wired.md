---
id: bug-csp-report-endpoint-not-wired
kind: story
stage: drafting
tags: [security, portal, csp]
parent: null
depends_on: []
release_binding: null
created: 2026-05-24
updated: 2026-05-24
---

# CSP report-uri `/_csp-report` endpoint is not wired to a real receiver

## Context

`internal/portal/router/security_headers.go` emits a
`Content-Security-Policy-Report-Only` header with `report-uri /_csp-report`
on every response (added in the `gate-security-anon-bearer-localstorage-xss-exposure`
security hardening story). This sends browser CSP violation reports to
`/_csp-report`, but no route handles that path — browsers receive a 404,
and the report body is silently dropped.

## Impact

CSP violation reports (e.g., inline-script regressions in the Svelte bundle)
are sent by the browser but not captured. The Report-Only header still
surfaces violations in browser devtools and error consoles, but no server-side
aggregation or alerting is possible until the endpoint is wired.

## Remediation

Add a `POST /_csp-report` route to `internal/portal/router/router.go` that:
1. Reads and logs the JSON report body (at `slog.WarnContext` level with key
   `csp_violation`).
2. Returns `204 No Content` (browsers do not use the response body).

Optionally: forward the report to an external observability sink (Sentry,
a structured log collector, etc.) once basic wiring is confirmed.

The endpoint should not require authentication — browsers POST CSP reports
without credentials.
