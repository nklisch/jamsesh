---
id: story-store-partition-architecture-doc
kind: story
stage: implementing
tags: [portal, documentation]
parent: feature-refactor-store-narrow-handler-signatures
depends_on: [story-store-partition-test-fixture-sweep]
release_binding: null
gate_origin: refactor-design
created: 2026-05-24
updated: 2026-05-24
---

# Roll docs/ARCHITECTURE.md forward to describe the consumer/producer Store split

## Brief

Step 3 of the parent feature. After steps 1+2 narrow production +
test signatures, `docs/ARCHITECTURE.md`'s data-layer section needs to
describe the convention as it now exists: producer (adapter) still
implements the umbrella `store.Store`; consumers (handlers, services,
workers) accept the narrowest sub-interface union they need.

## Approach

1. Read `docs/ARCHITECTURE.md` and find the data-layer section
   describing `store.Store`.
2. Add a one-paragraph note describing the consumer/producer split:
   > Consumers (handlers, services, workers) accept the narrowest
   > sub-interface union of `store.Store` they need; the adapter
   > (`internal/db/store/{sqlite,postgres}_adapter.go`) implements the
   > umbrella so callers can pass it to anything. This keeps each
   > consumer's data-layer dependency visible at its signature and
   > makes test mocks small.
3. Per the rolling-foundation rule: do NOT add legacy notes ("previously
   handlers took the full Store..."). Foundation docs describe the
   system NOW; git history is the audit trail.

## Acceptance criteria

- [ ] `docs/ARCHITECTURE.md`'s data-layer section has a paragraph
      describing the consumer/producer split.
- [ ] No legacy / migration-note prose added.
- [ ] No build/test verification needed (pure docs change).

## Risk

**Very low.** One paragraph in a foundation doc.

## Rollback

`git revert` the implementation commit.

## Sequencing

`depends_on: [story-store-partition-test-fixture-sweep]` — the doc
describes the post-state including the test fixture shape.
