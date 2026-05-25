---
id: gate-security-playground-internal-sql-errors-surface-to-anon
kind: story
stage: review
tags: [security, portal, playground, error-handling]
parent: feature-playground-hardening
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-24
updated: 2026-05-25
---

# `WrapDBIfTransient`/`fmt.Errorf` chains for playground store failures may surface internal SQL error strings to anonymous callers

## Severity
Low

## Domain
Error Handling & Logging

## Location
`internal/portal/playground/handler.go:146-148, 154-156, 165-167, 213-215, 238-240, 270-273, 280-283, 333-335, 350-352, 358-361, 397-399`

## Evidence
```go
if txErr != nil {
    return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: create session tx: %w", txErr))
}
```

All playground REST endpoints are unauthenticated or
anonymous-bearer-authenticated, and `httperr.WriteFromError` (router
pipeline) emits the wrapped error's `Message` in the response envelope on
non-classified failures. A transient DB error (e.g. SQLite
`database is locked`, pgx connection-reset) currently surfaces verbose
internal text to anonymous callers. `docs/SECURITY.md` does not address this
explicitly.

## Remediation direction
Audit the `httperr.WriteFromError` path to ensure the response envelope
strips internal error chains for anonymous endpoints (return a generic
`internal` 500), and reserve the wrapped chain for the structured access log
only.

## Implementation notes

- Audit confirmed: the `WrapDBIfTransient` + `httperr.WriteFromError`
  pipeline already maps transient DB failures to the canonical
  `dep.db_unavailable` envelope with the message
  `"database is currently unavailable"` — no internal SQL string leaks.
  Story scope is purely a test pin.
- Added `TestCreatePlaygroundSession_DBError_DoesNotLeakSQLDetail` in
  `internal/portal/playground/handler_test.go`. The test:
  - injects an `errors.New("sql: database is locked")` failure on
    `AddSessionMember` via the existing `failingAddSessionMemberStore`;
  - asserts response is 503;
  - asserts the response body does NOT contain `"sql:"`,
    `"database is locked"`, or `"AddSessionMember"`;
  - asserts the response body DOES contain `"dep.db_unavailable"` and
    `"database is currently unavailable"`.
- The injected SQL string still appears in the structured log (per the
  `code=dep.db_unavailable status=503 err="...sql: database is locked"`
  pattern) — that's expected and desirable for operator diagnosis. The
  test confirms it never leaks to anonymous HTTP responses.
- No production code changes needed — pipeline already enforces the
  invariant; the test pins it against regression.

Verified: `go test ./internal/portal/playground/... -count 1 -run DBError_DoesNotLeakSQLDetail` passes.
