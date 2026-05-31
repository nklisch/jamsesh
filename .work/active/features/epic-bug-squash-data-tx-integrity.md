---
id: epic-bug-squash-data-tx-integrity
kind: feature
stage: review
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
-- postgres (named/numbered ok): $1 = before_created_at, $2 = before_id
WHERE session_id = $3 /* ...filters... */
  AND (created_at < $1 OR (created_at = $1 AND id < $2))
ORDER BY created_at DESC, id DESC LIMIT $4;
-- sqlite: ANONYMOUS positional `?` ONLY (codex: `?1` aliases earlier params).
-- Pass the boundary created_at value TWICE (or use sqlc named params :before/:lastid):
WHERE session_id = ? /* ...filters... */
  AND (created_at < ? OR (created_at = ? AND id < ?))
ORDER BY created_at DESC, id DESC LIMIT ?;
```

Thread `cur.LastID` through the `*Params` structs and both adapters; callers
pass `cur.LastCreatedAt()` (twice for sqlite) AND `cur.LastID`.

**First-page sentinel (codex must-fix)**: there is NO separate first-page query
branch — the first page passes a `now()+1s` `created_at` sentinel
(`comments/service.go` List ~:364, `sessions/listing.go` ~:67). With the new
`AND id < ?` clause, the first page must ALSO pass a **max-id sentinel** that
sorts after every ULID (e.g. a string of 0xFF / `"~"`-padded, or
`strings.Repeat("z", 26)`) so all rows still qualify. Set both sentinels in the
no-cursor path.

**Implementation Notes**: sqlite anonymous `?` cannot use `?1`; either repeat
the boundary value or switch the query to sqlc named params. Keep the two
dialect files mirrored in column/filter/order semantics
(`dual-dialect-mirror-queries`). ULID `id` DESC is a stable unique tiebreaker;
add a cross-dialect collation-parity assertion in the test.

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

The acquire mutations run outside any tx; a mid-sequence failure leaves partial
state. **Codex must-fix: the tx must span the WHOLE sequence**, including the
earlier `ReleaseFinalizeLock` in the stale/override branch (~`lock_acquire.go:131`)
— otherwise a rollback won't restore the old active lock. Wrap from the release
through the status update:

```go
err := h.store.WithTx(ctx, func(tx store.TxStore) error {
    if needRelease { if err := tx.ReleaseFinalizeLock(ctx, ...); err != nil { return err } } // override/stale branch
    if err := tx.InsertFinalizeLock(ctx, ...); err != nil { return err } // bubbles ErrUniqueViolation
    if supersedeOldID != "" { if err := tx.SupersedeFinalizeLock(ctx, ...); err != nil { return err } }
    if err := tx.SetFinalizeLock(ctx, ...); err != nil { return err }
    if sess.Status == "active" { if err := tx.UpdateSessionStatus(ctx, ...); err != nil { return err } }
    return nil
})
// Codex must-fix: the active-lock unique index can be hit by a fresh-insert race
// too, not only when superseding — return 409 on ErrUniqueViolation regardless.
if errors.Is(err, store.ErrUniqueViolation) {
    return openapi.AcquireFinalizeLock409JSONResponse(...), nil // race-lost, tx rolled back
}
if err != nil { return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: acquire lock tx: %w", err)) }
// session.finalizing event emitted AFTER commit (tx-emit-then-fanout) — unchanged
```

**Implementation Notes**: `store.TxStore` already exposes the needed
finalize/session methods via composed interfaces (codex verified) and both tx
adapters implement them — no interface widening needed. `WithTx` returns the
closure error unchanged, so `ErrUniqueViolation` survives the boundary. The
post-commit `session.finalizing` emit stays outside the tx. Depends on Unit 4
(SQLite `WithTx` correctness) — don't build new tx usage on the primitive being
repaired. Decouple the pre-flight READS (existing-lock lookup, session load)
from the WithTx WRITE block; only the mutations belong in the tx.

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

**Implementation Notes**: codex verified `modernc.org/sqlite v1.50.1` honors
`_txlock=immediate` (the driver emits `BEGIN IMMEDIATE`); `busy_timeout` still
applies while waiting for the write lock. So adding `_txlock=immediate` to the
DSN is the fix — no fallback needed, though `MaxOpenConns=1` remains an optional
defense-in-depth for the single-file SQLite default. Make the `WithTx` comment
match.

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
generate` so `AllocateNextSeq*` return `int64`, then DELETE **every** seq-related
`int32` cast in `postgres_adapter.go` — codex enumerated: `AllocateNextSeqN`,
`InsertEvent`, `ListEventsSince`, AND `ListEventsSinceForDigest`, in BOTH the
outer adapter and the tx adapter. sqlite already stores 64-bit INTEGER, so no
sqlite change. Cover both columns, regen, all casts, an existing-row migration
test, and a no-destructive-down policy.

**Out-of-scope sibling (codex finding — deferred, NOT in this feature)**: the
tombstone aggregate fields (`db/schema/postgres.sql` ~:284) are domain `int64`
but Postgres `INTEGER` with `int32` casts — the same schema/domain mismatch
class, but NOT one of the 28 bug-scan findings. Explicitly deferred: surfaced in
the autopilot run summary for the user to park as a new story rather than
expanding this feature's scope.

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
- **Unit 4**: `_txlock=immediate` confirmed supported by modernc v1.50.1 (codex);
  `MaxOpenConns=1` is optional defense-in-depth, not a required fallback.
- **Unit 5 migration lock**: `ALTER COLUMN TYPE BIGINT` rewrites the table on
  Postgres (brief lock). Acceptable at current scale; note in the migration.
- **Unit 1 first-page sentinel**: forgetting the max-id sentinel would silently
  drop rows on page 1 — covered by the page-through test starting from no cursor.

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

Codex (cross-model, xhigh) reviewed this design. Verdict: request design changes
before implementation — applied below. Codex verified facts: modernc v1.50.1
supports `_txlock=immediate`; `TxStore` already exposes the finalize/session
methods; no other `created_at < cursor` paginated queries exist (activity/digest
are seq-based); ULID `id` DESC is a valid tiebreaker.

**Accepted & applied:**
- **Unit 1**: SQLite must use anonymous `?` (not `?1` — it aliases); pass the
  boundary `created_at` twice or use named params. Added the missing **max-id
  first-page sentinel** (the no-cursor path uses a `now()+1s` time sentinel and
  now also needs a max-id so `AND id < ?` admits all page-1 rows).
- **Unit 3**: widened the tx to span the WHOLE acquire sequence including the
  earlier `ReleaseFinalizeLock` (override/stale branch ~:131) — else rollback
  won't restore the prior lock. 409 now triggers on `ErrUniqueViolation`
  regardless of `supersedeOldID` (fresh-insert races hit the unique index too).
- **Unit 5**: enumerated ALL seq `int32` casts to drop — `AllocateNextSeqN`,
  `InsertEvent`, `ListEventsSince`, `ListEventsSinceForDigest`, outer + tx.
- **Unit 4**: confirmed `_txlock=immediate` is the fix (no fallback needed).

**Deferred (out of scope, recorded for the run summary):** tombstone aggregate
fields are the same Postgres-INTEGER vs domain-int64 mismatch (schema ~:284) but
not a bug-scan finding — to be parked as a separate story by the user.

## Implementation summary

All 5 child stories implemented and advanced to `stage: review` (per-story `implement: bug-squash-*` commits). Each landed a failing-first regression test; the codex feature-gate findings (see `## Other agent review`) were applied during design and honored in implementation. Verification at the orchestrator level: `go build ./...` + `go vet` clean; backend `-race`/package tests and frontend `vitest` (764 passing) + `svelte-check` green; `sqlc generate` matches spec.
