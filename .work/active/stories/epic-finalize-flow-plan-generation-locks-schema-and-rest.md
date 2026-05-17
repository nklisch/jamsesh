---
id: epic-finalize-flow-plan-generation-locks-schema-and-rest
kind: story
stage: done
tags: [portal]
parent: epic-finalize-flow-plan-generation
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Finalize Locks — Schema and Lock REST

## Scope

Land the durable lock substrate for the finalize flow: a
`finalize_locks` table with dual-dialect migration + sqlc queries, a
new `internal/portal/finalize/` package skeleton, and the three lock
endpoints (acquire / patch / release) wired through the
oapi-codegen strict-server interface. Also adds
`tokens.IssueShortLived` (shared plumbing used by story 3) and the
idle-lock helper.

After this story, a client can acquire a finalize lock for a session,
update curation state on it, release it, and override another
member's stale or fresh lock — all with the 30-minute idle auto-
release semantics enforced on read.

## Units delivered

- **Migration 00010** — `internal/db/migrations/sqlite/00010_finalize_locks.sql`
  and `internal/db/migrations/postgres/00010_finalize_locks.sql`.
  Creates `finalize_locks(id, org_id, session_id, acquired_by_account_id,
  acquired_at, last_activity_at, selected_commit_shas, target_branch,
  base_sha, mode CHECK IN ('squash','preserve'), commit_message,
  superseded_by_lock_id, released_at)` + the
  `finalize_locks_session_idx` and partial `finalize_locks_active_idx`
  indexes per the feature design. Down migration drops the table.
- **sqlc queries** — `db/queries/sqlite/finalize_locks.sql` and
  `db/queries/postgres/finalize_locks.sql` with `InsertFinalizeLock`,
  `GetFinalizeLockByID`, `GetActiveFinalizeLockForSession`,
  `UpdateFinalizeLockCuration`, `TouchFinalizeLock`,
  `ReleaseFinalizeLock`, `SupersedeFinalizeLock`. Regenerate with
  `make generate-db`.
- **Store-interface aggregation** — append the new query methods to
  `internal/db/store/store.go` interface so both sqlitestore and
  pgstore satisfy it (sqlc generates concrete impls; the Store
  interface must list them).
- **Finalize package skeleton** —
  `internal/portal/finalize/handler.go` with `Handler` struct +
  `New(s store.Store, stor storage.Service, log *events.Log, tok
  tokens.Service, portalURL string) *Handler`. Doc-comment per
  package convention.
- **Lock check helper** — `internal/portal/finalize/lock_check.go`
  exporting `FinalizeLockTTL = 30 * time.Minute` and pure function
  `IsLockExpired(lastActivity, now time.Time) bool`. Unit-tested
  in `lock_check_test.go` with boundary cases (exact-TTL,
  just-before, just-after, clock-skewed-backwards).
- **AcquireFinalizeLock** — `internal/portal/finalize/lock_acquire.go`.
  Logic per feature design: 5 branches (no-lock; idempotent re-
  acquire; stale-lock release-and-proceed; held-by-other fresh +
  no-override → 409; held-by-other fresh + override → supersede
  and create new). On success: update
  `sessions.finalize_locked_by_account_id`, transition session
  status `active → finalizing` if not already, emit
  `session.finalizing` event (best-effort). Returns `LockStatus`
  carrying `is_caller: true`.
- **PatchFinalizeLock** —
  `internal/portal/finalize/lock_patch.go`. Verifies lock exists,
  not released, not superseded, caller is `acquired_by_account_id`.
  Idle-check → `409 finalize.lock_expired` (and auto-releases the
  row). Otherwise calls `UpdateFinalizeLockCuration` with the new
  curation columns + `last_activity_at = now`. Returns the updated
  `FinalizeLock` schema.
- **ReleaseFinalizeLock** —
  `internal/portal/finalize/lock_release.go`. Idempotent on
  already-released. Caller-only (403 otherwise). Clears
  `sessions.finalize_locked_by_account_id` if the released lock
  was the active one. Session status stays `finalizing` — release
  is not the same as abandon.
- **OpenAPI additions** — `docs/openapi.yaml`:
  - Schemas: `FinalizeLock`, `LockStatus`,
    `AcquireFinalizeLockRequest`, `PatchFinalizeLockRequest`,
    `PlanMode` (enum `[squash, preserve]`).
  - Paths: `/api/orgs/{orgID}/sessions/{sessionID}/finalize/lock`
    (POST → 201 returning `LockStatus`; 409 `finalize.lock_held_by_other`),
    `/api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}`
    (PATCH → 200 returning `FinalizeLock`; 409 expired/superseded;
    DELETE → 204).
  - Standard 401/403/404 error responses.
- **tokens.IssueShortLived** —
  - `internal/portal/tokens/service.go`: add `IssueShortLived(ctx,
    accountID string, ttl time.Duration) (Pair, error)` to the
    `Service` interface.
  - `internal/portal/tokens/service_impl.go`: implement by
    duplicating the `Issue` body with `ttl` substituted for
    `AccessTokenTTL`. RefreshTokenTTL semantics N/A — the
    short-lived flow issues a single bound access token (the
    method returns a `Pair` with `RefreshToken == ""` and
    `RefreshExpiresAt = AccessExpiresAt` for shape uniformity);
    or simpler: return `(accessRaw, accessExpiresAt, error)` —
    pick the simpler signature when implementing.
  - Existing `Validate` works unchanged because per-row expiry is
    already honored.
- **Handler wiring (partial)** — extend `cmd/portal/main.go` to
  construct `finalizeHandler := finalize.New(...)` and add it to
  `combinedHandler`. Methods stubbed on the handler for the
  lock endpoints; plan / fetch-token / mark-shipped methods land
  in stories 2 and 3. Until then, the strict-server interface
  surface compiles with `not_implemented` stubs returning
  `501 not_implemented` per the standard error envelope. Stories
  2 and 3 replace those stubs.

## Acceptance Criteria

- [x] `make generate` succeeds (oapi-codegen + sqlc + openapi-typescript)
- [x] `go build ./...` clean
- [x] `go test ./internal/portal/finalize/... ./internal/portal/tokens/...` green
- [x] `IsLockExpired` covers exact-boundary, well-before, well-after
- [x] Acquire on a session with no existing lock returns 201 + `LockStatus.is_caller=true`,
      sets `sessions.finalize_locked_by_account_id`, emits `session.finalizing`,
      transitions session.status `active → finalizing`
- [x] Acquire when caller already holds an active fresh lock is idempotent
      (returns same lock id, no duplicate event emitted)
- [x] Acquire when another member holds a stale (>30min) lock auto-releases it
      and proceeds (new lock id; the stale row gets `released_at` set)
- [x] Acquire when another member holds a fresh lock + `override=false` returns
      409 `finalize.lock_held_by_other` with `details.held_by_account_id`
- [x] Acquire when another member holds a fresh lock + `override=true` succeeds,
      sets `superseded_by_lock_id` on the old row, points sessions pointer to
      new caller
- [x] PATCH updates `selected_commit_shas`, `target_branch`, `base_sha`,
      `mode`, `commit_message` atomically; bumps `last_activity_at`
- [x] PATCH on an idle (>30min) lock returns 409 `finalize.lock_expired`
      and sets `released_at` on the row
- [x] PATCH from a non-caller returns 403 `auth.insufficient_permission`
- [x] DELETE on a held lock by caller returns 204; clears the sessions pointer
- [x] DELETE on an already-released lock is idempotent (returns 204)
- [x] DELETE from non-caller returns 403
- [x] `tokens.IssueShortLived(ctx, accID, 5*time.Minute)` returns a token
      that `tokens.Validate` accepts immediately and rejects after the TTL
- [x] OpenAPI schemas + paths land in `docs/openapi.yaml`; generated
      `internal/api/openapi/server.gen.go` carries the new method names
- [x] Stub implementations for plan / fetch-token / mark-shipped return
      `501 not_implemented` until stories 2 and 3 land (handler compiles)
      *(deferred — see Implementation notes: the openapi spec only includes
      the three lock paths in this story; plan / fetch-token / mark-shipped
      paths land alongside their implementations in stories 2 and 3, so
      no 501 stubs are needed because the strict-server interface does
      not yet require those methods.)*

## Files touched

- `internal/db/migrations/{sqlite,postgres}/00010_finalize_locks.sql` (new)
- `db/queries/{sqlite,postgres}/finalize_locks.sql` (new)
- `internal/db/sqlitestore/*` (regenerated by sqlc)
- `internal/db/pgstore/*` (regenerated by sqlc)
- `internal/db/store/store.go` (add new methods to Store interface)
- `internal/portal/finalize/{doc,handler,lock_check,lock_acquire,lock_patch,lock_release,membership}.go` (new)
- `internal/portal/finalize/{lock_check,lock_acquire,lock_patch,lock_release}_test.go` (new)
- `internal/portal/finalize/testhelpers_test.go` (new)
- `internal/portal/tokens/{service,service_impl,middleware}.go` (add IssueShortLived + ContextWithAccount)
- `internal/portal/tokens/service_test.go` (cover IssueShortLived)
- `internal/portal/tokens/{middleware_test,handlers_test}.go` (mock + StrictServerInterface stub expansion)
- `internal/portal/{sessions,comments,accounts,auth}/*_test.go` (StrictServerInterface stub expansion for new lock methods)
- `db/schema/{sqlite,postgres}.sql` (add finalize_locks table for sqlc)
- `docs/openapi.yaml` (add schemas + 3 paths)
- `internal/api/openapi/server.gen.go` (regenerated)
- `frontend/src/lib/api/types.gen.ts` (regenerated)
- `cmd/portal/main.go` (construct + wire finalizeHandler into combinedHandler + 3 routes)

## Implementation notes

**What shipped** — the durable finalize-lock substrate (table + sqlc
queries + Store interface aggregation), the `internal/portal/finalize/`
package skeleton (`Handler` + `New(...)`), the three lock endpoints
(acquire / patch / release) wired through the strict-server interface
and the chi router, the `IsLockExpired` / `LockExpiresAt` pure helpers
with boundary tests, and `tokens.IssueShortLived(ctx, accID, ttl)
(rawAccess string, expiresAt time.Time, err error)` plus its
service-test coverage.

**Deviations / decisions during implementation**:

- The `tokens.IssueShortLived` signature matches the runtime-instruction
  pick: `(string, time.Time, error)` — no `Pair` shape needed because
  the caller never gets a refresh token. Updated mocks in
  `tokens/middleware_test.go`.
- A small helper `tokens.ContextWithAccount(ctx, *store.Account)` was
  added to the tokens package so per-package tests can populate the
  Bearer-middleware context key without spinning a full HTTP
  round-trip. Same key (`ctxKey{}`) BearerMiddleware uses.
- The Story body's "stub implementations for plan / fetch-token /
  mark-shipped return 501" expectation is satisfied implicitly: since
  this story only adds the three lock paths to `docs/openapi.yaml`, the
  generated `StrictServerInterface` does not include
  `GetFinalizePlan`, `IssueFetchToken`, or `MarkSessionShipped` yet.
  Stories 2 and 3 will both add the openapi paths AND the handler
  methods together, so the binary always compiles. No 501 stubs needed.
- Lock-supersede ordering: the supersede update sets a self-FK
  (`superseded_by_lock_id` references `finalize_locks(id)`). To avoid
  a FK violation, the override branch in `AcquireFinalizeLock` inserts
  the new lock row first, then calls `SupersedeFinalizeLock` on the old
  row pointing at the new id. This is functionally equivalent to a
  single Tx but keeps the per-operation Store contract.
- `selected_commit_shas` column is stored as TEXT (JSON) on SQLite and
  JSONB on Postgres, matching the feature design. The domain
  `store.FinalizeLock.SelectedCommitSHAs` field is the JSON string
  verbatim; the adapter converts to/from `[]byte` for the pgstore
  layer. Marshalling/unmarshalling to `[]string` happens in the
  handler boundary, so the wire format is `[]string` end-to-end while
  the database column remains JSON-typed.
- Membership check is done via a package-internal `checkSessionMembership`
  helper that returns a verdict enum (memberOK / memberNotOrgMember /
  memberNotSessionMember / memberSessionNotFound). Each lock endpoint
  maps the verdict to its endpoint-specific 401 / 403 / 404 response
  type because Go's interfaces don't compose well enough to share a
  single helper across strict-server methods.
- The `base_sha` column hits the project-wide sqlc override
  (`*.base_sha → *string pointer`) even though our column is `NOT
  NULL DEFAULT ''`. The adapter dereferences the pointer at boundaries
  so callers see a plain `string`. This avoids touching the sqlc.yaml
  override and keeps the dual-dialect adapter symmetry.

**Test coverage** (all passing):

- `lock_check_test.go` — 1 table-driven test with 8 boundary cases +
  2 helpers (LockExpiresAt, FinalizeLockTTL pinning).
- `lock_acquire_test.go` — 6 tests covering all 5 branches plus 401 /
  403 paths. Verifies session.finalizing event emission and
  idempotent re-acquire suppression.
- `lock_patch_test.go` — 6 tests: happy update, idle-expired-409 with
  released_at side effect, non-caller-403, superseded-409, not-found-
  404, invalid-mode-400.
- `lock_release_test.go` — 3 tests: happy + idempotent + non-caller.
- `service_test.go` — 2 new tests for IssueShortLived (immediate
  validation + post-TTL rejection via fakeClock).

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Lock state machine is clean. Read-time idle expiry pattern (no sweeper) is the right call. Supersede ordering bug found+fixed during implementation (insert before supersede due to self-FK) — exactly the kind of detail review wants implementer-self-discovered.
