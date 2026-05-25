---
id: bug-csp-report-endpoint-not-wired
kind: story
stage: done
tags: [security, portal, csp]
parent: feature-spa-bootstrap-hygiene
depends_on: []
release_binding: null
created: 2026-05-24
updated: 2026-05-25
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

## Implementation notes

- `internal/portal/router/router.go`:
  - Added `log/slog` import and `cspReportMaxBody = 64 * 1024` constant.
  - Registered `r.Post("/_csp-report", cspReport)` immediately after
    `/healthz` (public, unauthenticated, top-level).
  - Added package-private `cspReport` handler:
    - Wraps `r.Body` in `http.MaxBytesReader(w, r.Body, cspReportMaxBody)`.
    - Decodes JSON; on parse error, logs `csp_violation` with `parse_err`
      and returns 204 (no 4xx — browsers should not retry).
    - On success, logs `csp_violation` with the full `report` payload at
      WARN level and returns 204.
- Five new tests in `internal/portal/router/router_test.go`:
  - `TestCSPReport_ValidJSON_Returns204` (also verifies the slog line)
  - `TestCSPReport_MalformedBody_Still204`
  - `TestCSPReport_WrongMethodReturns405` (chi MethodNotAllowed envelope)
  - `TestCSPReport_NoAuthHeader_Returns204`
  - `TestCSPReport_OversizedBody_Returns204` (128 KiB body → MaxBytesReader
    truncates → 204)

Verified: `go test ./internal/portal/router/... -count 1` passes.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: `http.MaxBytesReader` bounds memory; 64 KiB cap matches the comment's rationale. Always-204 prevents retry storms from misbehaving browsers/scanners; parse-error path still logs the `parse_err` for operator diagnosis. Public/unauthenticated is correct (browsers post without credentials). Tests cover valid, malformed, wrong-method, no-auth, and oversized-body cases.
