---
id: feature-anon-bearer-test-integrity
kind: feature
stage: drafting
tags: [testing, tokens, migrations]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Anon-bearer test integrity

## Brief

Two tests under `feature-epic-ephemeral-playground-anon-bearer`
whose names lie about what they assert. Surfaced from review of
that feature (2026-05-23) under the test-integrity discipline in
CLAUDE.md: "A failing test that documents why it fails ... is more
honest than a green test that lies." Both green tests pass while
silently failing to exercise the acceptance criteria they're named
after.

## Why a feature

Both children share the same theme (mis-named tests on the same
feature) and need the same kind of work: re-name the existing tests
to describe what they actually assert, then add new tests that
exercise the original acceptance criteria. One feature gives the
work a coherent verdict and a single PR.

The migration story also lands a reusable `MigrateDown` test helper
(or `MigrateTo` if goose supports it) that future migration tests
can use — feature-design should consider whether to land that
helper first as a separate story.

## Child stories

- `story-anon-bearer-test-integrity-transactional-rollback` — rename
  the misleading test and add a real transactional-rollback test
  for `IssueAnonymousSessionBearer` (Phase 4 acceptance criterion in
  the original anon-bearer feature)
- `story-anon-bearer-test-integrity-migration-updownup` — implement
  the actual Up-Down-Up cycle promised by the test name, or rename
  to describe what it does; add `MigrateDown` helper

## Design notes (for /agile-workflow:feature-design)

Consider whether the `MigrateDown` helper should be:

1. A test-only helper in `internal/db/migrate_test.go` (cheapest;
   reusable by future migration tests in that file)
2. A public helper in `internal/db/` (broadest reuse but invites
   misuse — production code should never call it)

Option 1 is likely correct. The helper should be locked behind a
`testing.TB` argument or a build tag to prevent accidental
production use.

## Acceptance (rollup)

- Both children at stage:done with verdicts ≥ approve
- No test in the package has a name that lies about what it
  asserts
- `MigrateDown` test helper available for future migration tests
