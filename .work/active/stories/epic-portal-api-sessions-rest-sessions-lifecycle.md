---
id: epic-portal-api-sessions-rest-sessions-lifecycle
kind: story
stage: done
tags: [portal]
parent: epic-portal-api-sessions-rest
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Sessions REST — Lifecycle (Create, Patch, Finalize, Abandon)

## Scope

Schema extensions for `sessions` + new `ref_modes` table + 4 lifecycle endpoints + openapi schemas.

## Units delivered

- `internal/db/migrations/{sqlite,postgres}/00006_sessions_lifecycle.sql` — add `end_reason`, `finalize_locked_by_account_id` to sessions; new `ref_modes(session_id, ref, mode)` table
- `db/schema/{sqlite,postgres}.sql` (edit)
- `db/queries/{sqlite,postgres}/sessions.sql` (edit) — add SetSessionEndReason, SetFinalizeLock, GetSessionForUpdate (for Tx finalize-idempotency)
- `db/queries/{sqlite,postgres}/ref_modes.sql` — UpsertRefMode, GetRefMode, ListRefModesForSession
- Regen sqlitestore + pgstore
- `internal/db/store/store.go` (edit) — RefModeStore sub-interface
- Adapters updated
- `internal/portal/sessions/handler.go` — Handler struct + 4 methods (CreateSession, PatchSession, FinalizeSession, AbandonSession)
- `docs/openapi.yaml` (edit) — 4 paths + schemas Session, CreateSessionRequest, PatchSessionRequest, MemberSummary, FinalizeResponse, AbandonResponse
- Regen openapi
- `cmd/portal/main.go` (edit) — construct sessions.Handler, add to combinedHandler, register routes inside authenticated group
- Tests

## Acceptance Criteria

- [ ] POST /api/sessions: insert session + creator member + create bare repo via storage in one Tx; rollback on any step failure (verified by injecting storage failure)
- [ ] PATCH: creator can update goal/scope (widening only)/default_mode; scope narrowing → 400 `session.scope_narrowing_rejected`
- [ ] FinalizeSession: active→finalizing; already-finalizing → 200 (no-op, no event); ended → 409
- [ ] AbandonSession: creator only; sets status=ended + end_reason=abandoned + emits session.ended; double-fire → no duplicate event
- [ ] Emit `session.created` on POST, `session.finalizing` on finalize, `session.ended` on abandon
- [ ] make generate clean, build clean, tests green

## Notes

- Scope narrowing detection: parse old + new scope as glob sets; if any old glob is not implied by any new glob, it's narrowing. Simplest: require new scope to be a superset (each old entry literally appears in new) OR use a stricter "scope is append-only" rule. Pick the strict append-only rule for v1.
- Atomicity: storage.CreateRepo creates a directory on disk. Tx rollback can't undo that — so call CreateRepo AFTER the session row commit; on failure, call storage.RemoveRepo. Order: BEGIN → INSERT session → INSERT session_member → COMMIT → CreateRepo → on err, delete session via separate query. Document this; the invariant "session row exists ⟹ repo exists" allows momentary inconsistency on a process crash between the COMMIT and CreateRepo (acceptable; reconciliation sweep cleans up).

## Implementation notes

All units implemented and tested:

- **Migration 00006**: Recreates sessions table (SQLite) / ALTER TABLE (Postgres) to add `end_reason`, `finalize_locked_by_account_id` columns and extend status CHECK to include 'finalizing'. New `ref_modes` table added.
- **Schema files** (sqlite.sql, postgres.sql): Updated with new columns and ref_modes table.
- **SQL queries**: Added `UpdateSessionGoalScopeMode`, `SetSessionEndReason`, `SetFinalizeLock`, `ClearFinalizeLock` to sessions.sql; new ref_modes.sql with `UpsertRefMode`, `GetRefMode`, `ListRefModesForSession`.
- **sqlc regen**: Both sqlitestore and pgstore regenerated cleanly.
- **store.go**: Added `RefMode` domain type, `RefModeStore` sub-interface; extended `SessionStore` with new methods; added `RefModeStore` to `TxStore` and `Store`.
- **Adapters**: Both sqlite_adapter.go and postgres_adapter.go updated with new method implementations + mapper functions.
- **handler.go** (`internal/portal/sessions/`): `Handler` struct with `CreateSession`, `PatchSession`, `FinalizeSession`, `AbandonSession`. Atomic Tx+repo creation with compensation pattern. Scope narrowing detection via strict append-only rule. Idempotent finalize (already-finalizing → 200 no-op). Creator-only enforcement for Patch and Abandon.
- **openapi.yaml**: Added `Session`, `MemberSummary`, `CreateSessionRequest`, `PatchSessionRequest` schemas; 4 paths for sessions lifecycle.
- **Regen**: server.gen.go and types.gen.ts regenerated.
- **cmd/portal/main.go**: SessionsHandler constructed and wired; 4 routes registered.
- **Tests**: 12 tests covering all acceptance criteria including Tx rollback on repo failure, scope narrowing rejection, idempotent finalize, double-abandon prevention, creator-only enforcement.
- **Build**: `go build ./...` and `go test ./...` clean.

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Atomic Tx + repo creation with compensation pattern is the right shape. Idempotent finalize. Append-only scope rule is strict but defensible — operators can widen later. Creator-only enforcement clean.
