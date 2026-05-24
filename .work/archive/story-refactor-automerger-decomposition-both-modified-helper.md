---
id: story-refactor-automerger-decomposition-both-modified-helper
kind: story
stage: done
tags: [portal, refactor]
parent: feature-refactor-automerger-decomposition
depends_on: [story-refactor-automerger-decomposition-side-changes-helper]
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Extract mergeBothModifiedPath helper from applyChangesPerPath

## Brief

`internal/portal/automerger/merge.go:applyChangesPerPath` is 96 lines,
of which the "both modified the same path" branch (lines ~290-355,
~65 lines) is the longest and most complex. After extraction, the
remaining function becomes a tight switch over four clear cases:
ours-only, theirs-only, both-deleted, both-modified.

This is the load-bearing nesting reduction in the parent feature —
`applyChangesPerPath`'s nesting depth drops from 4 to 2.

## Current state

```go
case ourCh != nil && theirCh != nil:
    // Both sides changed this path.
    ourDeleted := ourCh.deleted
    theirDeleted := theirCh.deleted

    if ourDeleted && theirDeleted {
        delete(state.mergedEntries, path)
        continue
    }
    if ourDeleted || theirDeleted {
        state.hardConflicts = append(state.hardConflicts, Conflict{File: path})
        continue
    }
    if ourCh.toHash == theirCh.toHash {
        state.mergedEntries[path] = treeEntry{hash: ourCh.toHash, mode: ourCh.mode}
        continue
    }
    // Actually different edits — need per-file three-way merge.
    baseContent, err := blobContent(repo, state.mergedEntries[path].hash)
    // ... 40 more lines of three-way merge invocation, blob writes,
    // conflict-range parsing, and placeholder blob writing
```

## Target state

```go
// New helper, package-private:
//
// mergeBothModifiedPath handles a single path where both ours and theirs
// changed. It mutates state.mergedEntries / state.hardConflicts /
// state.conflictedFiles in place.
func mergeBothModifiedPath(repo *gogit.Repository, state *mergeState, path string, ourCh, theirCh *sideChange) error {
    // delete/delete → take delete
    if ourCh.deleted && theirCh.deleted {
        delete(state.mergedEntries, path)
        return nil
    }
    // delete/modify → hard conflict
    if ourCh.deleted || theirCh.deleted {
        state.hardConflicts = append(state.hardConflicts, Conflict{File: path})
        return nil
    }
    // identical edit → take either
    if ourCh.toHash == theirCh.toHash {
        state.mergedEntries[path] = treeEntry{hash: ourCh.toHash, mode: ourCh.mode}
        return nil
    }
    // different edits → three-way merge
    return runThreeWayMerge(repo, state, path, ourCh, theirCh)
}

// In applyChangesPerPath:
case ourCh != nil && theirCh != nil:
    if err := mergeBothModifiedPath(repo, &state, path, ourCh, theirCh); err != nil {
        return mergeState{}, err
    }
```

A further extraction (`runThreeWayMerge` for the 30-line blob-reading +
mergeFileContent + auto-resolve flow) is optional inside the same story —
do it if the function is still over ~30 lines after `mergeBothModifiedPath`
lands.

## Implementation notes

- `mergeBothModifiedPath` returns `error` and accepts `state *mergeState`
  so it can append to slices on the parent state.
- `runThreeWayMerge` (optional second extraction) takes the same signature
  and handles the blob-read → mergeFileContent → write-blob /
  write-placeholder-blob flow.
- `applyChangesPerPath` becomes a 4-arm switch over the (ourCh, theirCh)
  cases, each delegating to either an inline assignment (ours-only,
  theirs-only) or a helper (both-modified).
- The mergeState struct mutation pattern is preserved — no return-shape
  changes.

## Acceptance criteria

- [ ] `mergeBothModifiedPath` exists as a package-private function.
- [ ] `applyChangesPerPath` becomes ≤ 40 LoC, nesting depth ≤ 2.
- [ ] All existing automerger tests pass without modification.
- [ ] `go build ./...` clean.
- [ ] `go test ./internal/portal/automerger/...` clean.

## Risk

**Medium.** The both-modified path is the hottest branch of the merge
engine. The extraction is mechanical but errors in state-mutation
ordering would corrupt merge results.

Mitigation: existing test fixtures under `testdata/` cover the
three-way-merge path extensively. Run the full automerger suite as
verification.

## Rollback

`git revert` the commit. No schema/state changes.

## Sequencing

`depends_on: [story-refactor-automerger-decomposition-side-changes-helper]`
— both stories touch `merge.go`. The side-changes helper lands first to
avoid textual conflicts; this story rebases on top.

## Implementation notes

Extracted two helpers from `applyChangesPerPath`:

- **`mergeBothModifiedPath`** (lines 275–301): handles delete/delete,
  delete/modify, identical-edit, and different-edit sub-cases. Accepts
  `state *mergeState` for in-place mutation of `mergedEntries`,
  `hardConflicts`, and `conflictedFiles`.

- **`runThreeWayMerge`** (lines 302–362): invokes `mergeFileContent`, then
  writes either a clean merged blob or a conflict-marker placeholder blob
  and appends to `state.conflictedFiles`. The placeholder/conflictedFiles
  dual-write invariant is preserved — both mutations happen atomically
  within the helper before any return path.

`applyChangesPerPath` went from 96 lines to 36 lines (LoC delta: −60),
nesting depth from 4 to 2. `runThreeWayMerge` was extracted because
`mergeBothModifiedPath` alone would have been ~40 LoC — extracting both
brings both helpers to clean, readable lengths.

All automerger tests pass without modification. `go build ./...` and
`go test ./...` both clean.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Behavior-preserving refactor delivered as designed. Implementation notes document any deviations (typically agent adapting to the file's actual structure differing from the story body's assumption). All tests pass; build clean.
