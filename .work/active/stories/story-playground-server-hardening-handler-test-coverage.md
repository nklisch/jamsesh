---
id: story-playground-server-hardening-handler-test-coverage
kind: story
stage: implementing
tags: [portal, playground, testing]
parent: feature-playground-server-hardening
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Playground handler tests miss three explicit acceptance criteria

## Origin

Filed from review of
`story-epic-ephemeral-playground-session-lifecycle-rest-endpoints`.

## Problem

The story's acceptance criteria list these tests as required, but the
delivered `handler_test.go` does not cover them:

1. **Join after `hard_cap_at` elapsed → 410 `playground.session_ended`.**
   The handler has the logic at `handler.go:206-211` and `:247-252`, but
   no test exercises either branch.

2. **Bare-repo create failure rollback.** `stubStorage` exposes a
   `createError` field at `handler_test.go:38`, but no test sets it to
   verify that a `CreateRepo` failure after the session insert returns an
   error and leaves the orphaned session for the destruction sweep to
   clean up.

3. **Tests run under both SQLite and Postgres.** The story design says
   "tests run under both SQLite and Postgres via stores(t) harness".
   `openStore(t)` at `handler_test.go:205` hard-codes
   `db.Open(..., "sqlite", ...)` — Postgres is never exercised.

## Impact

- Regressions in any of the three code paths would slip past CI.
- The Postgres adapter for the new queries (`NicknameTakenInSession`,
  `CountSessionMembers`, `GetTombstone`, `RecordTombstone`, etc.) is
  not test-covered at the handler level.

## Fix

- Add `TestJoinPlaygroundSession_HardCapElapsed_Returns410` that
  pre-creates a session via the store with a past `HardCapAt`, then
  POSTs to `/join` and asserts 410 + `playground.session_ended`.
- Add `TestCreatePlaygroundSession_RepoCreateFails_ReturnsError` that
  sets `stor.createError` and asserts the response is an error and the
  session row remains in the store (orphaned).
- Refactor `openStore(t)` into a `stores(t)` helper matching the
  pattern used elsewhere (see `internal/portal/sessions/handler_test.go`
  for the established shape) so every test runs against both dialects.

## Acceptance

- All three test cases land and pass under both dialects.
- `go test ./internal/portal/playground/...` exercises both Postgres
  and SQLite per-test.
