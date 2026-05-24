---
id: story-anon-bearer-test-integrity-migration-updownup
kind: story
stage: done
tags: [testing, migrations]
parent: feature-anon-bearer-test-integrity
depends_on: []
release_binding: v0.4.0
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Implement actual Up-Down-Up cycle in TestMigrate00016_AnonymousBearers

## Brief

The test `TestMigrate00016_AnonymousBearers_UpDownUp` in
`internal/db/migrate_test.go` is named for an Up-Down-Up cycle but the body
only runs `MigrateUp` once, inserts rows, and asserts the columns/CHECK
exist. It never invokes goose Down and never re-applies Up. The acceptance
criterion in Unit 1 of `feature-epic-ephemeral-playground-anon-bearer`
called for:

> Goose `down` migration reverses cleanly (verify in a test)

…and the SQLite Down migration is non-trivial (table-rebuild dance to drop
`is_anonymous` from accounts, rebuild `oauth_tokens` without `session_id`
and without the new CHECK kind). The hand-written Down path deserves
exercise.

## Scope (from parent feature design)

This story implements **Units 1 + 2** of
`feature-anon-bearer-test-integrity`. Read the parent feature body for the
full design and code; this section is the short version.

**Two additions, both in `internal/db/migrate_test.go`:**

### 1. Test-only `migrateDown` helper (parent Unit 1)

Unexported. Takes `testing.TB`. Replicates the provider-construction
block from `MigrateUp(ctx, db, dialect)` (switch on dialect → embed.FS +
goose.Dialect → `fs.Sub` → `goose.NewProvider`), then calls
`provider.DownTo(ctx, version int64)`.

Confirmed signature on goose v3.27.1:
`(*Provider).DownTo(ctx, version int64) ([]*MigrationResult, error)`
— "rolls back all migrations down to, but not including, the specified
version" (so `DownTo(16)` leaves the DB at version 15).

Why locally re-derive the Provider instead of factoring a shared helper
out of `MigrateUp`? Production `MigrateUp` should not expose its Provider
just for tests; one test isn't enough demand to justify the public
surface. If a second migration test arrives, factor then.

Why `testing.TB` and `t.Fatalf` everywhere? Fail-fast — callers would
`t.Fatal` on any error anyway.

### 2. Extend the test body with Down + re-Up (parent Unit 2)

The existing body's setup (org, session, pre-migration token, new-kind
token) all stays — it's the precondition for the Down step. After the
current post-Up assertions, append:

- `migrateDown(t, ctx, db, "sqlite", 16)` — DB goes back to version 15.
- Assert `is_anonymous` column gone (INSERT with that column fails).
- Assert `session_id` column gone from `oauth_tokens` (INSERT fails).
- Assert all `anonymous_session_bearer` rows deleted
  (`SELECT COUNT(*) WHERE kind='anonymous_session_bearer'` → 0).
- Assert pre-migration `access` token (tok-001) survives the Down.
- `MigrateUp(ctx, db, "sqlite")` — re-apply 00016.
- Assert post-migration shape restored (insert a new
  `anonymous_session_bearer` row; must succeed).

Order matters: assert `anonymous_session_bearer` rows are gone **before**
re-Up. Otherwise the assertion proves nothing.

Postgres Down NOT exercised here. The PG Down is straightforward
`DROP COLUMN`; if a parity test is wanted later, file a follow-up.

## Acceptance criteria

- [ ] `migrateDown(t, ctx, db, dialect, version)` helper exists in
      `internal/db/migrate_test.go`, unexported, takes `testing.TB`.
- [ ] Helper is NOT referenced from any non-`_test.go` file under
      `internal/db/`.
- [ ] `TestMigrate00016_AnonymousBearers_UpDownUp` body runs Up → Down → Up
      and asserts the relevant invariants at each step.
- [ ] Down phase deletes all `anonymous_session_bearer` rows BEFORE
      re-applying Up.
- [ ] Pre-migration tokens (kind='access') survive Down without data loss.
- [ ] Re-Up restores the post-migration shape (new-kind insert succeeds).
- [ ] `go test ./internal/db/...` green (SQLite path).
- [ ] With `JAMSESH_TEST_PG_DSN` set, `go test ./internal/db/...` still
      green (the existing Postgres migration tests must not regress).
- [ ] Manual smoke (optional, don't commit): break a SQL statement in
      00016's Down section; the test must fail. Confirms the test exercises
      Down.

## Independence

This story is independent of
`story-anon-bearer-test-integrity-transactional-rollback` — different
package, different file, different invariants. No `depends_on`; can land
in parallel.

## Source

Surfaced during review of
`feature-epic-ephemeral-playground-anon-bearer`. Filed under the
test-integrity discipline in CLAUDE.md.

## Implementation notes (2026-05-23)

Both additions landed in `internal/db/migrate_test.go`:

1. **`migrateDown(t, ctx, db, dialect, version)` helper** — unexported,
   takes `testing.TB`, re-derives the goose Provider locally (same switch
   on dialect → embed.FS → fs.Sub → goose.NewProvider as `MigrateUp`).
   Uses `t.Fatalf` for fail-fast on every error.

2. **`TestMigrate00016_AnonymousBearers_UpDownUp` body extension** —
   appended Down + re-Up steps after the existing post-Up assertions.
   Order of Down assertions matters: `anonymous_session_bearer` rows
   asserted gone BEFORE re-Up so the assertion can't be satisfied by
   the re-Up's re-creation.

### Goose-semantics correction

Story design said `DownTo(16)` would leave the DB at version 15 ("rolls
back all migrations down to, but not including, the specified version").
Actual goose v3.27.1 semantics: `Provider.DownTo(N)` rolls back every
migration with version **strictly greater than N**, leaving the DB at
version N. Concretely, `DownTo(16)` rolls back 17 and 18 but leaves 16
applied. The first test run failed because of this — the post-Down
assertions all saw the 00016 schema still in place.

Resolution: call `migrateDown(t, ctx, db, "sqlite", 15)` to revert
through 00016, and document the corrected semantics in the helper's
doc comment so the next caller doesn't trip on the same misconception.
This also cleanly rolls back migrations 17 and 18 (org_protected,
playground_sessions) along with 16 — which is intentional and necessary,
since 00018 has a FK to oauth_tokens.session_id that 00016 introduces.

### Verification

- `go test -run TestMigrate00016 ./internal/db/...` → PASS
- `go test ./internal/db/...` → PASS (other migration tests unaffected)
- The Down assertions ARE meaningful: each one would fail if the Down SQL
  in 00016 (or any prerequisite Down) were broken, e.g. forgetting to
  delete `anonymous_session_bearer` rows before recreating the table
  without that CHECK value.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Implementation matches the design body precisely. The goose
`DownTo` semantics correction (DownTo(N) leaves DB at N, not N-1) is well
captured in both the helper's doc comment AND the story implementation
notes — that's exactly the right place for a future-caller-saving note.
Helper is unexported and verified test-only (`grep migrateDown
internal/db/*.go` returns matches only in `migrate_test.go`). Down
assertions correctly run before re-Up so they can't be satisfied by the
re-Up's re-creation. Test passes on SQLite (`go test ./internal/db/...`
→ green).

Sibling `story-anon-bearer-test-integrity-transactional-rollback` still
at stage:review — parent feature stays at stage:review until that
sibling lands.
