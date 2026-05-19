---
id: gate-cruft-test-only-exports-cluster
kind: story
stage: done
tags: [cleanup, portal, refactor]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# Cluster of test-only exports with no production callers

## Confidence
Medium

## Category
test-only public API surface

## Location
- `internal/portal/auth/middleware.go:20` (`OrgMemberFromContext`)
- `internal/portal/tokens/middleware.go:63` (`ContextWithAccount`)
- `internal/portal/httperr/httperr.go:95` (`ErrSessionNotFound`)
- `internal/portal/finalize/script.go:191` (`FirstParentLeafCommits`)
- `internal/portal/storage/objectstore/sync.go:580` (`ParsePackedRefsContent`)

## Evidence
`deadcode ./...` flagged all five as unreachable from `main`.
Verification confirms each is referenced only from `_test.go` files in
the matching `*_test` package — the export is required for the test to
compile, but no production code path exercises the function.

## Removal
Review whether each export documents a contract worth keeping or
whether the test exists only because the export does. Candidates:
- `OrgMemberFromContext` — middleware reads from context internally; no
  handler retrieves the member.
- `ContextWithAccount` — production injects accounts via middleware, not
  via this helper.
- `ErrSessionNotFound` — `httperr` has several constructors; the
  package code never calls this one.
- `FirstParentLeafCommits` — finalize never traverses commits this way
  in prod.
- `ParsePackedRefsContent` — comment admits "used in tests; exported
  for clarity."

For each, either delete (and drop its test) or document the
externally-supported contract.

## Implementation notes

| Symbol | Decision | Rationale |
|---|---|---|
| `OrgMemberFromContext` (auth) | **B** — move internal, lowercase | `TestRequireOrgRole_OrgMemberInContext` exercises real middleware context injection. Moved to `middleware_internal_test.go` (`package auth`); symbol renamed `orgMemberFromContext`. |
| `ContextWithAccount` (tokens) | **C** — keep, add doc comment | Used by `_test.go` files across 4 packages (`finalize`, `handlerauth`, `tokens`). Genuine cross-package test utility; not worth internalizing since it would require each package to duplicate the injection logic. Doc comment added. |
| `ErrSessionNotFound` (httperr) | **B** — move internal, lowercase | Tests exercise real constructor shape and `WriteFromError` pass-through behaviour. Moved 3 tests to `httperr_internal_test.go` (`package httperr`); symbol renamed `errSessionNotFound`. |
| `FirstParentLeafCommits` (finalize) | **B** — move internal, lowercase | Tests exercise complex DAG traversal logic (auto-merger merge commit skipping, chronological ordering). Moved to `script_internal_test.go` (`package finalize`); symbol renamed `firstParentLeafCommits`. |
| `ParsePackedRefsContent` (objectstore) | **B** — lowercase only | `sync_test.go` was already `package objectstore` (internal), so only the symbol rename was needed: `parsePackedRefsContent`. |

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: 5 test-only exports triaged per case: 4 of 5 (OrgMemberFromContext, ErrSessionNotFound, FirstParentLeafCommits, ParsePackedRefsContent) made package-private with tests moved/renamed to internal test packages. ContextWithAccount kept exported with a doc comment justifying the cross-package test-injection use (referenced by _test.go files in 4 sibling packages). All targeted packages build + test green.
