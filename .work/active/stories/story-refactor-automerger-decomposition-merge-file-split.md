---
id: story-refactor-automerger-decomposition-merge-file-split
kind: story
stage: implementing
tags: [portal, refactor]
parent: feature-refactor-automerger-decomposition
depends_on: [story-refactor-automerger-decomposition-both-modified-helper]
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Split mergeFileContent into runMergeFileTool + parseMergeOutput

## Brief

`internal/portal/automerger/merge.go:mergeFileContent` (lines ~610-654, ~45 LoC)
mixes two concerns: invoking the external `git merge-file` subprocess and
interpreting its output (merged content + conflict-marker count). Splitting
makes each half independently testable and clarifies the subprocess
boundary.

`ParseConflictRanges` (called from `applyChangesPerPath`) is a sibling
helper that parses conflict markers from merged output — verify whether
the new `parseMergeOutput` overlaps it before introducing a parallel
implementation.

## Current state

```go
// mergeFileContent runs git merge-file on three pieces of content via a
// temp-dir workspace and returns the merged bytes plus conflict count.
func mergeFileContent(base, ours, theirs []byte) (merged []byte, numConflicts int, err error) {
    // ~45 lines:
    //   - mktemp directory
    //   - write base.txt, ours.txt, theirs.txt
    //   - exec git merge-file --stdout ours.txt base.txt theirs.txt
    //   - read merged bytes from stdout
    //   - exit status: 0 = clean, 1..N = N conflicts, other = error
}
```

## Target state

```go
// runMergeFileTool invokes the external `git merge-file` binary against
// three input blobs via a temp workspace. Returns the merged bytes and the
// raw exit status. Does not interpret the exit status — the caller does
// that via interpretMergeFileExit (or the equivalent in mergeFileContent).
func runMergeFileTool(base, ours, theirs []byte) (mergedBytes []byte, exitCode int, err error) {
    // temp-dir + write 3 files + exec, returns raw stdout + exit
}

// interpretMergeFileExit translates a git-merge-file exit code into the
// (numConflicts, err) shape the rest of the package expects:
//   - 0:        clean merge, 0 conflicts.
//   - 1..127:   N conflict regions. (git merge-file caps the exit code
//               at 127 — if more than 127 conflicts occurred, the count
//               is "approximate but bounded".)
//   - 128+:     subprocess error.
func interpretMergeFileExit(code int) (numConflicts int, err error)

// mergeFileContent becomes a thin composition:
func mergeFileContent(base, ours, theirs []byte) ([]byte, int, error) {
    merged, code, err := runMergeFileTool(base, ours, theirs)
    if err != nil {
        return nil, 0, err
    }
    n, err := interpretMergeFileExit(code)
    if err != nil {
        return nil, 0, err
    }
    return merged, n, nil
}
```

## Implementation notes

- Verify `ParseConflictRanges` (used at line ~339 in `applyChangesPerPath`)
  is the conflict-marker scanner over merged bytes. It probably is — it
  parses `<<<<<<<` markers. Don't introduce a parallel parser; reuse it
  if applicable.
- The exit-code convention is documented inline in the new helper. Keep
  the wording from the current code's comments.
- `runMergeFileTool` is responsible for temp-dir lifecycle (defer
  cleanup); `mergeFileContent` no longer touches the filesystem directly.
- If the existing `mergeFileContent` has an explicit Context parameter,
  preserve it on `runMergeFileTool` so subprocess cancellation works.
  (Check the actual signature before assuming.)

## Acceptance criteria

- [ ] `runMergeFileTool` and `interpretMergeFileExit` exist as
      package-private functions.
- [ ] `mergeFileContent` is ≤ 15 LoC and contains no temp-dir / exec code.
- [ ] All existing automerger tests pass without modification.
- [ ] `go build ./...` clean.
- [ ] `go test ./internal/portal/automerger/...` clean.

## Risk

**Low.** The exec invocation and exit-code interpretation are well-tested
via fixtures. The split is mechanical.

## Rollback

`git revert` the commit.

## Sequencing

`depends_on: [story-refactor-automerger-decomposition-both-modified-helper]`
— this story touches the same file (`merge.go`) and lines further down.
Serial chain to avoid concurrent edits.
