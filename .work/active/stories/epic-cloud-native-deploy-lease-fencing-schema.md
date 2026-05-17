---
id: epic-cloud-native-deploy-lease-fencing-schema
kind: story
stage: implementing
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
