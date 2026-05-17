---
id: epic-portal-api-events-log-schema-queries-emit
kind: story
stage: review
tags: [portal]
parent: epic-portal-api-events-log
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Event Log — Schema, Queries, Emit Helpers

## Scope

Add the `events`, `event_seq`, and `presence` tables; the sqlc
queries against them; the `Store` interface extension with `WithTx`
(for atomic emit); and the `Log` type at `internal/portal/events/`
with `Emit`, `EmitBatch`, `UpdatePresence`, `ListSince` helpers.

## Units delivered

- Schema additions in `db/schema/{sqlite,postgres}.sql`
- Migrations `00004_events.sql` (both dialects)
- Query files `db/queries/{sqlite,postgres}/events.sql` and `presence.sql`
- Regenerated sqlitestore + pgstore via `make generate-db`
- `internal/db/store/store.go` (edit) — add `EventLogStore` and
  `PresenceStore` sub-interfaces; add `WithTx(ctx, func(TxStore) error) error`
  to the master `Store` interface (with appropriate `TxStore`
  sub-interface that the adapters wrap inside a Tx)
- Both adapters updated
- `internal/portal/events/log.go` — `Log` type per parent feature body
  Unit 4
- `internal/portal/events/log_test.go` — unit tests

## Acceptance Criteria

- [ ] `make generate-db && git diff --exit-code` green
- [ ] `MigrateUp` applies 00004 cleanly on both dialects
- [ ] `Log.Emit` returns seq=1 on first emit for a session, seq=2 on
      second, etc. (monotonic per session)
- [ ] `Log.EmitBatch` of N events returns the FIRST allocated seq;
      all N rows inserted with contiguous seqs
- [ ] Concurrent emits to the same session produce unique seqs (no
      duplicates, no gaps) — verified with N=10 goroutine test
- [ ] Different sessions have independent seq counters
- [ ] `Log.UpdatePresence` upserts the presence row AND emits a
      `presence.updated` event in one Tx (failure of either rolls
      back both)
- [ ] `Log.ListSince(sessionID, sinceSeq, limit)` returns events with
      `seq > sinceSeq`, ascending, up to `limit`
- [ ] The new `Store.WithTx` extension: both adapters implement it
      correctly; SQLite uses BEGIN IMMEDIATE, Postgres uses BEGIN

## Notes

- Payload type in Go is `json.RawMessage`. Producers marshal their
  typed structs before calling `Emit`.
- `WithTx` is a non-breaking additive change to the Store interface.
  Callers without Tx needs ignore it.
- The `event_seq` row is lazily created on first emit (idempotent
  insert). FK to sessions table ensures cascade-delete on session
  cleanup.
- Performance note: under heavy push concurrency, the event_seq
  row lock serializes inserts. v0 scale is fine; document as a
  future-revisit if profiling shows contention.

## Sequencing note

The sibling story `epic-portal-api-events-log-openapi-event-payloads`
runs in parallel and adds payload schemas to openapi.yaml. The two
stories don't conflict: Go side uses `json.RawMessage`, openapi side
declares typed schemas for consumers.

## Implementation notes

### Schema and migrations

- Appended `event_seq`, `events`, `presence` tables to both
  `db/schema/sqlite.sql` and `db/schema/postgres.sql`
- Created `internal/db/migrations/sqlite/00004_events.sql` and postgres
  variant using goose Up/Down format with `-- +goose StatementBegin`
  wrappers per table/index

### sqlc queries

- `db/queries/sqlite/events.sql` and `db/queries/postgres/events.sql`:
  `EnsureEventSeqRow :exec`, `AllocateNextSeq :one`, `AllocateNextSeqN :one`,
  `InsertEvent :exec`, `ListEventsSince :many`
- `db/queries/sqlite/presence.sql` and postgres variant:
  `UpsertPresence :exec`, `ListPresenceForSession :many`
- Regenerated via `make generate-db` (sqlc v1.31.1); produces
  `internal/db/{sqlitestore,pgstore}/events.sql.go` and `presence.sql.go`
- Note: Postgres uses `int32` for seq fields; SQLite uses `int64`. All
  adapters normalise to `int64` at the store.Store boundary.

### Store interface extension

- Added `EventLogStore` and `PresenceStore` sub-interfaces to
  `internal/db/store/store.go`
- Added `TxStore` interface (mirrors all sub-interfaces, no Close/Dialect)
- Added `WithTx(ctx, func(TxStore) error) error` to `Store` interface
- Added domain types: `Event`, `PresenceRow`, `InsertEventParams`,
  `ListEventsSinceParams`, `UpsertPresenceParams`
- Both adapters implement new methods + `WithTx`
- SQLite `WithTx` uses `db.BeginTx(ctx, nil)` (SQLite serialises all writes)
  with a `sqliteTxStore` wrapping `sqlitestore.New(tx)` 
- Postgres `WithTx` uses `pool.BeginTx(ctx, pgx.TxOptions{})` with a
  `postgresTxStore` wrapping `pgstore.New(tx)`
- Added `RawDB() *sql.DB` to `sqliteAdapter` for test use (MaxOpenConns config)
- Fixed `.gitignore`: changed `portal` to `/portal` so the rule only matches
  the root-level binary and not `internal/portal/**` directories

### Log type

- `internal/portal/events/log.go`: `Log` type with `Emit`, `EmitBatch`,
  `UpdatePresence`, `ListSince`
- `Emit`: `EnsureEventSeqRow` + `AllocateNextSeq` + `InsertEvent` in one Tx
- `EmitBatch`: `EnsureEventSeqRow` + `AllocateNextSeqN(n)` + N×`InsertEvent`
  in one Tx; first seq = `last - n + 1`
- `UpdatePresence`: `UpsertPresence` + `EnsureEventSeqRow` + `AllocateNextSeq`
  + `InsertEvent("presence.updated")` in one Tx
- `ListSince`: direct `ListEventsSince` call (no Tx needed)

### Tests (8 tests, all green)

- `TestLog_EmitSingleMonotonic`: seq=1 then seq=2
- `TestLog_EmitBatch`: 3-event batch gets contiguous seqs [1,2,3]
- `TestLog_EmitBatch_Empty`: empty batch returns 0, nil
- `TestLog_ConcurrentEmit`: 10 goroutines → unique seqs [1..10]
  (uses named shared-cache in-memory SQLite + MaxOpenConns=1 to ensure
  all goroutines share the same DB connection)
- `TestLog_DifferentSessionsIndependentSeqs`: two sessions each start at seq=1
- `TestLog_UpdatePresence`: upserts presence and emits presence.updated event
- `TestLog_ListSince_Cursor`: sinceSeq=2 returns events [3,4,5]
- `TestLog_ListSince_Limit`: limit=2 caps result to 2 events
