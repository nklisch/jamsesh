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

## Design decisions

- **`MigrateDown` helper home**: test-only helper in `internal/db/migrate_test.go`. Unexported, takes `testing.TB`, replicates the provider-creation block from `MigrateUp(ctx, db, dialect)` then calls `provider.DownTo(ctx, version int64)`. Cannot be called from production code. Confirms the feature body's recommendation. Future migration tests in the same file get reuse for free; cross-package reuse can come later if a second migration test needs it.
- **Rollback test interposition pattern**: embed real `store.Store`, override only `WithTx` to pass a wrapped `TxStore` whose `CreateAnonymousBearer` returns the injected error. Go's struct embedding handles the other ~20 sub-interface methods automatically. Lightest implementation — no full TxStore decorator, no driver-level shim. The pattern is introduced fresh in `internal/portal/tokens/anon_bearer_test.go`; future tests in the package needing similar interception can copy it.
- **Rename strategy for both misleading tests**: rename existing tests to describe what they actually do AND add new properly-named tests:
  - `TestIssueAnonymousSessionBearer_TransactionalRollback` → `TestIssueAnonymousSessionBearer_EmptySessionID_NoDBCalls` (preserves the no-DB-calls assertion that is its real value); add new `TestIssueAnonymousSessionBearer_TransactionalRollback` that exercises the bearer-insert-error rollback path with the embedded-store pattern.
  - `TestMigrate00016_AnonymousBearers_UpDownUp` → keep the name AND implement the real Up-Down-Up cycle in its body (the existing body is mostly post-Up shape assertions; extend with Down step + re-Up step). The existing assertions document the post-Up state, which is still valuable; no rename needed if the body actually does the cycle.
  - (The migration test essentially absorbs option-3 wording for itself, since its current body is salvageable. The bearer test follows the rename-and-add path.)

## Acceptance (rollup)

- Both children at stage:done with verdicts ≥ approve
- No test in the package has a name that lies about what it asserts
- `TestIssueAnonymousSessionBearer_TransactionalRollback` actually
  exercises a bearer-insert-error rollback via the embedded-store pattern
- `TestMigrate00016_AnonymousBearers_UpDownUp` actually runs Up → Down → Up
- `MigrateDown` test helper available in `internal/db/migrate_test.go`
  for future migration tests
