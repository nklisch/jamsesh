---
id: epic-auto-merger-merge-engine-three-way-merge
kind: story
stage: review
tags: [portal]
parent: epic-auto-merger-merge-engine
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Merge Engine — Three-Way Merge Driver

## Scope

Build the pure three-way merge function: input is three go-git
Commit pointers, output is a classified MergeResult (`CleanMerge` or
`HardConflict` — `SafeAutoResolve` is added in the next story).

After this story, the auto-merger worker can call `automerger.Merge(...)`
and get a deterministic, structured result.

## Units delivered

- `internal/portal/automerger/types.go` — `MergeResult`, `ResultKind`, `Conflict`, `LineRange`
- `internal/portal/automerger/merge.go` — `Merge(ctx, repo, source, draftTip, ancestor) (MergeResult, error)`
- `internal/portal/automerger/conflicts.go` — conflict-marker parser → `[]LineRange`
- `internal/portal/automerger/merge_test.go` + `testdata/clean/`, `testdata/hard-conflict/`
- go.mod: add `github.com/go-git/go-git/v5@latest`

## Acceptance Criteria

- [ ] `Merge` against three commits where ours and theirs touch
      different files returns `CleanMerge` with a non-empty
      `MergedTreeSHA`
- [ ] `Merge` against three commits where ours and theirs modify
      the same line differently returns `HardConflict` with
      `Conflicts` populated; each conflict carries the file path
      and 1-indexed line ranges of conflict regions
- [ ] `Merge` short-circuits when source == ancestor (returns
      CleanMerge with draftTip's tree) or draftTip == ancestor
      (returns CleanMerge with source's tree)
- [ ] The conflict-marker parser correctly extracts line ranges
      for both single-region and multi-region conflicts in a file
- [ ] `testdata/clean/disjoint-files`, `testdata/clean/disjoint-lines`,
      `testdata/hard-conflict/same-line-different`,
      `testdata/hard-conflict/delete-vs-modify` all pass
- [ ] No DB, no event-emitter, no ref-updater imports — pure
      library with go-git as the only external dep (plus the
      system `git` binary via `os/exec` for `git merge-file`)

## Implementation notes

### Files delivered

- `internal/portal/automerger/types.go` — `ResultKind`, `MergeResult`, `Conflict`, `LineRange` exactly as specified in Unit 1.
- `internal/portal/automerger/merge.go` — `Merge(ctx, repo, source, draftTip, ancestor)` implementing the full three-way merge flow (Units 2 + design decisions). Key design choices:
  - `sideChange` is a package-level type so `effectivePath` helper can reference it cleanly.
  - Short-circuit on `source == ancestor` (returns draftTip's tree) and `draftTip == ancestor` (returns source's tree).
  - Per-file classification: only-ours, only-theirs, both-deleted, delete-vs-modify, identical-result, different-edits.
  - `mergeFileContent` shells out to `git merge-file --stdout` for byte-level three-way merge; exit code 0 = clean, ≥1 = N conflict hunks.
  - `buildTree` assembles the merged tree bottom-up (deepest dirs first), writing every sub-tree object via `repo.Storer.SetEncodedObject`.
  - `flattenTree` snapshots base tree into a `map[path]treeEntry` so the merge plan can apply incremental changes.
- `internal/portal/automerger/conflicts.go` — `ParseConflictRanges` scans `<<<<<<<`/`>>>>>>>` markers, returns 1-indexed `[]LineRange`.
- `internal/portal/automerger/merge_test.go` — 9 passing tests: 4 corpus scenarios, 2 short-circuit tests, 3 `ParseConflictRanges` unit tests.
- `internal/portal/automerger/testdata/` — corpus with `clean/disjoint-files`, `clean/disjoint-lines`, `hard-conflict/same-line-different`, `hard-conflict/delete-vs-modify`.
- `go.mod` / `go.sum` — `github.com/go-git/go-git/v5@v5.19.0` added.

### Acceptance criteria status

- [x] Disjoint-files returns `CleanMerge` with non-empty `MergedTreeSHA`
- [x] Same-line-different returns `HardConflict` with `Conflicts` populated
- [x] Short-circuit: source == ancestor → CleanMerge with draftTip's tree
- [x] Short-circuit: draftTip == ancestor → CleanMerge with source's tree
- [x] Conflict-marker parser handles single-region and multi-region correctly
- [x] All 4 testdata scenarios pass
- [x] No DB/event/ref-updater imports — pure library, go-git + system `git`

### Notes for reviewer

- The `buildTree` function handles empty trees (e.g., ours deletes all files) by producing a valid empty tree object.
- Binary files: `git merge-file` exits with a negative code on binary input, which is surfaced as an error (not a conflict) — acceptable for v0; a binary-detection guard can be added when needed.
- `AllowEmptyCommits` in the test helper handles the delete-vs-modify scenario where ours commits no files.

## Notes

- The auto-loaded `go-git` skill carries the verified API surface
  for `object.DiffTree`, `object.MergeBase`, and tree composition.
- For per-file three-way merging, invoke `git merge-file -p` via
  `os/exec` on the three blob contents. Exit code 0 = clean, 1 =
  conflicts. Capture stdout as merged content.
- Tree composition: assemble a new `object.Tree` with merged blobs
  + unchanged entries; write to the repo's ObjectStorer; return
  the tree's hash as `MergedTreeSHA`.
- Test corpus runner: walk `testdata/`, for each subdirectory build
  a synthetic git repo (`git init`, commit base, branch ours, commit
  ours, branch theirs, commit theirs), call Merge, assert result
  matches `expected.json`.
- Story 2 (safe-auto-resolve) layers heuristics on this story's
  HardConflict path. This story's HardConflict classification is
  the conservative default; story 2 narrows it.
