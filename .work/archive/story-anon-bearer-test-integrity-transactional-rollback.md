---
id: story-anon-bearer-test-integrity-transactional-rollback
kind: story
stage: done
tags: [testing, tokens]
parent: feature-anon-bearer-test-integrity
depends_on: []
release_binding: v0.4.0
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Add a real transactional-rollback test for IssueAnonymousSessionBearer

## Brief

The current `TestIssueAnonymousSessionBearer_TransactionalRollback` test in
`internal/portal/tokens/anon_bearer_test.go` does NOT exercise transactional
rollback. Its body only invokes the issuance helper with an empty `sessionID`,
which is rejected by the pre-tx validation guard — no DB calls are made at
all, so there's nothing for `WithTx` to roll back. The test name is a lie.

The Phase 4 acceptance criterion in
`feature-epic-ephemeral-playground-anon-bearer` explicitly called for:

> Transactional rollback: if account creation succeeds but bearer creation
> fails (e.g., via a wrapping store injecting an error), no account row is
> left behind

This is the test we actually need.

## Scope (from parent feature design)

This story implements **Unit 3** of `feature-anon-bearer-test-integrity`.
Read the parent feature body for the full design — this section is the
short version.

**Two changes in `internal/portal/tokens/anon_bearer_test.go`:**

1. **Rename the misnamed test.** The existing test (currently named
   `_TransactionalRollback`) becomes `_EmptySessionID_NoDBCalls`. Body
   unchanged — the no-DB-calls assertion is its real value. Add a comment
   explaining the rename and pointing at the new test.

   Note the name collision risk with the existing
   `TestIssueAnonymousSessionBearer_EmptySessionID_Rejected`: keep both.
   `_Rejected` asserts the error surface; `_NoDBCalls` adds the
   no-row-written invariant. Distinct invariants, distinct tests.

2. **Add the real rollback test** named `_TransactionalRollback`. Uses the
   **embedded-store override pattern** locked in by parent design:

   ```go
   type txStoreOverride struct {
       store.TxStore
       bearerErr error
   }
   func (o *txStoreOverride) CreateAnonymousBearer(...) (store.OAuthToken, error) {
       return store.OAuthToken{}, o.bearerErr
   }

   type storeOverride struct {
       store.Store
       bearerErr error
   }
   func (o *storeOverride) WithTx(ctx, fn) error {
       return o.Store.WithTx(ctx, func(tx store.TxStore) error {
           return fn(&txStoreOverride{TxStore: tx, bearerErr: o.bearerErr})
       })
   }
   ```

   The test wraps the real store with `storeOverride`, calls
   `IssueAnonymousSessionBearer`, asserts the injected error propagates via
   `errors.Is`, then asserts no account row exists with the test's
   display_name.

   `errors.Is` works because `service_impl.go:240` wraps with `%w`.

## Read-side query for the post-rollback assertion

If no domain-level "list anonymous accounts by display_name" query exists,
the story can drop to a raw `SELECT COUNT(*) FROM accounts WHERE
display_name=?` via a test-local `*sql.DB` handle. Acceptance is the
invariant ("zero rows"), not the query shape.

## Acceptance criteria

- [ ] `TestIssueAnonymousSessionBearer_TransactionalRollback` exists and
      exercises the bearer-insert-error rollback path via the embedded-store
      pattern (NOT via empty-sessionID short-circuit).
- [ ] Injected `bearerErr` propagates back via `errors.Is`.
- [ ] After the failed call, zero account rows exist with
      `display_name='fern-moth'` (or whatever the test uses).
- [ ] `TestIssueAnonymousSessionBearer_EmptySessionID_NoDBCalls` exists
      (the renamed-from `_TransactionalRollback`) and preserves the
      no-DB-calls assertion.
- [ ] `TestIssueAnonymousSessionBearer_EmptySessionID_Rejected` is
      untouched and still passes.
- [ ] `go test ./internal/portal/tokens/...` green.
- [ ] Manual smoke (optional, don't commit): temporarily change
      `service_impl.go:240` from `%w` to `%v`; the test's `errors.Is`
      check must fail. Confirms the test is real.

## Independence

This story is independent of
`story-anon-bearer-test-integrity-migration-updownup` — different package,
different file, different invariants. No `depends_on`; can land in parallel.

## Source

Surfaced during review of
`feature-epic-ephemeral-playground-anon-bearer`. Filed under the
test-integrity discipline in CLAUDE.md ("A failing test that documents why
it fails ... is more honest than a green test that lies").

## Implementation notes (2026-05-23)

Three changes landed in `internal/portal/tokens/anon_bearer_test.go`:

1. **Renamed** `TestIssueAnonymousSessionBearer_TransactionalRollback`
   (line 209-230) → `TestIssueAnonymousSessionBearer_EmptySessionID_NoDBCalls`.
   Body unchanged — preserves the no-DB-calls assertion. Updated the doc
   comment to explain the rename and point at the new real-rollback test
   below it. Distinct from the existing `_Rejected` test (which only asserts
   the error surface).

2. **Added the embedded-store override pattern** (`txStoreOverride` +
   `storeOverride`) per the parent feature's locked-in design. Go struct
   embedding satisfies every other `store.Store`/`store.TxStore` method
   automatically — the two override types only redirect `WithTx` and
   `CreateAnonymousBearer`. Pattern stays test-local, not promoted to a
   shared helper.

3. **Added the real rollback test**:
   `TestIssueAnonymousSessionBearer_TransactionalRollback`. Wraps the real
   store with `storeOverride`, injects `errors.New("synthetic bearer-insert
   failure")`, calls `IssueAnonymousSessionBearer`, then asserts:
   - `err` is non-nil
   - `errors.Is(err, injectErr)` succeeds (proves the `%w` wrap chain
     survives — would fail if `service_impl.go:240` ever switched to `%v`)
   - Zero rows in `accounts` table with `display_name='fern-moth'`

### Read-side query for the post-rollback assertion

The `store.Store` interface has no "list/count accounts by display_name"
method. Rather than add a domain query just for the test, added a small
helper `openStoreAndSQLWithSession(t)` that returns BOTH the `store.Store`
AND the underlying `*sql.DB` from `db.Open`. The test then runs a raw
`SELECT COUNT(*) FROM accounts WHERE display_name=?` via that `*sql.DB`.
Acceptable because:

- The raw query is test-only; production code never sees the `*sql.DB`.
- `db.Open` already returns both values — we're just stopping discarding
  the second one in this one test.
- A future test that needs the same shape can call the same helper.

### Verification

- `go test -run TestIssueAnonymousSessionBearer ./internal/portal/tokens/...` →
  PASS (all 9 tests in the suite: 7 originals + the renamed + the new).
- `errors.Is(err, injectErr)` is the load-bearing assertion that exercises
  the rollback — it would fail if `service_impl.go:240`'s `%w` wrap broke,
  satisfying the story's "manual smoke" intent automatically.

### Notes on the existing name collision

The renamed `_EmptySessionID_NoDBCalls` and the pre-existing
`_EmptySessionID_Rejected` are intentionally distinct: `_Rejected` asserts
the error surface only; `_NoDBCalls` additionally asserts that the empty
sessionID short-circuits BEFORE any DB write occurs. Two invariants, two
tests, both kept.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- `openStoreAndSQLWithSession` near-duplicates `openStoreWithSession`'s
  org+session seeding (different IDs only). Parent feature design
  explicitly chose to keep helpers test-local rather than promote a shared
  variant, so this is consistent with that choice; mentioning for future
  refactor if a third caller appears.

**Notes**:
- Acceptance criteria all met: real rollback test exists and uses the
  embedded-store override pattern, `errors.Is` propagation asserted, zero
  `accounts` rows with `display_name='fern-moth'` asserted post-rollback,
  `_EmptySessionID_NoDBCalls` rename preserves the no-DB-calls invariant,
  `_EmptySessionID_Rejected` untouched.
- Manual smoke confirmed during review: flipping `service_impl.go:240`
  from `%w` to `%v` causes
  `TestIssueAnonymousSessionBearer_TransactionalRollback` to fail with
  `expected wrapped synthetic bearer-insert failure (via errors.Is), got
  create anon bearer: synthetic bearer-insert failure` — confirming the
  test exercises real wrap-chain propagation through `WithTx`.
- `go test ./internal/portal/tokens/...` green; `go vet` clean.
- Test-integrity discipline satisfied: no test name in the package lies
  about what it asserts. Three distinct empty-/rollback-related invariants
  now have three distinct tests with names that match their bodies.
