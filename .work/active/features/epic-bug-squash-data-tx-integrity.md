---
id: epic-bug-squash-data-tx-integrity
kind: feature
stage: implementing
tags: [bug, portal]
parent: epic-bug-squash
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Data-layer & transactional/event-emission integrity

## Brief

This feature fixes correctness defects in the persistence and
transaction/event-emission layer. The bug-scan found five: cursor pagination
that drops rows sharing a `created_at` (no `id` tiebreaker), a SQLite `WithTx`
that opens DEFERRED despite a "BEGIN IMMEDIATE" comment (lock-upgrade deadlock
risk), a Postgres `seq` column that is 32-bit while the domain model is int64,
a comments WS fan-out that omits the allocated `seq` (breaking client replay
dedup), and a finalize-lock acquisition that runs a 4-step mutation with no
enclosing transaction.

This feature delivers keyset-stable pagination, correct SQLite write-lock
acquisition, dialect-consistent column types, seq-carrying event fan-out, and
atomic multi-step mutations — preserving the dual-dialect (sqlite/postgres)
mirror discipline and the `tx-emit-then-fanout` invariant throughout. It covers
store/query/schema/tx correctness only; it does NOT redesign the data model,
add new tables, or change the event schema. Schema changes (seq → BIGINT,
keyset columns) require mirrored sqlc regeneration and a forward goose
migration.

## Epic context
- Parent epic: `epic-bug-squash`
- Position in epic: independent backend feature — touches `internal/db/store`,
  `db/queries/{sqlite,postgres}`, `db/schema`, `internal/portal/{comments,finalize,pagination}`.

## Foundation references
- `docs/SPEC.md` — sqlc dual-dialect, SQLite default / Postgres swap
- `docs/ARCHITECTURE.md` — Portal § data store
- Patterns: `dual-dialect-mirror-queries`, `tx-emit-then-fanout`, `adapter-wrap-helpers`

## Child stories (pre-existing, from bug-scan — re-parented here)
- `bug-squash-cursor-pagination-drops-rows` — Medium, data-layer — `db/queries/sqlite/comments.sql:27`
- `bug-squash-sqlite-withtx-deferred-not-immediate` — Medium, data-layer — `internal/db/store/sqlite_adapter.go:1034`
- `bug-squash-comments-fanout-omits-seq` — Medium, error-handling — `internal/portal/comments/service.go:254`
- `bug-squash-finalize-lock-no-transaction` — Medium, error-handling — `internal/portal/finalize/lock_acquire.go:187`
- `bug-squash-postgres-seq-32bit` — Low, data-layer — `db/schema/postgres.sql:118`

## Design caveats (from codex decomposition gate — feature-design must honor)
- **seq INTEGER→BIGINT is non-destructive *widening*, not "additive"**: the
  migration must cover BOTH `events.seq` AND `event_seq.next`, regenerate sqlc
  for both dialects, drop every `int32(...)` cast in the postgres adapter, and
  ship an existing-row migration test. Goose down policy: no destructive down
  (a narrowing down-migration would truncate live data) — document the
  irreversibility. Account for the table-rewrite/lock cost of `ALTER COLUMN
  TYPE` on Postgres.
- **Intra-feature ordering**: `bug-squash-sqlite-withtx-deferred-not-immediate`
  lands before `bug-squash-finalize-lock-no-transaction` (declared via the
  latter's `depends_on`) — don't wrap new work in `WithTx` while the tx
  primitive's lock-acquisition behavior is still under repair.
- **Keyset pagination**: thread `cur.LastID` into both dialect queries and both
  adapters; keep the sqlite/postgres queries mirrored (`dual-dialect-mirror-queries`).

## Architectural choice

**Local fixes within the existing dual-dialect sqlc + tx-emit-then-fanout
patterns** — no new persistence abstraction. Each fix stays inside the
established seams: mirrored sqlite/postgres queries, `store.WithTx`,
`AllocateNextSeq` + `FanOut`. The 5 stories touch `db/queries/{sqlite,postgres}`,
`db/schema`, `internal/db/store`, `internal/portal/{pagination,comments,finalize}`.

## Implementation Units

### Unit 1: Keyset cursor pagination ((created_at, id) tiebreaker)
**Files**: `db/queries/sqlite/comments.sql`, `db/queries/postgres/comments.sql`,
`db/queries/sqlite/sessions.sql`, `db/queries/postgres/sessions.sql`,
`internal/db/store/{sqlite,postgres}_adapter.go`, `internal/portal/comments/service.go` (List),
`internal/portal/sessions/listing.go`
**Story**: `bug-squash-cursor-pagination-drops-rows` (Medium)

The cursor already carries `LastCreatedAtNs` + `LastID` (`pagination/cursor.go`),
but the queries bound only on `created_at < ?` with `ORDER BY created_at DESC` —
dropping rows that share the boundary `created_at`. Make it a true keyset:

```sql
-- ListCommentsForSession / ListSessionsForOrgWithCursor (both dialects, mirrored)
WHERE session_id = ? /* ...filters... */
  AND (created_at < ?1 OR (created_at = ?1 AND id < ?2))
ORDER BY created_at DESC, id DESC
LIMIT ?;
```

Thread `cur.LastID` through the `*Params` structs and both adapters; callers
pass `cur.LastCreatedAt()` AND `cur.LastID`. The first page (no cursor) keeps a
sentinel that admits all rows (e.g. max time + max id, or a separate query
branch as today).

**Implementation Notes**: postgres uses `$1/$2`, sqlite uses `?` positional —
keep the two files mirrored in column/filter/order semantics
(`dual-dialect-mirror-queries`). Verify `id` (ULID string) DESC ordering is a
stable total order alongside `created_at`.

**Acceptance Criteria**:
- [ ] Rows sharing a `created_at` at a page boundary are neither dropped nor
      duplicated (test: insert ≥ `limit`+2 comments with identical `created_at`,
      page through, assert every id appears exactly once).
- [ ] sqlite and postgres produce identical pagination (dual-dialect test).

### Unit 2: Comments fan-out carries the allocated seq + id
**File**: `internal/portal/comments/service.go` (Create ~:254, Resolve ~:336)
**Story**: `bug-squash-comments-fanout-omits-seq` (Medium)

Both `FanOut` calls build `events.Event` with `Seq` (and `ID`) defaulted to 0/"".
Capture the tx-allocated `seq`/`eventID` into outer vars (as `UpdatePresence`
does) and set them on the fan-out event:

```go
var seq int64
var eventID string
err = s.Store.WithTx(ctx, func(tx store.TxStore) error {
    // ...allocate seq, eventID; InsertEvent(... Seq: seq, ID: eventID ...)
})
// ...
s.Log.FanOut(events.Event{ID: eventID, OrgID: ..., SessionID: ..., Seq: seq,
    Type: "comment.added", Payload: payloadBytes, CreatedAt: now})
```

**Implementation Notes**: applies to both `comment.added` (Create) and
`comment.resolved` (Resolve). Without the seq, the SPA's `lastSeenSeq` replay
cursor never advances for comment events → duplicate redelivery on reconnect.
Same shape as `events/log.go`'s own fan-out.

**Acceptance Criteria**:
- [ ] The fanned-out `comment.added`/`comment.resolved` events carry the same
      positive `seq` and `id` as the persisted row (assert via a fan-out capture
      in the service test).

### Unit 3: Finalize-lock acquisition wrapped in a transaction
**File**: `internal/portal/finalize/lock_acquire.go` (~:187-242)
**Story**: `bug-squash-finalize-lock-no-transaction` (Medium) — depends on Unit 4

The 4 mutations (InsertFinalizeLock → SupersedeFinalizeLock → SetFinalizeLock →
UpdateSessionStatus) run outside any tx; a mid-sequence failure leaves partial
state. Wrap them in `store.WithTx`, preserving the unique-violation→409 path:

```go
err := h.store.WithTx(ctx, func(tx store.TxStore) error {
    if err := tx.InsertFinalizeLock(ctx, ...); err != nil { return err } // bubble ErrUniqueViolation
    if supersedeOldID != "" { if err := tx.SupersedeFinalizeLock(ctx, ...); err != nil { return err } }
    if err := tx.SetFinalizeLock(ctx, ...); err != nil { return err }
    if sess.Status == "active" { if err := tx.UpdateSessionStatus(ctx, ...); err != nil { return err } }
    return nil
})
if errors.Is(err, store.ErrUniqueViolation) && supersedeOldID != "" {
    return openapi.AcquireFinalizeLock409JSONResponse(...), nil // race-lost, tx rolled back
}
if err != nil { return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: acquire lock tx: %w", err)) }
// session.finalizing event emitted AFTER commit (tx-emit-then-fanout) — unchanged
```

**Implementation Notes**: requires `store.TxStore` to expose
`InsertFinalizeLock`/`SupersedeFinalizeLock`/`SetFinalizeLock`/`UpdateSessionStatus`
(verify; add to the TxStore interface + both adapters if missing). The
post-commit `session.finalizing` emit stays outside the tx. Depends on Unit 4
(SQLite `WithTx` correctness) — don't build new tx usage on the primitive being
repaired.

**Acceptance Criteria**:
- [ ] A forced failure at step 2/3/4 rolls back step 1 (no orphaned lock row);
      tested via a failing-Nth-call stub store.
- [ ] The unique-violation race still returns 409 (tx rolled back, no partial state).

### Unit 4: SQLite WithTx acquires the write lock upfront
**Files**: `internal/db/connect.go` (DSN), `internal/db/store/sqlite_adapter.go` (comment)
**Story**: `bug-squash-sqlite-withtx-deferred-not-immediate` (Medium)

`BeginTx(ctx, &sql.TxOptions{})` opens DEFERRED despite the "BEGIN IMMEDIATE"
comment, risking lock-upgrade deadlock under concurrent read-then-write txns.
Add `_txlock=immediate` to the SQLite DSN in `sqliteDSN`:

```go
// connect.go sqliteDSN: ...&_txlock=immediate (with existing _foreign_keys, busy_timeout)
```

**Implementation Notes**: VERIFY modernc.org/sqlite honors `_txlock=immediate`
as a DSN param (consult the sqlc/connect skill). If unsupported, fall back to
capping SQLite `MaxOpenConns=1` (serialize writers — matches the
"effectively single-writer" note in connect.go) and fix the misleading comment.
Either way the code and the comment must agree.

**Acceptance Criteria**:
- [ ] Concurrent read-then-write `WithTx` calls do not spuriously fail with
      SQLITE_BUSY on lock upgrade (test: N goroutines each doing a read-then-write
      tx against a shared sqlite DB; assert all commit).
- [ ] The `WithTx` comment matches the actual begin mode.

### Unit 5: Postgres seq widened to BIGINT
**Files**: `db/schema/postgres.sql` (seq + event_seq.next), regenerated
`internal/db/pgstore/*`, `internal/db/store/postgres_adapter.go` (drop int32 casts),
new goose migration under `internal/db/migrations/`
**Story**: `bug-squash-postgres-seq-32bit` (Low)

```sql
-- new forward migration (non-destructive widening)
ALTER TABLE events     ALTER COLUMN seq  TYPE BIGINT;
ALTER TABLE event_seq  ALTER COLUMN next TYPE BIGINT;
-- down: intentionally no destructive narrowing (would truncate live data)
```

**Implementation Notes**: update `db/schema/postgres.sql` to `BIGINT`, `make
generate` so `AllocateNextSeq*` return `int64`, then DELETE the `int32(p.Seq)` /
`Seq: int32(...)` casts in `postgres_adapter.go`. sqlite already stores 64-bit
INTEGER, so no sqlite change. Per the codex epic-gate caveat: cover BOTH columns,
regen, all casts, an existing-row migration test, and a no-destructive-down policy.

**Acceptance Criteria**:
- [ ] `events.seq` / `event_seq.next` are `BIGINT`; adapter has no `int32` seq casts.
- [ ] An existing-row migration test confirms data preserved across the up migration.

## Implementation Order
1. Unit 4 (SQLite WithTx) before Unit 3 (finalize tx) — declared via Unit 3's
   `depends_on`. 2. Units 1, 2, 5 independent (parallelizable). Units 1 & 2 both
   touch `comments/service.go` (List vs Create/Resolve — different funcs);
   `implement-orchestrator` should bundle them into one worktree to avoid a
   same-file merge conflict.

## Testing
- Unit 1: dual-dialect (sqlite in-memory + postgres testcontainer) page-through
  with tied `created_at`.
- Unit 2: fan-out capture asserts seq/id on the event.
- Unit 3: failing-Nth-call stub store asserts rollback + the 409 path.
- Unit 4: concurrent read-then-write tx stress (no SQLITE_BUSY).
- Unit 5: goose up against a seeded DB; assert rows preserved + BIGINT type.

## Risks
- **Unit 4 driver support**: `_txlock=immediate` support in modernc must be
  verified; `MaxOpenConns=1` is the safe fallback (a small write-throughput hit,
  acceptable for the SQLite default deployment).
- **Unit 5 migration lock**: `ALTER COLUMN TYPE BIGINT` rewrites the table on
  Postgres (brief lock). Acceptable at current scale; note in the migration.

## Design decisions
- **Keyset over offset**: `(created_at, id)` keyset, not offset pagination —
  stable under concurrent inserts and matches the cursor's existing fields.
- **Finalize tx scope**: wrap only the 4 mutations; keep the `session.finalizing`
  emit post-commit (tx-emit-then-fanout). The 409 race path returns after a clean
  rollback.
- **SQLite immediacy**: prefer `_txlock=immediate` (matches the comment's intent)
  with `MaxOpenConns=1` as the verified fallback.
- **seq widening**: forward-only, non-destructive; no narrowing down-migration.

## Other agent review

_Codex (xhigh) feature peer-review gate pending._
