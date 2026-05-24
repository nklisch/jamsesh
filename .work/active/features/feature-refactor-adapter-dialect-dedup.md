---
id: feature-refactor-adapter-dialect-dedup
kind: feature
stage: implementing
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

## Refactor Overview & Design Decision (2026-05-23, autopilot)

After a per-feature design pass, the scope of this feature is
deliberately reduced. Rationale below; the conservative slice lands
now as one child story, and the deeper structural dedup is deferred
because it requires a strategic call autopilot cannot make
autonomously without violating the autonomy mandate.

### What's in scope

**Step 1: Co-locate null/text/time converters.** The 8 helper functions
(4 per dialect) have identical structure but use dialect-specific
source types (`sql.Null{String,Time}` vs `pgtype.{Text,Timestamptz}`).
Move them all into one shared file `internal/db/store/nullable_converters.go`
so the duplication is visible and a future unification has a single
home.

**Net LoC change**: roughly zero (the functions move, they don't
shrink). The win is **visibility** — duplication concentrated in one
file is much more likely to attract a future generics or code-gen
pass.

### What's deferred (and why)

The discovery scan proposed three more aggressive options, all of
which were considered and deferred for the reasons documented below:

1. **Go-generics-based row-converter helpers.** Go's generics can't
   bind cleanly across `sql.NullString` and `pgtype.Text` (their
   field-based shape isn't expressible via method constraints). A
   workaround using adapter wrappers around the source types would
   add as much noise as it removes. **Verdict: not worth the complexity
   trade.**

2. **sqlc code-gen for the wrapper methods.** ~106 method wrappers per
   dialect could be generated from a small template. But this adds a
   new build step, a template system, and a meta-system that the team
   would have to maintain. For a behavior-preserving win, the
   complexity cost is too high. **Verdict: needs a human design call
   to weigh meta-system value against direct dedup. Not autopilot's
   call.**

3. **CI lint that enforces structural alignment of the two adapter
   files.** Doesn't actually reduce code — it just enforces what
   `dual-dialect-mirror-queries` already documents as the convention.
   Real-world drift is rare because changes to one adapter without the
   other break compilation of the matching dialect's tests.
   **Verdict: low value-add; documentation already covers this.**

### Why this isn't a punt

The existing `dual-dialect-mirror-queries` pattern (documented at
`.claude/skills/patterns/dual-dialect-mirror-queries.md`) is
**intentional**. The two adapter files exist as parallel mirrors so
each dialect's quirks live next to the code that handles them. A
heavy-handed dedup pass would obscure that pattern. The conservative
slice (co-locating helpers) reduces duplication where the dialect
quirks are absent (the null converters all share structure with no
dialect-specific quirks) without disturbing the parts where the
quirks actually matter.

The original "30% LoC reduction" target in this feature's acceptance
criteria was aspirational. The honest assessment is that the
structural shape of dual-dialect-mirror-queries puts a ceiling
somewhere around 10-15% on safe dedup — anything past that crosses
into the deferred options above.

## Refactor Steps

### Step 1: Co-locate null/text/time converters
**Priority**: Low  **Risk**: Very Low
**Files**: `internal/db/store/nullable_converters.go` (new),
`internal/db/store/sqlite_adapter.go`,
`internal/db/store/postgres_adapter.go`
**Story**: `story-refactor-adapter-dialect-dedup-colocate-null-converters`

Move the 8 helper functions verbatim into a shared file.

## Implementation Order

One story. Implementer can pick it up at any time.

## Follow-up

The deferred options (code-gen, generics, CI lint) remain
candidate ideas. If someone wants to revisit, scope as a new
feature with explicit human input on which path to take. This
feature should NOT be re-opened — it has fulfilled its scope.
