---
id: story-playground-server-hardening-writable-scope-validation
kind: story
stage: implementing
tags: [portal, playground, validation]
parent: feature-playground-server-hardening
depends_on: []
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
- Refactor `validateWritableScope` into a package importable from both
  `playground` and `sessions` (e.g. `internal/portal/sessionscope`) so
  the two handlers share one implementation.
