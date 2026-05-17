---
id: refactor-split-merge-function
kind: story
stage: implementing
tags: [refactor, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Refactor — Split `automerger.Merge()` into phases

## Why

`internal/portal/automerger/merge.go:Merge()` (lines 37-334, ~298 LoC) is
the auto-merger's central function. It currently performs four conceptual
phases inline:

1. **Tree flatten + diff** — flatten ours/theirs/base trees, compute the
   change sets
2. **Conflict detection** — identify file-level overlaps and same-file
   divergence
3. **Heuristic resolution** — apply auto-resolve heuristics (whitespace,
   non-overlapping line ranges, etc.)
4. **Tree recomposition** — build the merged tree object and write blobs

The function is testable end-to-end but its individual phases are not —
unit-testing only the conflict-detection logic, or only the heuristic pass,
requires running the entire merge. This refactor extracts each phase to its
own named function so phases can be tested independently and so future
phase changes have a smaller surface area.

## Files

- Modify: `internal/portal/automerger/merge.go`
- Possibly modify: `internal/portal/automerger/merge_test.go` (add
  phase-level unit tests)

## Current shape

```go
func Merge(repo, oursHash, theirsHash, baseHash plumbing.Hash) (Result, error) {
    // [lines 37-334, all inline]
}
```

## Target shape

```go
func Merge(repo, oursHash, theirsHash, baseHash plumbing.Hash) (Result, error) {
    diff, err := computeMergeDiff(repo, oursHash, theirsHash, baseHash)
    if err != nil { return Result{}, err }

    conflicts := detectConflicts(diff)
    resolved, remaining := applyHeuristics(repo, diff, conflicts)
    if len(remaining) > 0 {
        return Result{Conflicts: remaining}, nil
    }
    tree, err := composeMergedTree(repo, diff, resolved)
    if err != nil { return Result{}, err }
    return Result{Tree: tree}, nil
}

func computeMergeDiff(...)  (mergeDiff, error)
func detectConflicts(...)   []conflict
func applyHeuristics(...)   (resolvedChanges, []conflict)
func composeMergedTree(...) (plumbing.Hash, error)
```

(Exact phase boundaries to be confirmed during implementation by reading
the current function carefully — the goal is faithful extraction, not
restructuring the logic.)

## Implementation notes

- **READ the current function thoroughly** before extracting. The phase
  boundaries above are a hypothesis — the actual cohesive seams may differ.
- Each extracted function should be `package-private` (lowercase) unless
  there's a clear reason to export.
- The intermediate types (`mergeDiff`, `conflict`, `resolvedChanges`) live
  in `merge.go` next to `Result` and the existing helpers.
- The existing tests in `merge_test.go` must pass unchanged — this is the
  primary safety net for the refactor.
- Add one new unit test per extracted phase that exercises the phase in
  isolation with hand-rolled inputs. These are bonus coverage, not gate
  blockers, but they justify the extraction.

## Acceptance

- [ ] `go build ./...` passes
- [ ] `go test ./internal/portal/automerger/...` passes with **identical**
      output to pre-refactor (no test changes or skips)
- [ ] `Merge()` is ≤ 50 lines (orchestration only)
- [ ] Each extracted phase has a docstring stating its inputs, outputs,
      and the invariant it preserves
- [ ] At least one new unit test per phase (4 new tests total, minimum)

## Risk

**MEDIUM.** The auto-merger is the heart of the jamsesh model. Behavior
changes here propagate to every push that lands on a sync ref. Mitigations:

- Pre-flight: confirm `merge_test.go` covers the conflict scenarios listed
  in `docs/SPEC.md` before splitting. If coverage is thin, **add tests
  first as a separate commit** so the safety net is in place.
- Run the test suite repeatedly during extraction, not just at the end.
- Behavior parity is the only acceptable outcome — if a phase split surfaces
  a latent bug, fix the bug in a separate follow-up commit so the refactor
  commit stays pure.

## Rollback

`git revert` the commit. The function is self-contained in one file.
