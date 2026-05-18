---
id: gate-cruft-automerger-exported-test-shim
kind: story
stage: review
tags: [cleanup, portal, refactor]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# `ExportedComputeAddressedTo` test-only shim with explicit "for testing" comment

## Confidence
Medium

## Category
test-only export shim

## Location
`internal/portal/automerger/addressing.go:120-124`

## Evidence
```go
// ExportedComputeAddressedTo is the exported shim for testing.
// Production callers should use computeAddressedTo directly within the package.
func ExportedComputeAddressedTo(repo *gogit.Repository, draftTip plumbing.Hash, conflicts []Conflict, sourceRef string) ([]string, error) {
    return computeAddressedTo(repo, draftTip, conflicts, sourceRef)
}
```

## Removal
Move `addressing_test.go` from `package automerger_test` to
`package automerger` (an internal test file) so it can call the
lowercase `computeAddressedTo` directly, then delete the export shim.
Eliminates a function whose only purpose is bypassing the package
boundary for tests.

## Implementation notes

- Changed `addressing_test.go` from `package automerger_test` to `package automerger`; dropped the `jamsesh/internal/portal/automerger` import; replaced all `automerger.ExportedComputeAddressedTo(...)` calls with `computeAddressedTo(...)` and `automerger.Conflict` with `Conflict`.
- Created `testhelpers_test.go` (`package automerger`) with copies of `initRepo`, `commitFiles`, `commitFilesWithMessage`, and `run` — these are distinct from the same-named helpers in `merge_test.go`/`outcomes_test.go` which remain in `package automerger_test` (separate compile unit, no collision).
- Deleted the `ExportedComputeAddressedTo` shim and its doc comment from `addressing.go`.
- `go test ./internal/portal/automerger/...` passes.
