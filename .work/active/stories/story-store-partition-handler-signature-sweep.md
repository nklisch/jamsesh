---
id: story-store-partition-handler-signature-sweep
kind: story
stage: implementing
tags: [portal, refactor]
parent: feature-refactor-store-narrow-handler-signatures
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-24
updated: 2026-05-24
---

# Narrow handler/service/worker signatures from store.Store to specific sub-interfaces

## Brief

Step 1 of the parent feature. Sweep every production consumer of
`store.Store` to accept the narrowest sub-interface union it actually
needs. ~54 production call sites in `internal/portal/` and `cmd/`.

## Approach

For each consumer:

1. Identify which `store.<Domain>Store` sub-interfaces' methods it
   actually calls. Use `grep` on the consumer's package to find every
   `s.<MethodName>` call, then look up which sub-interface declares
   that method in `internal/db/store/store.go`.

2. If a single sub-interface covers everything, change the signature
   to that:
   ```go
   // Before
   func New(s store.Store, ...) *Handler
   
   // After
   func New(s store.CommentStore, ...) *Handler
   ```

3. If the consumer touches multiple domains, declare a named composed
   interface in the consumer's package and use that:
   ```go
   // In internal/portal/accounts/orgs.go (or a sibling file)
   type orgsStore interface {
       store.OrgStore
       store.OrgMemberStore
       store.AccountStore
       store.OrgInviteStore
   }
   
   func New(s orgsStore, ...) *Handler
   ```
   Recommend: lower-case `orgsStore` (package-private) since the
   composed type is purely internal to the consumer.

4. **`WithTx` callbacks**: keep the existing `func(tx TxStore) error`
   signature — the tx scope is a different concern from consumer scope.

5. **Constructors with multiple use modes**: if the consumer's struct
   stores `store.Store` and methods on the struct each touch different
   slices, declare a single composed interface that's the union of
   what the struct's methods touch. Don't try to split the struct.

## Consumer inventory (verified 2026-05-24)

`grep -rn "store\.Store" internal/portal cmd --include='*.go' | grep -v _test.go | wc -l` →
**54 references**.

Concrete consumers (read the full list before starting):

```bash
grep -rn "store\.Store" internal/portal cmd --include='*.go' | grep -v _test.go
```

Notable surfaces:
- `internal/portal/tokens/service_impl.go` — TokenStore, AccountStore
- `internal/portal/automerger/` — wide use; likely needs a composed interface
- `internal/portal/auth/` — provision/oauth/magic-link; multi-domain
- `internal/portal/storage/service.go` — narrow (just archive methods)
- `internal/portal/accounts/handlers.go` — multi-domain (Org + OrgMember + Account + OrgInvite)
- `internal/portal/sessions/handler.go` — multi-domain
- `internal/portal/comments/service.go` — single-domain (CommentStore)
- `internal/portal/playground/` — multi-domain
- `cmd/portal/main.go` — top-level wiring; keeps `store.Store` (it's the producer surface)

## Acceptance criteria

- [ ] Every production consumer in `internal/portal/` and `cmd/portal/` (excluding `main.go` and direct adapter callers) takes either a single sub-interface or a named composed interface — NOT `store.Store`.
- [ ] Named composed interfaces are declared in the consumer's package, lowercase (package-private).
- [ ] `WithTx` callbacks unchanged.
- [ ] `cmd/portal/main.go` still passes a full `store.Store` (the adapter satisfies all sub-interfaces). The narrowing is on consumer signatures, not on what the adapter implements.
- [ ] `go build ./...` clean.
- [ ] `go test ./...` clean. Tests may need updating; if they take `store.Store` and the narrowing requires `store.CommentStore`, that surfaces as a test compile error.
- [ ] `grep -rn "store\.Store" internal/portal --include='*.go' | grep -v _test.go | wc -l` returns ≤ 10 (down from 54). The remaining are the legitimate full-Store consumers (boot-path wiring, anywhere that genuinely needs the umbrella).

## Implementation notes

- **Don't fight the refactor**: if a consumer genuinely touches 8+ sub-interfaces and naming a composed interface adds no clarity, leaving it on `store.Store` is acceptable. Document any such exceptions in implementation notes.
- **Commit chunking**: this is a big refactor (~50 files). Consider committing per package (e.g., one commit for `accounts/`, one for `comments/`, one for `automerger/`). The test suite running clean after each chunk is the safety net.
- **The mild flavor only** — no multi-handle DB connections, no per-domain backings. Just narrowing consumer signatures.

## Risk

**Medium.** Wide blast radius (50+ files). Mitigation: the compiler catches every type mismatch; each narrowing is mechanical; existing test suite catches behavior regressions.

## Rollback

`git revert` the implementation commit(s). The `store.Store` interface itself is unchanged; only consumer signatures.

## Notes on follow-up

This story is a refactor — it changes signatures but no behavior.
After landing, sibling story
`story-store-partition-test-fixture-sweep` narrows the test mock
fixtures to match (the test compile errors from this story's narrowing
are what the sibling story addresses).
