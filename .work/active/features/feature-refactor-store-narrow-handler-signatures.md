---
id: feature-refactor-store-narrow-handler-signatures
kind: feature
stage: implementing
tags: [portal, refactor]
parent: null
depends_on: [feature-refactor-adapter-generic-wrap-helpers]
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Narrow handler/service signatures from `store.Store` to specific sub-interfaces

## Brief

`internal/db/store/store.go` already declares 20 domain
sub-interfaces (`OrgStore`, `AccountStore`, `SessionStore`,
`CommentStore`, `LeaseStore`, `PlaygroundSessionStore`, ...) plus
the umbrella `Store` interface that unions them all.

The sub-interfaces are **defined but unused at call sites**. Every
handler, service, and worker today accepts `store.Store` — the full
100+ method surface — even when it operates on a single domain.

This feature does the Go-idiomatic narrowing: each consumer accepts
the smallest interface union that reflects its actual dependency.

## Why this matters

The autopilot deferred this during the discovery scan because the
umbrella `Store` was documented-as-intentional. Reconsidered with the
user: the sub-interfaces existing-but-unused is the actual smell;
the partition is canonical Go (`accept interfaces, return structs`,
small interfaces, consumer-defined dependencies).

Concrete payoffs:

1. **Test mock shrinkage.** Today's failing-store test fixtures
   embed `store.Store` (100+ methods) just to override one
   (`failingGetOrgMemberStore struct { store.Store }` in
   `internal/portal/accounts/orgs_test.go`). With narrow interfaces
   each mock implements 3-10 methods.

2. **Compiler-enforced architectural boundaries.** Today
   `comments.Service` could call any `Store` method including
   `DeletePlaygroundSession`. With narrow interfaces the compiler
   refuses cross-domain leakage.

3. **Honest signatures.** `comments.NewService(s store.CommentStore, ...)`
   documents the dependency at the function signature. Today's
   `comments.NewService(s store.Store, ...)` lies about scope.

4. **Future service extraction.** If a service ever moves to its
   own binary (or has parts of its backing swapped), narrow
   interfaces are the precondition.

## Scope: the mild flavor only

Two flavors were considered:

- **Mild (this feature)**: consumers stop importing umbrella
  `Store`, use the existing sub-interfaces. Producer/adapter side
  unchanged — still implements everything. ~50 file edits, mostly
  mechanical.
- **Aggressive (NOT this feature)**: split the umbrella into
  truly independent backings, possibly multi-handle DB
  connections. Architecture change. Out of scope.

This feature implements ONLY the mild flavor.

## Design questions for refactor-design

1. **Consumer audit.** Enumerate every handler, service, and worker
   that takes `store.Store` in its constructor or as a field. Map
   each to its actual method-usage. Tools: `grep -rn "store.Store"
   internal/ cmd/ | grep -v _test.go` then per-file analysis.

2. **Multi-domain consumers.** Some genuinely span domains
   (e.g. `accounts.AcceptOrgInvite` touches `OrgStore`,
   `OrgMemberStore`, `AccountStore`, `SessionStore`). Two options:
   - **Inline anonymous unions**:
     `func NewHandler(s interface { OrgStore; OrgMemberStore; AccountStore; SessionStore })`
   - **Named composed interface**:
     `type AcceptOrgInviteStore interface { OrgStore; OrgMemberStore; AccountStore; SessionStore }`
     declared in the accounts package.
   Recommendation: named composed interfaces in the consumer
   package — Go-idiomatic and clearer at the import site. The
   design pass should confirm.

3. **`TxStore` handling.** `WithTx(fn func(TxStore) error)` is
   itself an umbrella. Tx scope is a different concern from
   consumer scope. **Keep `TxStore` as-is** — leave the union for
   transaction callbacks. Document this explicitly.

4. **Test-fixture sweep.** Every `_test.go` that has a
   `struct { store.Store }` embedding becomes a narrow mock. This
   may surface tests that depend on `store.Store` umbrella access
   in ways the consumer doesn't actually use — clean those up.

5. **Foundation-doc roll-forward.** `docs/ARCHITECTURE.md`'s data-
   layer section describes the `Store` interface. Update it to
   reflect the consumer/producer split convention. One paragraph,
   per the rolling-foundation rule.

## Children (declared up front; design pass refines)

| Child | Stage | Notes |
|---|---|---|
| `story-store-partition-handler-signature-sweep` | implementing | Bulk consumer narrowing — handlers + services + workers |
| `story-store-partition-test-fixture-sweep` | implementing | Test mocks updated to narrow interfaces |
| `story-store-partition-architecture-doc` | implementing | Foundation-doc roll-forward |

The design pass may merge or split these depending on the consumer-
audit results.

## Acceptance criteria (target)

- No production `internal/portal/` or `cmd/` constructor takes
  `store.Store` unless it genuinely needs the umbrella (e.g.
  the lifecycle worker that touches every domain). Document any
  exceptions.
- Test fixtures use narrow interface mocks (5-10 methods each
  instead of 100+).
- `docs/ARCHITECTURE.md` data-layer section updated.
- `go build ./...` and `go test ./...` clean.
- No behavior change.

## Sequencing

This feature depends on
`feature-refactor-adapter-generic-wrap-helpers` landing FIRST.
Reasons: (a) the wrap-helpers sweep changes adapter signatures
slightly, and landing the partition on top of a stable
post-Option-A adapter avoids churn-on-churn; (b) the test-fixture
sweep here is cleaner against the smaller post-Option-A adapter.

The dependency is declared at the substrate level — this feature
should be picked up only after the Option A feature reaches
`stage: review` or `done`.

## Out of scope

- Aggressive partition (multi-handle DB, per-domain backings).
- Splitting `TxStore` — kept as umbrella for tx callbacks.
- Renaming the existing sub-interfaces (they already match
  domain names).
- Anything code-gen.

## Notes

Pure refactor. Tagged `[refactor]`. No public API beyond
`internal/` is affected.

## Refactor Overview & Design (2026-05-24, autopilot)

Consumer audit:
- 54 non-test references to `store.Store` in production code
- 80 references in tests (mostly the `failingFooStore struct { store.Store }` embedding pattern)
- 19 existing sub-interfaces ready to be consumed

Plan: 3-step serial chain. Step 1 narrows production signatures.
Step 2 narrows test mocks to match. Step 3 rolls the foundation doc
forward.

## Refactor Steps

### Step 1: Handler/service/worker signature sweep
**Priority**: High  **Risk**: Medium
**Files**: ~50 files across `internal/portal/` and `cmd/portal/`
**Story**: `story-store-partition-handler-signature-sweep`

Sweep production consumers to take narrowest sub-interface union.
Multi-domain consumers use lowercase composed interfaces declared in
their own package. `WithTx` callbacks unchanged. cmd/portal/main.go
still passes full `store.Store` (the producer surface).

### Step 2: Test fixture sweep
**Priority**: High  **Risk**: Low
**Files**: ~30+ test files
**Story**: `story-store-partition-test-fixture-sweep`
**Depends on**: Step 1

Drop `struct { store.Store }` embedding pattern in test mocks; implement
only the narrow interface the consumer-under-test takes. Test mocks
shrink ~30-40%.

### Step 3: Foundation-doc roll-forward
**Priority**: Low  **Risk**: Very Low
**Files**: `docs/ARCHITECTURE.md`
**Story**: `story-store-partition-architecture-doc`
**Depends on**: Step 2

One-paragraph addition describing the consumer/producer split.

## Implementation Order

Serial chain: Step 1 → Step 2 → Step 3.

Step 1 is the wide-blast-radius work; the compiler catches every type
mismatch. Step 2 follows naturally as test files inherit the narrowed
signatures. Step 3 documents the final state.

## Design decisions

- **Mild flavor only** — narrowing consumer signatures, not splitting
  the adapter or introducing multi-handle DB. The producer (adapter)
  continues to implement the umbrella.
- **Composed interfaces in consumer packages, lowercase** — package-
  private; consumer-owned; the producer doesn't know they exist.
- **`TxStore` umbrella kept** — tx scope is a different concern.
- **No partial advance** — Step 1 must run to completion; partial
  consumer narrowing leaves the codebase in a mixed state.
