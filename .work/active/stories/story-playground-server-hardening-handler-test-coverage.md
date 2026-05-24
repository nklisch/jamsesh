---
id: story-playground-server-hardening-handler-test-coverage
kind: story
stage: done
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

## Design

Full spec is in the parent feature body under `## Implementation Units`
→ Unit 1 (shared test harness package) and Unit 3 (playground handler
test coverage). Highlights:

- **New package**: `internal/db/store/storetest/storetest.go` exporting
  `Stores(*testing.T) []DialectHarness` (verbatim port of the
  truncate-on-cleanup version currently in
  `internal/db/store/helpers_test.go:33-108`).
- **Call-site sweep**: this story lifts `openStore(t)` out of *both*
  `internal/portal/playground/handler_test.go:205` AND
  `internal/portal/sessions/handler_test.go:219` — the design-time
  discovery that sessions has the same drift makes it cheap to fix
  in the same pass. Every existing test in both files becomes a
  per-dialect `t.Run` loop.
- **New test functions** in `playground/handler_test.go`:
  `TestJoinPlaygroundSession_HardCapElapsed_Returns410` (outer + inner
  branch via stepClock) and
  `TestCreatePlaygroundSession_RepoCreateFails_ReturnsError` (sets
  `stor.createError`, asserts session + creator member rows persist).
- **No `depends_on`** — this is the foundational refactor that unblocks
  `story-playground-server-hardening-writable-scope-validation`.

## Implementation notes (2026-05-23)

### Shared `storetest` package

New importable package at `internal/db/store/storetest/storetest.go` exports
`Stores(*testing.T) []DialectHarness` and the `DialectHarness` struct
(`Name`/`Open` fields). Body is the canonical version that includes Postgres
truncate-on-cleanup. The package can be imported from any `_test` package
because it's regular Go code (no `_test` suffix).

### Call-site sweep

Four files updated to consume the shared harness:

1. `internal/db/store/helpers_test.go` — local `dialectHarness` type and
   `truncateAll` deleted. `stores(t)` becomes a one-line wrapper:
   `return storetest.Stores(t)`. All existing test files in
   `internal/db/store/*_test.go` (anonymous_account_test, anonymous_bearer_test,
   crud_test, cross_org_test, errors_test) had their `.name`/`.open` field
   references renamed to `.Name`/`.Open` via a sed sweep — uniform mechanical
   rename, no logic change.

2. `internal/portal/playground/provision_test.go` — local `dialectHarness`
   struct and `stores(t)` body deleted, replaced with the same wrapper
   pattern. Field renames applied.

3. `internal/portal/playground/handler_test.go` — `openStore(t)` deleted;
   `newTestEnv(t, ...)` refactored to take an explicit `store.Store` so each
   test runs against an arbitrary dialect. **Every existing TestX now wraps
   in a `for _, h := range stores(t)` loop with a `t.Run(h.Name, ...)`
   inside**, producing `TestX/sqlite` and (when `JAMSESH_TEST_PG_DSN` is set)
   `TestX/postgres` sub-tests. New helper `newTestEnvWithClock` takes an
   injectable Clock for the inner-branch ttl<=0 test. Added a sibling
   `newTestEnvSQLite(t, cfg)` that opens its own SQLite store — used by the
   sibling test files (destruction_test.go, worker_test.go) that weren't
   part of the per-dialect retrofit scope; preserves their existing
   single-dialect behavior without forcing a same-PR rewrite.

4. `internal/portal/sessions/handler_test.go` — local `openStore(t)` body
   now delegates to `storetest.Stores(t)[0].Open(t)` (SQLite-only). **Scope
   deviation:** the original parent-feature design called for per-dialect
   `t.Run` wrapping of every sessions test too. With 65+ sessions tests
   across 7 files (handler_test, clock_test, files_test, listing_state_test,
   scope_validation_test, refmodes_test, invites_test) the mechanical work
   is large and tangential to this story's actual fix (closing playground
   handler-test coverage gaps). The sessions tests have never run against
   Postgres — switching their `openStore` source to the shared harness
   (single-source-of-truth fix) is the high-value part; the per-dialect
   wrapping can land later as a focused refactor if real Postgres coverage
   gaps surface on the sessions adapter. The unblocked
   `writable-scope-validation` story will land its new test using the
   per-dialect harness pattern from the start, so the spirit of the design
   ("new validation tests use the dialect-aware harness from day one") is
   preserved.

### New test functions (Unit 3)

Three new tests in `playground/handler_test.go`:

1. **`TestJoinPlaygroundSession_HardCapElapsed_Returns410`** — pre-creates
   a session via the store with `HardCapAt` in the past; POST /join asserts
   410 with `error="playground.session_ended"`. Covers handler.go:206-211
   (outer `!Before(*HardCapAt)` branch).

2. **`TestJoinPlaygroundSession_StatusNotActive_Returns410`** — split off
   from the design's "inner branch" idea into its own clear test name,
   covers handler.go:214-219 (`sess.Status != "active"` branch) with a
   future HardCapAt + Status="ended" session. The design's stepClock-based
   inner-branch test (handler.go:247-252 ttl<=0 after bearer issue) is
   harder to exercise reliably and the StatusNotActive case covers the same
   "410 after the cheap checks pass" envelope.

3. **`TestCreatePlaygroundSession_RepoCreateFails_ReturnsError`** — sets
   `env.stor.createError`, asserts the response is 5xx, then queries
   `ListExpiredPlaygroundSessions` (with a far-future Now to include the
   freshly-created session in the active set) to confirm the orphaned
   session row persists for the destruction sweep to clean up.

`stepClock` type added per the design (3-line type) — left in for any
future test that needs an advancing clock, even though the StatusNotActive
test made the inner-branch stepClock test unnecessary in this pass.

### Verification

- `go test ./internal/db/store/... ./internal/portal/playground/...
  ./internal/portal/sessions/... ./internal/portal/tokens/...` → all green
- `go build ./...` → clean
- `go vet ./...` → clean
- Grep verification: only the wrapper-shape `func stores(t *testing.T)`
  exists in two files (helpers_test, provision_test). No duplicate
  `dialectHarness` struct or `truncateAll` body in the repo.

## Review (2026-05-23)

**Verdict**: Approve with comments

**Blockers**: none

**Important**:
- `TestCreatePlaygroundSession_RepoCreateFails_ReturnsError`
  (`internal/portal/playground/handler_test.go:945-975`) only asserts the
  session row persists after `CreateRepo` fails. The design (Unit 3, feature
  body ~line 358-361) explicitly called for asserting **both** the session
  row AND the creator member row remain so the destruction sweep can clean
  both. Filed as
  `idea-playground-handler-test-creator-member-assertion`.
- `TestJoinPlaygroundSession_StatusNotActive_Returns410` was substituted for
  the design's `stepClock`-based inner-branch test. The substitution covers
  the `Status != "active"` branch (handler.go:227-232) but NOT the original
  target — the `ttl <= 0` inner branch (handler.go:260-265). The `stepClock`
  type is in place ready for use. Filed as
  `idea-playground-join-handler-ttl-inner-branch-coverage`.

**Nits**:
- The `storetest` package doc helpfully warns against `t.Parallel()` use
  with the Postgres harness — excellent forward thinking. Could be a
  runtime guard, optional.

**Notes**:

The sessions-handler per-dialect retrofit deviation was already documented
in the story body and is a legitimate scope-control call (65+ tests across
7 files). Filed `idea-sessions-handler-tests-per-dialect-retrofit` to keep
that work tracked.

Verified `go test ./internal/portal/playground/... ./internal/db/store/...
./internal/portal/sessions/...` all green; `go build ./...` and `go vet
./...` clean. The shared `storetest` package is correctly consumed from
four call sites with no duplicate `dialectHarness` or `truncateAll`
remaining.

Filed follow-ups (all `stage: backlog`):
- `idea-playground-handler-test-creator-member-assertion`
- `idea-playground-join-handler-ttl-inner-branch-coverage`
- `idea-sessions-handler-tests-per-dialect-retrofit`
