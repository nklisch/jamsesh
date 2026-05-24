---
id: gate-tests-applychangesperpath-extracted-branches
kind: story
stage: implementing
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
