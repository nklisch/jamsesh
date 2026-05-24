---
id: story-store-partition-test-fixture-sweep
kind: story
stage: implementing
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

- [ ] Every test mock that embedded `store.Store` to override a small
      method count now implements only the narrow sub-interface.
- [ ] Test mock LoC reduced meaningfully — the goal is honest test
      surfaces, not LoC, but expect ~30-40% reduction in mock code.
- [ ] `go test ./...` clean.
- [ ] `go build ./...` clean.

## Risk

**Low.** Tests are isolated; failures show up clearly. Each mock is
local to its test file.

## Rollback

`git revert` the implementation commit.

## Sequencing

`depends_on: [story-store-partition-handler-signature-sweep]` — the
narrowed signatures from step 1 are what the test mocks adapt to.
