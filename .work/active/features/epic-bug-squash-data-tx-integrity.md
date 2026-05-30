---
id: epic-bug-squash-data-tx-integrity
kind: feature
stage: drafting
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

<!-- feature-design fills in the keyset query rewrite, the migration plan, and
the dual-dialect test matrix (sqlite + postgres via testcontainers). -->
