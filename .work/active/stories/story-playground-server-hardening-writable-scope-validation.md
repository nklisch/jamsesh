---
id: story-playground-server-hardening-writable-scope-validation
kind: story
stage: implementing
tags: [portal, playground, validation]
parent: feature-playground-server-hardening
depends_on: [story-playground-server-hardening-handler-test-coverage]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# CreatePlaygroundSession stores writable_scope without validation

## Origin

Filed from review of
`story-epic-ephemeral-playground-session-lifecycle-rest-endpoints`.

## Problem

`internal/portal/playground/handler.go:96-101` accepts the request
body's `Scope` field and stores it verbatim (or defaults to `["**"]`).
The durable session handler at
`internal/portal/sessions/handler.go:91-96` validates the writable_scope
JSON via `validateWritableScope` before insert and returns 400
`session.invalid_writable_scope` for malformed input.

The playground handler skips this validation entirely. A caller can
supply `scope: "not json"` and create the session successfully. The
malformed scope only surfaces later — at pre-receive time, when the
first `git push` hits the session and `prereceive.Validate` fails to
parse the stored scope (per `validate.go:43`, a malformed
writable_scope returns an error rather than a rejection, which becomes
a 500-ish git protocol error).

## Impact

- Inconsistent input validation between playground and durable session
  creation paths.
- A caller-supplied invalid scope leaves a poisoned session that fails
  every push until destruction.
- Easy to DoS the playground org's repo storage by creating many
  poison-scope sessions whose disk space sticks around until destruction
  sweep.

## Fix

Call `validateWritableScope(scope)` (or factor it into a shared
package — currently it lives in `internal/portal/sessions/`) before
the session insert in `CreatePlaygroundSession`. Return
`CreatePlaygroundSession400JSONResponse` with
`error: session.invalid_writable_scope` on failure.

## Acceptance

- `POST /api/playground/sessions` with `scope: "not json"` returns 400
  `session.invalid_writable_scope`.
- `POST /api/playground/sessions` with `scope: "[\"src/**\"]"` succeeds.
- `validateWritableScope` lives in `internal/portal/prereceive/` as
  exported `ValidateWritableScope(raw string) (msg string, ok bool)`
  and is imported by both `playground` and `sessions` handlers.

## Design

Full spec is in the parent feature body under `## Implementation Units`
→ Unit 2 (`ValidateWritableScope` extraction + playground call site).
Highlights:

- **Home**: extend `internal/portal/prereceive/scope.go` (NOT a new
  `sessionscope` package — see `## Design decisions` in the feature).
  Function signature: `ValidateWritableScope(raw string) (msg string, ok bool)`.
- **Move source**: verbatim from
  `internal/portal/sessions/handler.go:443-455`; the existing
  `prereceive.parseWritableScope` (unexported, line 86 of `validate.go`)
  stays — different signature, used by the `Validator.Validate` hot path.
- **Sessions handler**: delete local helper, update both call sites
  (`handler.go:91` create + `:217` patch) to delegate.
- **Playground handler**: add validation block immediately after
  scope-defaulting at `handler.go:98-101`, BEFORE the TX. Returns
  `openapi.CreatePlaygroundSession400JSONResponse` with
  `error: "session.invalid_writable_scope"`.
- **OpenAPI dependency**: check
  `internal/api/openapi/server.gen.go` for
  `CreatePlaygroundSession400JSONResponse`. If absent, add a `400`
  response to the operation in `docs/openapi.yaml` (mirror the
  `CreateSession` 400 shape) and `go generate ./...`.
- **New unit test**: `TestValidateWritableScope` in
  `internal/portal/prereceive/scope_test.go` (table with
  empty/`[]`/well-formed/`not json`/malformed-glob cases).
- **`depends_on`**: `story-playground-server-hardening-handler-test-coverage`
  must land first so the new playground test
  (`TestCreatePlaygroundSession_InvalidScope_Returns400`) can use the
  per-dialect `storetest.Stores(t)` harness from the start.
