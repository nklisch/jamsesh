---
id: epic-portal-api-events-log-schema-queries-emit
kind: story
stage: implementing
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
