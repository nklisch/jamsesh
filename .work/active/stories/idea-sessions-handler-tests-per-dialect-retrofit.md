---
id: idea-sessions-handler-tests-per-dialect-retrofit
kind: story
stage: implementing
tags: [portal, sessions, testing]
parent: feature-test-spec-drift-and-coverage
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-25
---

# Retrofit sessions handler tests to run under both SQLite and Postgres

## Origin

Documented as a deferred follow-up during implementation of
`story-playground-server-hardening-handler-test-coverage`. The story's
original design called for per-dialect wrapping of every sessions handler
test alongside the playground retrofit. Deferred because the mechanical
scope (65+ tests across 7 files) was tangential to the playground
coverage-gap fix that drove the story.

## Problem

`internal/portal/sessions/handler_test.go` and its 6 sibling test files
(`clock_test.go`, `files_test.go`, `listing_state_test.go`,
`scope_validation_test.go`, `refmodes_test.go`, `invites_test.go`) all
consume `openStore(t)` which currently returns SQLite only (via
`storetest.Stores(t)[0].Open(t)`). The sessions adapter for queries
unique to the durable-session path is therefore never exercised against
Postgres in handler-level tests.

The single-source-of-truth fix was applied (delegating to `storetest`
instead of inlining `db.Open` locally), but the per-dialect `t.Run`
wrapping was not.

## Fix

Mirror the pattern applied in
`internal/portal/playground/handler_test.go` (commit f59e45f):

1. Refactor `newTestEnv` / `newTestEnvWithStore` to take an explicit
   `store.Store` argument instead of opening one internally.
2. Wrap every `func TestX(t *testing.T)` body in:
   ```go
   for _, h := range storetest.Stores(t) {
       h := h
       t.Run(h.Name, func(t *testing.T) {
           env := newTestEnv(t, h.Open(t))
           // ... existing body unchanged ...
       })
   }
   ```
3. Keep a `newTestEnvSQLite` shim for any test that genuinely cannot run
   under Postgres (e.g. SQLite-specific error path coverage).

## Acceptance

- All tests in `internal/portal/sessions/*_test.go` run as `TestX/sqlite`
  and (with `JAMSESH_TEST_PG_DSN` set) `TestX/postgres` sub-tests.
- The Postgres adapter for sessions-specific queries gets exercised at the
  handler integration level.
- `go test ./internal/portal/sessions/...` passes against both dialects.

## Notes

- This is pure mechanical refactor — no behavior changes expected.
- Tag `refactor` if scoped that way; otherwise the existing `testing`
  tag is appropriate.

## Implementation details

**`internal/portal/sessions/handler_test.go`**:
1. Delete the `openStore` helper function (the one that calls
   `storetest.Stores(t)[0].Open(t)` and the comment explaining why it
   only takes SQLite).
2. Change `newTestEnv(t *testing.T)` signature to
   `newTestEnv(t *testing.T, s store.Store) *testEnv`.
3. Each top-level test function (`func TestX(t *testing.T)`) wraps its
   body in the dialect loop:
   ```go
   for _, h := range storetest.Stores(t) {
       h := h
       t.Run(h.Name, func(t *testing.T) {
           env := newTestEnv(t, h.Open(t))
           // existing body
       })
   }
   ```
4. Tests using `newTestEnvWithStore` need a base store for the dialect
   loop; use `h.Open(t)` as both `handlerStore` base and the wrapping
   store's delegate.
5. `newTestEnvWithClock` in `handler_test.go` (around line 850) calls
   `openStore(t)` — replace with `storetest.Stores(t)` loop or just
   `storetest.Stores(t)[0].Open(t)` if clock tests don't need Postgres
   coverage (acceptable — clock behavior is dialect-independent).

**`internal/portal/sessions/clock_test.go`**:
- Two tests call `newTestEnv(t)`. Update to
  `newTestEnv(t, storetest.Stores(t)[0].Open(t))` or wrap in the loop.
  Wrapping is preferred for consistency.

**`internal/portal/sessions/files_test.go`**:
- `buildFilesEnv` calls `newTestEnv(t)`. Update its signature to accept
  `store.Store` and pass `h.Open(t)` from the dialect loop in each test.

**`internal/portal/sessions/invites_test.go`**,
**`internal/portal/sessions/listing_state_test.go`**,
**`internal/portal/sessions/refmodes_test.go`**:
- Each test calls `newTestEnv(t)`. Wrap each `func TestX(t)` body in the
  `storetest.Stores(t)` loop and update calls to `newTestEnv(t, h.Open(t))`.

**`internal/portal/sessions/scope_validation_test.go`**:
- Tests call pure validation functions — no store, no env. No changes needed.

**Verification**:
```bash
grep -n 'newTestEnv(t)' internal/portal/sessions/*_test.go
# Should return zero results after the retrofit.

go test ./internal/portal/sessions/...
# Should pass (SQLite only without JAMSESH_TEST_PG_DSN).
```
