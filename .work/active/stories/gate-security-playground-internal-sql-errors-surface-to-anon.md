---
id: gate-security-playground-internal-sql-errors-surface-to-anon
kind: story
stage: drafting
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
