---
id: anon-bearer-migration-updownup-test-coverage
kind: story
stage: implementing
tags: [testing, migrations]
parent: null
depends_on: []
release_binding: null
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

Implementation:

1. Add a `MigrateDown(ctx, db, dialect)` helper (or expose `MigrateTo(...)`
   if goose's API supports it) so the test can drive backward migration
   programmatically.
2. After the initial Up + data insertion, run Down to one version below
   00016 and verify:
   - `is_anonymous` is no longer a column on `accounts`
   - `session_id` is no longer a column on `oauth_tokens`
   - rows with `kind = 'anonymous_session_bearer'` were deleted
3. Run Up again and re-verify the post-migration shape.

Either rename the test to match what it actually does, or implement the
Up-Down-Up cycle the name promises. The rolling-foundation principle
applies to test names too: a test name should be the present-tense
description of what the test asserts.

## Source

Surfaced during review of
`feature-epic-ephemeral-playground-anon-bearer`. Filed under the
test-integrity discipline in CLAUDE.md.
