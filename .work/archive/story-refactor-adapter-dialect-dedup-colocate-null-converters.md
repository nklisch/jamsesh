---
id: story-refactor-adapter-dialect-dedup-colocate-null-converters
kind: story
stage: done
tags: [portal, refactor]
parent: feature-refactor-adapter-dialect-dedup
depends_on: []
release_binding: v0.4.0
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Co-locate dialect null/text/time converters in a shared file

## Brief

The 8 null/text/time converter helpers (4 in `sqlite_adapter.go` lines
67-99, 4 in `postgres_adapter.go` lines 66-98) are structurally
identical apart from the dialect-specific source type. Both source
types (`sql.NullString`/`sql.NullTime` vs `pgtype.Text`/`pgtype.Timestamptz`)
expose `.Valid bool` and a value field of the same name (`.String` /
`.Time`).

Go generics can't cleanly bind across these because the field-access
pattern isn't expressible via method constraints. But co-locating
the 8 helpers in a single file makes the structural duplication
visible and creates a single home for a future generics rewrite or
code-gen pass.

This is the conservative slice of the parent feature; the deeper
structural dedup (row converters, method wrappers) is deferred — see
the feature body for rationale.

## Current state

```go
// internal/db/store/sqlite_adapter.go lines 67-99
func nullStringToPtr(ns sql.NullString) *string { ... }
func ptrToNullString(s *string) sql.NullString  { ... }
func nullTimeToPtr(nt sql.NullTime) *time.Time  { ... }
func ptrToNullTime(t *time.Time) sql.NullTime   { ... }

// internal/db/store/postgres_adapter.go lines 66-98
func pgTextToPtr(t pgtype.Text) *string                  { ... }
func ptrToPgText(s *string) pgtype.Text                  { ... }
func pgTimestamptzToPtr(ts pgtype.Timestamptz) *time.Time { ... }
func ptrToPgTimestamptz(t *time.Time) pgtype.Timestamptz { ... }
```

## Target state

```go
// internal/db/store/nullable_converters.go (new file)
//
// Null/text/time converters used by the dialect-specific adapters.
// Co-located here so the structural similarity across dialects is
// visible and any future deduplication (generics-based helpers,
// code-gen) has a single home.
package store

import (
    "database/sql"
    "time"

    "github.com/jackc/pgx/v5/pgtype"
)

// SQLite-side helpers — sql.NullString / sql.NullTime.
func nullStringToPtr(ns sql.NullString) *string { ... }
func ptrToNullString(s *string) sql.NullString  { ... }
func nullTimeToPtr(nt sql.NullTime) *time.Time  { ... }
func ptrToNullTime(t *time.Time) sql.NullTime   { ... }

// Postgres-side helpers — pgtype.Text / pgtype.Timestamptz.
func pgTextToPtr(t pgtype.Text) *string                  { ... }
func ptrToPgText(s *string) pgtype.Text                  { ... }
func pgTimestamptzToPtr(ts pgtype.Timestamptz) *time.Time { ... }
func ptrToPgTimestamptz(t *time.Time) pgtype.Timestamptz { ... }
```

## Implementation notes

- Move the 8 functions verbatim — no signature changes, no logic
  changes.
- Both adapter files lose ~30 lines each; net LoC change is roughly
  zero (the helpers just live in one file now), but the duplication
  becomes a single point of failure when changes are needed.
- Imports in the adapter files lose `database/sql` (sqlite) and
  `pgtype` (postgres) iff those imports were only used by the
  helpers — verify before removing.
- The package is the same (`store`), so the adapter files continue
  to call the helpers without any import path changes.
- Add a top-of-file comment block documenting why the helpers are
  co-located and what the deferred next step is (generics-based
  unification, or code-gen).

## Acceptance criteria

- [ ] `internal/db/store/nullable_converters.go` exists with all 8
      functions.
- [ ] `sqlite_adapter.go` and `postgres_adapter.go` no longer define
      these functions.
- [ ] `go build ./...` clean.
- [ ] `go test ./internal/db/store/...` clean.
- [ ] Full `go test ./...` clean (the adapters are used everywhere).

## Risk

**Very low.** Move-only refactor — no signature changes, no logic
changes, single Go package.

## Rollback

`git revert` the commit.

## Implementation notes

Moved all 8 functions verbatim with no signature or logic changes.

- `internal/db/store/nullable_converters.go` — new file, all 8 helpers,
  package-level comment documenting the co-location rationale and deferred
  generics/code-gen next step.
- `internal/db/store/sqlite_adapter.go` — removed lines 67–99 (4 helpers).
  `database/sql` and `time` imports retained: both are still used by
  `mapSQLiteErr` (`sql.ErrNoRows`), the `sqliteAdapter` struct (`*sql.DB`),
  and `sqliteArchivedSession` (`var endedAt time.Time`).
- `internal/db/store/postgres_adapter.go` — removed lines 66–98 (4 helpers).
  `pgtype` and `time` imports retained: `pgtype` is used in
  `pgArchivedSession` (`row.ArchivedAt.Valid` / `.Time` on a
  `pgtype.Timestamptz`); `time` is used for the `endedAt`/`archivedAt`
  local variables in the same mapper.

Build: `go build ./...` clean.
Tests: `go test ./...` — all packages pass (57 packages, 0 failures).

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Behavior-preserving refactor delivered as designed. Implementation notes document any deviations (typically agent adapting to the file's actual structure differing from the story body's assumption). All tests pass; build clean.
