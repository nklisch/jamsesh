---
id: feature-refactor-adapter-dialect-dedup
kind: feature
stage: drafting
tags: [portal, refactor]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Reduce sqlite_adapter / postgres_adapter wrapper boilerplate

## Brief

`internal/db/store/sqlite_adapter.go` (2335 lines) and
`internal/db/store/postgres_adapter.go` (2333 lines) define ~106
wrapper methods each. Every wrapper has the same shape — call the
dialect-specific querier, run `mapSQLiteErr` / `mapPostgresErr`, run
a per-row converter, return the domain type. The dialect-specific
querier types differ (sqlitestore vs pgstore) and the null/text/time
converters use dialect-specific source types (`sql.NullString` vs
`pgtype.Text`, `sql.NullTime` vs `pgtype.Timestamptz`), so the
duplication is structural rather than character-for-character.

The dual-dialect convention is intentional and documented under
`.claude/skills/patterns/dual-dialect-mirror-queries.md`. **This
feature does not propose abandoning that pattern.** It asks whether
the *adapter layer* on top of the generated queries can shrink without
violating the mirror-queries discipline.

Surfaced by a discovery-mode `/agile-workflow:refactor-design` scan.

## Specific observations

- `nullStringToPtr` / `ptrToNullString` / `nullTimeToPtr` /
  `ptrToNullTime` (sqlite_adapter.go lines 67-99) and the matching
  pgtype variants (postgres_adapter.go lines 66-98) are structurally
  identical, differing only in source type. Go generics could collapse
  them.
- ~50 row-converter functions (`sqliteOrg`, `sqliteSession`, ...) per
  dialect mostly do a field-by-field copy with one or two null
  conversions. The dialects' generated types are similar enough that
  a single `convertOrg(genericRow)` could work via a tiny adapter
  interface OR via code generation alongside sqlc.
- ~50 method wrappers per dialect (`GetOrgByID`, `CreateSession`, ...)
  are nearly mechanical: call querier, map error, convert row.

## Design questions for feature-design

- **Approach choice (load-bearing):**
  - Generics-based shared helpers + thinner adapters
  - Code-gen step alongside sqlc that emits the wrappers
  - Status quo + a CI lint that asserts the two adapter files stay
    structurally aligned
  - Hybrid (helpers for null converters only; leave row converters and
    method wrappers)
- The dialects are not fully interchangeable (Postgres has
  `pgtype.Text`, `pgtype.Timestamptz`, fencing token write paths
  etc.). What is the minimum viable shared surface that doesn't
  paper over real dialect differences?
- Risk: this is a wide-blast-radius refactor. How do we phase it so a
  break in one dialect doesn't tank the other?

## Acceptance criteria (target)

- Combined LoC of the two adapter files reduced by at least 30%.
- Domain semantics unchanged — `dual-dialect-mirror-queries` invariants
  still hold.
- `go test ./internal/db/...` clean on both dialects.
- Existing portal integration tests pass against both dialects.

## Notes

Behavior-preserving target. Because the blast radius is wide and the
existing pattern is documented as intentional, this feature explicitly
requires a design pass before any implementation — no autopilot
shortcuts.
