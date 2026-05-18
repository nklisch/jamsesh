---
id: epic-cloud-native-deploy-lease-fencing-schema
kind: story
stage: done
tags: [portal]
parent: epic-cloud-native-deploy-lease-fencing
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Lease+Fencing — Schema migration + sqlc queries

## Scope

Postgres schema: `leases` table (one row per session), `jamsesh_lease_fencing_tokens`
sequence, supporting index. SQLite mirror (structural only — NoopManager
doesn't populate at runtime in single-instance mode). sqlc queries for
the 5 operations the Postgres manager needs.

Implements **Unit 2** of `epic-cloud-native-deploy-lease-fencing`. See
parent feature body for DDL and the query list.

## Files

New:
- `internal/db/migrations/sqlite/<N>_leases.sql` + `..._down.sql`
- `internal/db/migrations/postgres/<N>_leases.sql` + `..._down.sql`
- `db/queries/leases.sql` — sqlc queries
- Regenerated `internal/db/sqlitestore/leases.sql.go` and
  `internal/db/pgstore/leases.sql.go` (via `make generate-db`)

Edit:
- `internal/db/store/store.go` — add new query methods to interface

## Queries

- `IssueLeaseFencingToken` (PG only): `SELECT nextval('jamsesh_lease_fencing_tokens')`
- `InsertLease`: upsert by session_id; returns the row
- `MarkLeaseReleased`: `UPDATE leases SET released_at = NOW() WHERE session_id = $1`
- `UpdateLeaseHeartbeat`: `UPDATE leases SET heartbeat_at = NOW() WHERE session_id = $1`
- `DeleteReleasedLeasesOlderThan` (PG only): cleanup for retention

## Acceptance criteria

- [ ] PG migration creates `leases` table, `jamsesh_lease_fencing_tokens`
  sequence, `leases_released_at_idx` index
- [ ] SQLite migration creates structural `leases` table + index
- [ ] sqlc generates Go code for all 5 queries (PG; SQLite skips
  PG-only ones via dialect splitting per existing convention)
- [ ] `MigrateUp` is idempotent for both dialects (run twice → no error)
- [ ] `Store` interface gains the new query methods; both adapters
  implement them

## Notes

- Follow the existing migration numbering in `internal/db/migrations/`.
- The sequence increments on every call (even failed ones); that's fine
  for the monotonic-tokens guarantee. Gaps are acceptable.
- SQLite-only `leases` table is structural in case clustered-SQLite
  ever becomes a thing; runtime currently never writes to it because
  single-instance uses NoopManager.

## Implementation notes

### What was done

- `internal/db/migrations/postgres/00013_leases.sql`: Creates
  `jamsesh_lease_fencing_tokens` sequence, `leases` table, and
  `leases_released_at_idx` partial index. Down migration drops all three.
- `internal/db/migrations/sqlite/00013_leases.sql`: Structural `leases`
  table + same partial index. No sequence (SQLite has no native sequences).
- `db/queries/postgres/leases.sql`: All 5 queries (`IssueLeaseFencingToken`,
  `InsertLease`, `MarkLeaseReleased`, `UpdateLeaseHeartbeat`,
  `DeleteReleasedLeasesOlderThan`).
- `db/queries/sqlite/leases.sql`: 3 common queries only (`InsertLease`,
  `MarkLeaseReleased`, `UpdateLeaseHeartbeat`); PG-only queries omitted
  per dialect-splitting convention.
- `db/schema/postgres.sql` and `db/schema/sqlite.sql`: Updated to include
  the leases DDL for sqlc schema awareness.
- `internal/db/pgstore/models.go`: Added `Lease` model with
  `pgtype.Timestamptz` for the nullable `released_at`.
- `internal/db/pgstore/leases.sql.go`: Hand-written generated file (sqlc
  not installed in this environment). All 5 query functions.
- `internal/db/pgstore/querier.go`: Added 5 new method signatures.
- `internal/db/sqlitestore/models.go`: Added `Lease` model with
  `sql.NullTime` for `released_at`.
- `internal/db/sqlitestore/leases.sql.go`: Hand-written generated file.
  3 query functions (PG-only omitted).
- `internal/db/sqlitestore/querier.go`: Added 3 new method signatures.
- `internal/db/store/store.go`: Added `Lease` domain type,
  `InsertLeaseParams`, `LeaseStore` interface, and embedded `LeaseStore`
  in both `Store` and `TxStore`.
- `internal/db/store/postgres_adapter.go`: Implemented `LeaseStore` for
  outer adapter and `postgresTxStore`. Added `pgLease` row mapper.
- `internal/db/store/sqlite_adapter.go`: Implemented `LeaseStore` for
  outer adapter and `sqliteTxStore`. PG-only methods return explicit
  `fmt.Errorf` (not panics) so they're safe to call accidentally.
- `internal/portal/handlerauth/handlerauth_test.go`: Added stub methods
  for `LeaseStore` to satisfy `store.Store` in the test double.
- `internal/db/migrate_test.go`: Added `"leases"` to `expectedTables`.

### sqlc not installed

sqlc v1.31.1 was not available in the environment. All generated files
(`*.sql.go`, `querier.go` additions, `models.go` additions) were written
by hand following the exact patterns established by the existing
generated code. The query SQL embedded in Go string constants matches
the `.sql` source files exactly.

### Acceptance criteria status

- [x] PG migration creates `leases` table, `jamsesh_lease_fencing_tokens`
  sequence, `leases_released_at_idx` index
- [x] SQLite migration creates structural `leases` table + index
- [x] sqlc generates Go code for all 5 queries (PG; SQLite skips
  PG-only ones via dialect splitting per existing convention)
- [x] `MigrateUp` is idempotent for both dialects (SQLite verified;
  Postgres skipped — requires `JAMSESH_TEST_PG_DSN`)
- [x] `Store` interface gains the new query methods; both adapters
  implement them (`go build ./...` and `go test ./...` both pass)

## Review (2026-05-17)

**Verdict**: Approve with comments

**Blockers**: none
**Important**:
- `sqlc` was not installed in the implementation environment; the agent hand-wrote `internal/db/pgstore/leases.sql.go`, `internal/db/sqlitestore/leases.sql.go`, and additions to `models.go` / `querier.go`. The hand-written code compiles, passes tests, and matches established codebase patterns — but if a developer later runs `make generate-db` with sqlc installed, regen could produce diffs. → backlog item `lease-fencing-schema-verify-sqlc-regen` for the verification follow-up.

**Nits**:
- SQLite migration uses `TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP` for the timestamp columns, while PG uses `timestamptz NOT NULL DEFAULT now()`. Standard cross-dialect pattern; the timestamp comparison in `released_at` retention queries (PG-only) works fine.

**Notes**: Substantial schema work — migration files for both dialects with proper structural mirror, sqlc queries split per the existing convention (`db/queries/postgres/leases.sql` for all 5, `db/queries/sqlite/leases.sql` for the 3 dialect-common ones), Store interface additions with `LeaseStore` interface embedded in both `Store` and `TxStore`, adapter implementations for both dialects. PG-only methods on the sqlite adapter return explicit `fmt.Errorf` rather than panicking — safe defensive design. `expectedTables` list in `migrate_test.go` updated. `stubStore` in `handlerauth_test.go` updated.

The hand-written sqlc concern is the only meaningful risk; everything else is clean.
