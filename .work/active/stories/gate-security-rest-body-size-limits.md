---
id: gate-security-rest-body-size-limits
kind: story
stage: implementing
tags: [security, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# No body-size limits on REST endpoints (only git smart-HTTP)

## Severity
Medium

## Domain
API Security

## Location
`internal/portal/router/router.go` (no `MaxBytesReader` wrap), e.g.
`internal/portal/comments/handlers.go:30-100`

## Evidence
Only `internal/portal/githttp/receive_pack.go:57` wraps `r.Body` with
`http.MaxBytesReader`. Strict-server-generated POST handlers
(CreateComment, RequestMagicLink, PatchFinalizeLock, all session APIs)
read whole JSON bodies without a cap. Combined with the absence of
length constraints in `docs/openapi.yaml` (`grep maxLength` returns
nothing), an attacker can POST hundreds-of-megabyte comment bodies that
hit the DB and propagate over WebSocket to every session viewer.

## Remediation direction
Add a global middleware that wraps `r.Body` with
`http.MaxBytesReader(w, r.Body, 1<<20)` (or similar) before the
strict-handler decode step, and add `maxLength` constraints on user-text
fields in `docs/openapi.yaml` so oapi-codegen validation enforces them
too.
