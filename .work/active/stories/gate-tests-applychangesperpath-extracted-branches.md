---
id: gate-tests-applychangesperpath-extracted-branches
kind: story
stage: done
tags: [testing, portal, refactor]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# `applyChangesPerPath` extracted-seam branches (both-add-same, both-delete, non-overlapping chunks) untested

## Priority
Medium

## Spec reference
Item: `story-refactor-automerger-decomposition-both-modified-helper`

Acceptance criterion: Feature AC: "Behavior-preserving — no merge-strategy changes." Existing TestCorpus covers end-to-end behavior; new phase tests are sparse around the freshly extracted boundary.

## Gap type
missing test for valid partition (extracted seam)

## Suggested test
```go
func TestApplyChangesPerPath_BothAddedSameContent(t *testing.T) { ... }
func TestApplyChangesPerPath_BothModified_NonOverlappingChunks(t *testing.T) { ... }
func TestApplyChangesPerPath_BothDeleted(t *testing.T) { ... }
```
TestCorpus is a fixture sweep; the `merge_phases_test` seams need direct
coverage to catch a future re-extraction that silently changes branch
routing.

## Test location (suggested)
`internal/portal/automerger/merge_phases_test.go`

## Implementation notes

Added three tests directly to `internal/portal/automerger/merge_phases_test.go`,
each targeting a branch in `mergeBothModifiedPath` (called from `applyChangesPerPath`):

- `TestApplyChangesPerPath_BothAddedSameContent` — exercises the identical-hash
  fast-path (`ourCh.toHash == theirCh.toHash`): both sides add `new.txt` with
  identical bytes; expects a clean merge with the shared blob in `mergedEntries`.

- `TestApplyChangesPerPath_BothDeleted` — exercises the delete/delete fast-path
  (`ourCh.deleted && theirCh.deleted`): base has `deleted.txt`; both sides omit
  it; expects the path absent from `mergedEntries` with no conflicts.

- `TestApplyChangesPerPath_BothModified_NonOverlappingChunks` — exercises the
  "different edits → `runThreeWayMerge`" path with non-overlapping edits (header
  vs footer), which produces a clean three-way merge; the merged blob is verified
  to contain both changes.

All three pass. Full `./internal/portal/automerger/...` suite: PASS.

## Review notes

Approve. Three tests each pin a distinct branch of `mergeBothModifiedPath`:
identical-hash fast-path (asserts blob hash equality), both-delete fast-path
(asserts path absent), and non-overlapping three-way merge (asserts merged
bytes contain both edits). Real go-git repos, no mocks. Tests pass.
