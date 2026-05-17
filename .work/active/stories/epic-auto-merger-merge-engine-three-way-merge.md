---
id: epic-auto-merger-merge-engine-three-way-merge
kind: story
stage: implementing
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
