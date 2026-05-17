---
id: epic-portal-git-storage-archive-and-stub
kind: story
stage: done
tags: [portal]
parent: epic-portal-git-storage
depends_on: [epic-portal-git-storage-bare-repo-helpers]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Git Storage — Archive Operation and Stub Formatter

## Scope

Add the `archived_sessions` schema and migration, the archive
helper that moves a live session to the archived state, and the
stub-response formatter used by both REST and git smart-HTTP for
410-Gone responses.

## Units delivered

- `internal/db/migrations/sqlite/00002_archived_sessions.sql`
- `internal/db/migrations/postgres/00002_archived_sessions.sql`
- `db/schema/sqlite.sql` (edit — append CREATE TABLE)
- `db/schema/postgres.sql` (edit — append CREATE TABLE)
- `db/queries/sqlite/archived_sessions.sql` + postgres variant
- Regenerate sqlitestore + pgstore via `make generate-db`
- `internal/db/store/store.go` (edit — add `ArchivedSessionStore`
  sub-interface methods: `InsertArchivedSession`, `GetArchivedSession`,
  `DeleteSession`)
- Update sqliteAdapter + postgresAdapter to satisfy the new
  Store methods
- `internal/portal/storage/archive.go` — `ArchiveSession`,
  `LookupArchived`, the `ArchiveInfo` and `ArchivedRecord` types
- `internal/portal/storage/stub.go` — `StubResponse` formatter
- Tests for archive end-to-end + stub formatter

## Acceptance Criteria

- [ ] `make generate-db && git diff --exit-code` is green after
      the new query files land
- [ ] `MigrateUp` on a fresh SQLite + Postgres applies `00002_*`
      and creates the `archived_sessions` table
- [ ] `MigrateUp` is idempotent across multiple invocations (goose
      handles this)
- [ ] `ArchiveSession` succeeds end-to-end against a freshly
      created session: row inserts in `archived_sessions`, bare
      repo is removed, original `sessions` row is deleted (with
      CASCADE cleaning up `session_members`)
- [ ] Re-running `ArchiveSession` on an already-archived session
      is a no-op (returns nil; verified by inspecting that the
      `archived_sessions` row is unchanged)
- [ ] `LookupArchived` returns `*ArchivedRecord` for an archived
      session, `ErrNotFound` for a live or non-existent session
- [ ] `StubResponse` produces a struct with `Error:
      "session.archived"`, `HTTPStatus: 410`, and a `Details`
      payload carrying `archived_at`, `final_branch_name` (omitted
      when nil), `end_reason`
- [ ] Tests green: `go test ./internal/portal/storage/... ./internal/db/...`

## Notes

- The Store-interface extension adds methods; existing callers
  are unaffected. The adapters need new wrappers per method.
- `DeleteSession` cascades through `session_members` via the FK
  constraint on the schema (verify in the test that no orphan
  rows remain).
- `archived_sessions.member_account_ids` is stored as JSON string
  (TEXT in SQLite, TEXT in Postgres). The domain type
  `[]string` is marshaled at write and unmarshaled at read.
- The stub formatter's `final_branch_name` field uses
  `omitempty` JSON tag — when nil, the field is absent from the
  serialized response.

## Implementation notes

- New files: `db/queries/sqlite/archived_sessions.sql`,
  `db/queries/postgres/archived_sessions.sql`,
  `internal/db/migrations/sqlite/00002_archived_sessions.sql`,
  `internal/db/migrations/postgres/00002_archived_sessions.sql`.
- Edited schema files: `db/schema/sqlite.sql`, `db/schema/postgres.sql`
  (appended `archived_sessions` table + index).
- Added `DeleteSession :exec` to both sessions.sql query files.
- `sqlc generate` regenerated: `internal/db/sqlitestore/archived_sessions.sql.go`,
  `internal/db/pgstore/archived_sessions.sql.go`, plus updated models.go,
  querier.go, sessions.sql.go in both stores.
- `internal/db/store/store.go`: added `ArchivedSession` domain type,
  `InsertArchivedSessionParams`, `GetArchivedSessionParams`,
  `DeleteSessionParams`, `ArchivedSessionStore` sub-interface, `DeleteSession`
  on `SessionStore`, embedded `ArchivedSessionStore` in master `Store`.
- `internal/db/store/sqlite_adapter.go` and `postgres_adapter.go`: implemented
  all new methods. SQLite adapter JSON-decodes `member_account_ids` on read,
  stores encoded on write. Postgres adapter converts `pgtype.Timestamptz` ↔
  `time.Time` for `archived_at`.
- sqlc global `*.ended_at` override makes `EndedAt` a `*time.Time` even for
  NOT NULL `archived_sessions.ended_at`; adapters dereference the pointer
  (will always be non-nil in practice).
- `internal/portal/storage/service.go`: rewrote with proper types (removed
  `interface{}` stubs from previous story).
- `internal/portal/storage/archive.go`: `ArchiveSession` + `LookupArchived`.
  Archive order: INSERT archived row (idempotent on unique violation) → RemoveRepo
  → DeleteSession.
- `internal/portal/storage/stub.go`: `StubResponse` formatter.
- `internal/portal/storage/archive_test.go`: end-to-end tests covering
  full archive lifecycle, re-archive no-op, ErrNotFound paths, StubResponse
  table tests (with/without final_branch_name). All pass.

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Schema-additive 00002 migration on both dialects clean. Adapter JSON encode/decode for member_account_ids handled in both dialects. Archive ordering correct per locked decision (INSERT → RemoveRepo → DeleteSession). Idempotent re-archive verified. The DeleteSession addition to the Store interface is a clean extension.
