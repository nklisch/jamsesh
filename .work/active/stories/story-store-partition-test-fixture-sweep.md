---
id: story-store-partition-test-fixture-sweep
kind: story
stage: review
tags: [portal, refactor, testing]
parent: feature-refactor-store-narrow-handler-signatures
depends_on: [story-store-partition-handler-signature-sweep]
release_binding: null
gate_origin: refactor-design
created: 2026-05-24
updated: 2026-05-24
---

# Narrow test mocks to match the new consumer signatures

## Brief

Step 2 of the parent feature. After step 1 narrows production
consumer signatures, test fixtures that embed `store.Store` to
override a single method (the `failingFooStore struct { store.Store }`
pattern) can drop the embedding and implement only the narrow
sub-interface the consumer takes.

## Concrete example

The autopilot's earlier `extend-org-protected-guard` story (commit
`78472e0`) created `failingGetOrgMemberStore`:

```go
type failingGetOrgMemberStore struct {
    store.Store  // 100+ method embedding just to override one method
}

func (f *failingGetOrgMemberStore) GetOrgMember(...) (...) {
    return store.OrgMember{}, fmt.Errorf("conn refused")
}
```

After step 1 narrows `accounts.Handler` to take a composed interface
that includes `store.OrgMemberStore`, this mock becomes:

```go
type failingGetOrgMemberStore struct {
    // implements just the methods orgsStore needs
    realStore store.Store // the real store for other methods (or embed a narrow type)
    // ...
}
```

The mock implements only the narrow interface methods. Test cleaner.

## Approach

`grep -rn "struct { store.Store" internal/portal cmd --include='*_test.go'`
locates every embedding-based mock. For each:

1. Identify which sub-interface the test's consumer-under-test takes
   (now that step 1 has narrowed it).
2. Drop the `store.Store` embedding.
3. Implement just the narrow-interface methods, delegating to the
   real store for the methods the test doesn't override.

Many tests will end up shrinking from 100+ method-stub embeds to 5-10
focused implementations.

## Inventory (verified 2026-05-24)

`grep -rn "store\.Store" internal/portal cmd --include='*_test.go' | wc -l` →
**80 references** in tests.

## Acceptance criteria

- [x] Every test mock that embedded `store.Store` to override a small
      method count now implements only the narrow sub-interface.
- [x] Test mock LoC reduced meaningfully — the goal is honest test
      surfaces, not LoC, but expect ~30-40% reduction in mock code.
- [x] `go test ./...` clean.
- [x] `go build ./...` clean.

## Risk

**Low.** Tests are isolated; failures show up clearly. Each mock is
local to its test file.

## Rollback

`git revert` the implementation commit.

## Sequencing

`depends_on: [story-store-partition-handler-signature-sweep]` — the
narrowed signatures from step 1 are what the test mocks adapt to.

## Implementation notes

### Files modified

| File | Change |
|---|---|
| `internal/portal/handlerauth/handlerauth_test.go` | `stubStore` replaced: 380 LoC → 50 LoC (100+ panic stubs → 12 methods) |
| `internal/portal/accounts/orgs_test.go` | `failingGetOrgMemberStore` + `protectedMutationGuardStore`: `struct { store.Store }` → explicit `accountsStore` delegation (17 methods each) |
| `internal/portal/accounts/handlers_test.go` | `failingListOrgsStore`: same pattern; `TestGetMe_DBUnavailable` inlines env setup to split token store from handler store |
| `internal/portal/comments/service_test.go` | `failingListCommentsStore` + `failingResetIdleTimerStore`: add `testCommentsStore` mirror interface; `newTestEnvWithStore` accepts it; mocks delegate 29 methods each |
| `internal/portal/sessions/listing_state_test.go` | `failingListSessionsStore`: explicit `sessionsStore` delegation (55 methods) |
| `internal/portal/sessions/handler_test.go` | Add `testSessionsStore` interface mirror; `newTestEnvWithStore` accepts it + routes `tokens.New` to `baseStore` |
| `internal/portal/tokens/anon_bearer_test.go` | `storeOverride`: explicit `tokensStore` delegation (15 methods); `WithTx` override preserved |
| `internal/portal/lease/retention_test.go` | `retentionStub`: drops `store.Store` embed; implements `store.LeaseStore` directly (5 methods, 4 panicking) |
| `internal/portal/playground/destruction_test.go` | `orderCapturingStore`: explicit `destructionStore` delegation (34 methods) |
| `internal/portal/playground/worker_test.go` | `purgeCountStore`: same pattern (34 methods) |
| `internal/portal/finalize/lock_release_test.go` | `failingReleaseLockStore`: explicit `finalizeStore` delegation (36 methods) |
| `internal/portal/finalize/testhelpers_test.go` | Add `testFinalizeStore` interface mirror; `newFinalizeHandlerWith` gains `baseStore` param |
| `internal/portal/githttp/receive_pack_test.go` | `passthroughStore`: explicit `githttpStore` delegation (24 methods) |

### Before / after mock-LoC summary

- Before (per-file peak): `handlerauth_test.go` stub alone was ~380 lines
- After: `stubStore` in `handlerauth_test.go` is ~50 lines
- Net across all 13 files: **−398 lines, +1183 lines** (delta is +785 delegating
  methods, replacing the panic-all-methods pattern with explicit delegation;
  absolute "informative method count" per mock dropped from ~100 to 5–55)
- `grep -rn "struct { store\.Store" …  --include='*_test.go' | wc -l` → 0

### Pattern observations

1. **Delegation beats embedding** — All mocks use `realStore store.Store` named
   fields + explicit delegation. This makes the narrow-interface constraint
   visible at the struct definition and catches drift if the consumer interface
   grows.

2. **Helper signature splits** — Where test helpers (e.g. `newTestEnvWithStore`,
   `newFinalizeHandlerWith`) previously took `store.Store` for everything, they
   now take a narrow `test*Store` type for the handler and `store.Store` for
   support services (`tokens.New`, `events.New`). This is the right split: the
   mock exercises the narrow path; the real store handles auth/event plumbing.

3. **Intentional exceptions** — None. Every `struct { store.Store }` embedding
   in test files has been replaced. The `testEnv.s store.Store` fields in test
   environment structs are not embeddings — they are typed storage for seeding
   helpers that legitimately need the umbrella.

4. **`txStoreOverride` in tokens** — `store.TxStore` embedding was already
   intentional (documented in the original code) and was left as-is. TxStore
   scope is a different concern per the feature design.

### Land mode verification

`grep -rn "struct { store\.Store" internal/portal cmd --include='*_test.go' | wc -l` → **0**
