---
id: gate-cruft-test-only-exports-cluster
kind: story
stage: implementing
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
