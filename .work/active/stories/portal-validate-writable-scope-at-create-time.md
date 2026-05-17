---
id: portal-validate-writable-scope-at-create-time
kind: story
stage: done
tags: [portal, ux]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Validate `writable_scope` at session-create / session-patch time

## Finding

Surfaced while documenting the writable_scope contract in
`docs/SPEC.md` (story `docs-scope-glob-validation-rules`).

`internal/portal/sessions/handler.go > CreateSession` and `> PatchSession`
both accept `req.Body.Scope` as a JSON-encoded string and store it on the
session row without any glob validation. The validation lives in
`internal/portal/prereceive/validate.go:46`, where `CompileScope` runs at
push time:

```go
scope, err := CompileScope(globs)
if err != nil {
    return ValidateResult{}, fmt.Errorf("prereceive: compile scope: %w", err)
}
```

That error path returns `(ValidateResult{}, error)` from `Validate`, which
the smart-http pre-receive subprocess surfaces as a generic git error to
the pusher — not as a structured API response to the session creator. The
poor user experience is:

1. User creates a session with `writable_scope: ["docs/{"]`.
2. Session is created successfully (200 OK).
3. User pushes work to `jam/<session>/<user>/main`.
4. Push fails with an opaque "internal failure" message; the user has no
   way to know the scope itself is malformed.

## Why it matters

`docs/SPEC.md > Writable scope syntax` (newly added) documents that
patterns are validated at push time, with API-time validation tracked as a
backlog item. This is the backlog item.

Closing the gap turns malformed scope from a deferred push-time foot-gun
into an immediate API-time error visible to the session creator.

## Suggested implementation

In `internal/portal/sessions/handler.go`:

1. After unmarshaling `req.Body.Scope` (already JSON-encoded as an array
   of strings on input), call `prereceive.CompileScope(globs)` to validate.
2. On error, return a structured 400 with a code like
   `session.invalid_writable_scope` (matches the existing
   `session.scope_narrowing_rejected` pattern in `PatchSession` at
   `handler.go:181`).
3. Apply the same check in `PatchSession` for the `req.Body.Scope`
   widening path.
4. Update `docs/SPEC.md > Writable scope syntax` to remove the
   "currently does not pre-validate" caveat and the backlog cross-ref.
5. Update `docs/PROTOCOL.md > Error response` to register
   `session.invalid_writable_scope` alongside the existing error codes.

A unit test in `internal/portal/sessions/handler_test.go` (or a new
`scope_validation_test.go`) covers the 400 path with a table of malformed
patterns (`docs/{`, `[abc`, `{a,b`, etc.).

## Acceptance criteria

- [ ] `POST /api/orgs/{orgID}/sessions` with a malformed `writable_scope`
      returns 400 with code `session.invalid_writable_scope`.
- [ ] `PATCH /api/orgs/{orgID}/sessions/{sessionID}` with a malformed
      `writable_scope` returns 400 with the same code (and the session row
      is unchanged).
- [ ] `docs/SPEC.md > Writable scope syntax` updated to reflect the
      new contract (validation at API time, not just push time).
- [ ] `docs/PROTOCOL.md > Error response` lists the new code.
- [ ] Unit test covers the rejection path.

## Notes

The push-time validation in `internal/portal/prereceive/validate.go`
should stay as defence in depth — a malformed pattern that somehow gets
into the database (e.g. a botched migration) is still caught at push
time. This story closes the front door, not the back.

## Implementation notes

- Added a shared `validateWritableScope(raw string) (msg, ok)` helper in
  `internal/portal/sessions/handler.go`. It mirrors the existing
  `parseWritableScope` shape in `internal/portal/prereceive/validate.go` —
  empty string is deny-all and accepted unchanged, otherwise JSON-unmarshal
  to `[]string` then call `prereceive.CompileScope`. On any failure the
  helper returns a human-readable message that becomes the body of a
  `session.invalid_writable_scope` 400.
- Wired the helper into `CreateSession` (immediately after the org-member
  auth check, before the Tx) and into `PatchSession` (alongside the
  existing `session.scope_narrowing_rejected` check, evaluated first so a
  malformed widening attempt surfaces as `invalid_writable_scope` rather
  than masking under the narrowing rule).
- `docs/PROTOCOL.md > HTTP error contract` now lists
  `session.invalid_writable_scope` and `session.scope_narrowing_rejected`
  as 400 business codes.
- `docs/SPEC.md > Writable scope syntax` rewritten to describe the
  two-layer (API + push) validation contract. Removed the
  "currently does not pre-validate" caveat and the backlog cross-ref.
- New table-driven tests in
  `internal/portal/sessions/scope_validation_test.go` cover both
  `CreateSession` and `PatchSession` with malformed (`docs/{`, `[abc`,
  `{a,b`), well-formed, and empty/deny-all payloads. The patch test also
  asserts the session row is unchanged on rejection. All 10 sub-tests
  pass; `go build ./...` clean.

## Review

**Verdict:** Approve.

- Helper `validateWritableScope` mirrors `prereceive.parseWritableScope`
  (empty → deny-all, JSON-unmarshal then `prereceive.CompileScope`) and
  reuses the production compiler, so API-time and push-time accept
  exactly the same set of globs. No duplicated validation logic to drift.
- `CreateSession` invokes the helper after the org-member auth check and
  before the Tx — malformed payloads are rejected before any DB write.
- `PatchSession` invokes the helper before `isScopeNarrowing`, so a
  malformed widening surfaces as `session.invalid_writable_scope` rather
  than masking under `session.scope_narrowing_rejected`. Correct ordering.
- Error envelope shape matches the existing `scope_narrowing_rejected`
  pattern (`openapi.ErrorEnvelope{Error, Message}` via the typed 400
  response). Patch test re-fetches the row and confirms no mutation on
  rejection.
- `docs/SPEC.md > Writable scope syntax` and `docs/PROTOCOL.md > HTTP
  error contract` are rolled-forward in place — no "previously was"
  prose, the two-layer contract reads as the current truth.
- `go test ./internal/portal/sessions/... ./internal/portal/prereceive/...`
  passes; `go build ./...` clean.

**Findings:** 0 blockers, 0 important, 0 nits.
