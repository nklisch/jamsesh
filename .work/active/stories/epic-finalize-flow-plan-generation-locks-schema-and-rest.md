---
id: epic-finalize-flow-plan-generation-locks-schema-and-rest
kind: story
stage: implementing
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

- [ ] `make generate` succeeds (oapi-codegen + sqlc + openapi-typescript)
- [ ] `go build ./...` clean
- [ ] `go test ./internal/portal/finalize/... ./internal/portal/tokens/...` green
- [ ] `IsLockExpired` covers exact-boundary, well-before, well-after
- [ ] Acquire on a session with no existing lock returns 201 + `LockStatus.is_caller=true`,
      sets `sessions.finalize_locked_by_account_id`, emits `session.finalizing`,
      transitions session.status `active → finalizing`
- [ ] Acquire when caller already holds an active fresh lock is idempotent
      (returns same lock id, no duplicate event emitted)
- [ ] Acquire when another member holds a stale (>30min) lock auto-releases it
      and proceeds (new lock id; the stale row gets `released_at` set)
- [ ] Acquire when another member holds a fresh lock + `override=false` returns
      409 `finalize.lock_held_by_other` with `details.held_by_account_id`
- [ ] Acquire when another member holds a fresh lock + `override=true` succeeds,
      sets `superseded_by_lock_id` on the old row, points sessions pointer to
      new caller
- [ ] PATCH updates `selected_commit_shas`, `target_branch`, `base_sha`,
      `mode`, `commit_message` atomically; bumps `last_activity_at`
- [ ] PATCH on an idle (>30min) lock returns 409 `finalize.lock_expired`
      and sets `released_at` on the row
- [ ] PATCH from a non-caller returns 403 `auth.insufficient_permission`
- [ ] DELETE on a held lock by caller returns 204; clears the sessions pointer
- [ ] DELETE on an already-released lock is idempotent (returns 204)
- [ ] DELETE from non-caller returns 403
- [ ] `tokens.IssueShortLived(ctx, accID, 5*time.Minute)` returns a token
      that `tokens.Validate` accepts immediately and rejects after the TTL
- [ ] OpenAPI schemas + paths land in `docs/openapi.yaml`; generated
      `internal/api/openapi/server.gen.go` carries the new method names
- [ ] Stub implementations for plan / fetch-token / mark-shipped return
      `501 not_implemented` until stories 2 and 3 land (handler compiles)

## Files touched

- `internal/db/migrations/{sqlite,postgres}/00010_finalize_locks.sql` (new)
- `db/queries/{sqlite,postgres}/finalize_locks.sql` (new)
- `internal/db/sqlitestore/*` (regenerated by sqlc)
- `internal/db/pgstore/*` (regenerated by sqlc)
- `internal/db/store/store.go` (add new methods to Store interface)
- `internal/portal/finalize/{doc,handler,lock_check,lock_acquire,lock_patch,lock_release}.go` (new)
- `internal/portal/finalize/{lock_check,lock_acquire,lock_patch,lock_release}_test.go` (new)
- `internal/portal/tokens/{service,service_impl}.go` (add IssueShortLived)
- `internal/portal/tokens/service_test.go` (cover IssueShortLived)
- `docs/openapi.yaml` (add schemas + 3 paths)
- `internal/api/openapi/server.gen.go` (regenerated)
- `cmd/portal/main.go` (construct + wire finalizeHandler into combinedHandler)
