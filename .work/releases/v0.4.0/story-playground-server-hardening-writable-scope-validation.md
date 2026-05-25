---
id: story-playground-server-hardening-writable-scope-validation
kind: story
stage: done
tags: [portal, playground, validation]
parent: feature-playground-server-hardening
depends_on: [story-playground-server-hardening-handler-test-coverage]
release_binding: v0.4.0
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

## Implementation notes (2026-05-23)

### `ValidateWritableScope` export

Added to `internal/portal/prereceive/scope.go`. Signature:

    func ValidateWritableScope(raw string) (msg string, ok bool)

Body is the verbatim move from
`internal/portal/sessions/handler.go:443-455`. Added `encoding/json` to
scope.go's imports. The package-internal `parseWritableScope` in
`validate.go:86` stays — different signature `([]string, error)` used by
`Validator.Validate` hot path. The doc comment on the new export
explicitly notes the conceptual overlap and why both exist.

### Call-site updates

- `internal/portal/sessions/handler.go` — deleted local
  `validateWritableScope` and the lingering `encoding/json`+`fmt` uses it
  required. Both call sites (`CreateSession` line 91, `PatchSession` line
  217) now delegate to `prereceive.ValidateWritableScope`. Identical
  behavior — same error code, same envelope shape, same response.

- `internal/portal/playground/handler.go` — added validation block
  immediately after the scope-default fallback (so the default `["**"]`
  also gets compile-checked). On rejection returns
  `openapi.CreatePlaygroundSession400JSONResponse` with
  `error: "session.invalid_writable_scope"`. The generated type
  `CreatePlaygroundSession400JSONResponse` was already present in
  `internal/api/openapi/server.gen.go:6662` — no openapi.yaml change
  required.

### Tests

- `internal/portal/prereceive/scope_test.go::TestValidateWritableScope` —
  new table-driven test with the seven cases from the parent-feature
  acceptance: empty / `[]` / well-formed src/** / multi-glob / non-json
  / json-string-not-array / malformed-glob. Added a small private
  `contains` helper to avoid pulling `strings` into the test file just
  for one substring check.

- `internal/portal/playground/handler_test.go::TestCreatePlaygroundSession_InvalidScope_Returns400` —
  exercises both `["docs/{"]` (malformed glob) and `"not json"` (non-JSON
  payload). Uses the `storetest.Stores(t)` per-dialect harness from the
  unblocking story, so the test runs against SQLite always and Postgres
  when `JAMSESH_TEST_PG_DSN` is set. Identical malformed-glob payload to
  the durable-session test in `scope_validation_test.go` so identical
  inputs prove identical answers.

### Verification

- `go test ./internal/portal/playground/... ./internal/portal/sessions/...
  ./internal/portal/prereceive/...` → all green
- `go build ./...` → clean
- `go vet ./...` → clean
- `go test ./...` → all green across the whole repo (no regressions in
  any other package that imports prereceive or sessions).

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- Test file uses a hand-rolled `contains` helper to dodge a one-call-site
  `strings` import (`internal/portal/prereceive/scope_test.go`). Choice is
  documented inline and reasonable; leaving as-is.

**Notes**:
- `ValidateWritableScope` extraction is faithful — verbatim move from
  `sessions/handler.go:443-455` with empty/`[]`/well-formed/multi-glob/
  non-json/json-string/malformed-glob test coverage in
  `prereceive/scope_test.go::TestValidateWritableScope`.
- Both sessions call sites (`CreateSession` line 91, `PatchSession` line
  217) delegate to the shared helper; envelope shape and error code
  (`session.invalid_writable_scope`) are byte-identical to the pre-extract
  behavior — no breaking change.
- Playground front-door validation closes the poisoned-session DoS path
  described in the original problem statement; validation runs after the
  `["**"]` default is applied so the default itself is compile-checked.
- `playground` has no PATCH endpoint — no second call site to retrofit.
- `prereceive`'s package-internal `parseWritableScope` (different
  signature, used by `Validator.Validate` hot path) is intentionally kept
  alongside the new export; the doc comment on `ValidateWritableScope`
  explains the dual existence.
- `go build ./...`, `go vet ./...`, `go test ./internal/portal/playground/...
  ./internal/portal/sessions/... ./internal/portal/prereceive/...` all
  green at review time.
- Sibling `story-playground-server-hardening-handler-test-coverage`
  remains at stage:review; parent
  `feature-playground-server-hardening` advances only once that sibling
  is also reviewed-and-approved.
