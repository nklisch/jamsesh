---
id: epic-auto-merger-outcomes-apply
kind: story
stage: done
tags: [portal]
parent: epic-auto-merger-outcomes
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Auto-Merger Outcomes — Apply

## Scope

Single story: ship the `conflict_events` schema + Store extension + `Apply` entrypoint that turns a MergeResult into side effects (merge commit + ref advance + event emission, or conflict_events row + event emission).

## Units delivered

- `internal/db/migrations/{sqlite,postgres}/00008_conflict_events.sql`
- `db/schema/{sqlite,postgres}.sql` (edit)
- `db/queries/{sqlite,postgres}/conflict_events.sql` — Insert, GetByID, MarkResolved, ListOpenForSession
- Regen sqlitestore + pgstore
- `internal/db/store/store.go` (edit) — ConflictEventStore sub-interface + domain type
- Both adapters
- `internal/portal/automerger/outcomes.go` — Apply entrypoint + helpers
- `internal/portal/automerger/addressing.go` — computeAddressedTo
- Tests

## Acceptance Criteria

- [x] Clean merge: Apply creates merge commit with author=source-author, committer=auto-merger, trailers (Auto-Merger:true, Source-Commit, Source-Ref), advances draft, emits merge.succeeded
- [x] Safe-auto-resolve: same + Auto-Resolved:<heuristic> trailer
- [x] Hard-conflict: inserts conflict_events row, emits conflict.detected, leaves draft unchanged
- [x] Resolves-Conflict trailer on source: marks matching open event resolved, emits conflict.resolved
- [x] Mismatch (unknown event-id): silent no-op
- [x] computeAddressedTo: walks back up to 100 draft commits, includes source-ref owner + each conflicted-file's last-modifier
- [x] `go test ./internal/portal/automerger/...` green; `go build ./...` clean

## Notes

- Auto-merger identity: `Name: "jamsesh auto-merger", Email: "auto-merger@<portalHost>"`. portalHost is parsed from cfg.PortalURL.
- The Apply function takes an `events.Log` + `store.Store` via constructor.
- For draft ref advance use `repo.Storer.SetReference(plumbing.NewHashReference(draftRefName, newSHA))`.

## Implementation notes

### Files delivered
- `internal/db/migrations/sqlite/00008_conflict_events.sql`
- `internal/db/migrations/postgres/00008_conflict_events.sql`
- `db/schema/sqlite.sql` — conflict_events table appended
- `db/schema/postgres.sql` — conflict_events table appended
- `db/queries/sqlite/conflict_events.sql` — InsertConflictEvent, GetConflictEventByID, MarkConflictEventResolved, ListOpenConflictEventsForSession
- `db/queries/postgres/conflict_events.sql` — same queries, $N placeholders
- `internal/db/sqlitestore/conflict_events.sql.go` — sqlc-generated
- `internal/db/pgstore/conflict_events.sql.go` — sqlc-generated
- `internal/db/store/store.go` — ConflictEvent domain type, InsertConflictEventParams, MarkConflictEventResolvedParams, ConflictEventStore interface; ConflictEventStore added to TxStore + Store
- `internal/db/store/sqlite_adapter.go` — ConflictEventStore implementation + nullTimeToPtr/ptrToNullTime helpers
- `internal/db/store/postgres_adapter.go` — ConflictEventStore implementation + pgTimestamptzToPtr/ptrToPgTimestamptz helpers
- `internal/portal/automerger/outcomes.go` — Applier struct, ApplyInput/ApplyOutput, Apply, success path, conflict path
- `internal/portal/automerger/addressing.go` — computeAddressedTo, parseSourceRefOwner, ExportedComputeAddressedTo (test shim)
- `internal/portal/automerger/outcomes_test.go` — 5 tests
- `internal/portal/automerger/addressing_test.go` — 3 tests

### Design choices
- `computeAddressedTo` errors are non-fatal (logged, fall back to source-ref owner only) so a git walk failure never blocks the conflict event from being inserted.
- `tryResolveConflict` errors are non-fatal (logged) so a DB race on the conflict event doesn't prevent the merge commit from being returned.
- `ExportedComputeAddressedTo` is a thin exported shim; it's only for white-box testing from `automerger_test` package.
- `nullTimeToPtr` / `ptrToNullTime` added to sqlite_adapter.go; `pgTimestamptzToPtr` / `ptrToPgTimestamptz` added to postgres_adapter.go — both helpers will be reused by future tables with nullable timestamps.

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Non-fatal computeAddressedTo and tryResolveConflict errors prevent secondary failures from blocking the merge commit return — defensible. Schema + Store extension + Apply entrypoint all clean. 8 tests cover the matrix.
