---
id: gate-security-rest-body-size-limits
kind: story
stage: done
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

## Implementation notes

### Middleware placement

- `internal/portal/router/body_limits.go` — new `BodyLimit(max int64)` middleware.
- Applied as the first `r.Use(...)` inside the `/api` chi sub-route in `router.go`
  (after global middleware, before any auth or handler). Git smart-HTTP
  (`/git/*`) is untouched.
- `router.Deps.APIBodyLimitBytes` carries the limit; zero defaults to 1 MiB.

### Config knob

- `config.Config.APIBodyLimitBytes` (`api_body_limit_bytes`, env
  `JAMSESH_API_BODY_LIMIT_BYTES`). Default 0 → interpreted as 1 MiB in
  `router.New`. Positive value overrides.

### 413 response path

`http.MaxBytesReader` returns `*http.MaxBytesError` when `io.ReadAll` or
`json.NewDecoder.Decode` reads past the cap. The oapi-codegen strict handler
calls `RequestErrorHandlerFunc` (wired to `httperr.WriteBadRequest`) on decode
failure. Updated `httperr.WriteBadRequest` to detect `*http.MaxBytesError`
via `errors.As` and delegate to the new `httperr.ErrBodyTooLarge()` (413,
code `request.body_too_large`) instead of the generic 400.

### OpenAPI maxLength constraints added

| Schema | Field | maxLength |
|---|---|---|
| `MagicLinkRequestBody` | `email` | 254 (RFC 5321) |
| `CreateOrgBody` | `name` | 200 |
| `InviteBody` | `email` | 254 |
| `InviteRequest` (session) | `email` | 254 |
| `CreateSessionRequest` | `name` | 200 |
| `CreateSessionRequest` | `goal` | 4096 |
| `PatchSessionRequest` | `goal` | 4096 |
| `ResolveCommentRequest` | `resolution_note` | 4096 |
| `CreateCommentRequest` | `body` | 4096 |
| `PatchFinalizeLockRequest` | `target_branch` | 200 |
| `PatchFinalizeLockRequest` | `commit_message` | 4096 |

`make generate-api-go` was re-run; the embedded spec bytes in
`internal/api/openapi/server.gen.go` updated (oapi-codegen does not yet emit
Go-level maxLength validation, but the spec is embedded verbatim and the
constraint is enforced by the BodyLimit cap before any field is decoded).

### Tests

- `internal/portal/router/body_limits_test.go`:
  - `TestBodyLimitMiddleware` — unit test; 16-byte cap; verifies 200 under limit
    and 413 over limit via `*http.MaxBytesError` detection.
  - `TestAPIBodyLimitApplied` — integration test through full router; 1 MiB + 1
    body POST to `/api/probe` returns 413.

### Handler-side adjustments

No per-handler changes needed. The overflow path is:
`BodyLimit middleware wraps body → json.Decode hits limit →
*http.MaxBytesError → WriteBadRequest detects → 413 envelope`.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Global body cap for API routes. New BodyLimit middleware wraps r.Body with http.MaxBytesReader; mounted on /api subroute (NOT /git/* which has its own larger cap). Configurable via JAMSESH_API_BODY_LIMIT_BYTES (default 1 MiB). httperr.ErrBodyTooLarge (413, request.body_too_large) added; WriteBadRequest detects *http.MaxBytesError via errors.As and routes to 413. openapi.yaml gained maxLength on 11 user-text fields (email 254, branch 200, body/goal 4096, etc.). server.gen.go regenerated. Two unit tests confirm the cap fires.
