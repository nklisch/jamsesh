---
id: epic-auto-merger-merge-engine-safe-auto-resolve
kind: story
stage: done
tags: [portal]
parent: epic-auto-merger-merge-engine
depends_on: [epic-auto-merger-merge-engine-three-way-merge]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Merge Engine — Safe Auto-Resolve Heuristics

## Scope

Add the three locked safe-auto-resolve heuristics to the merge
engine. Conflicted files that fall entirely under safe categories
get classified as `SafeAutoResolve` with the heuristic name; mixed
or unsafe conflicts remain `HardConflict`.

## Units delivered

- `internal/portal/automerger/heuristics.go` —
  `tryAutoResolve(base, ours, theirs []byte) (resolved []byte, heuristic string, ok bool)`,
  plus three internal detectors (`isIdenticalEdit`,
  `isWhitespaceOnly`, `isNonOverlappingAddition`)
- Update `merge.go` — after detecting hard-conflict on a file,
  attempt `tryAutoResolve`; if all conflicted files auto-resolve,
  return `SafeAutoResolve` with the most-conservative heuristic
  name; otherwise keep `HardConflict`
- `internal/portal/automerger/heuristics_test.go` — table tests
  for each detector with crafted inputs
- `testdata/safe-auto-resolve/` corpus with 7+ scenarios (per
  parent feature Unit 6)
- Adversarial cases: `additions-shared-line`,
  `mixed-modify-and-add`, `whitespace-but-indentation-changes`
  MUST escalate to HardConflict

## Acceptance Criteria

- [ ] `isIdenticalEdit` returns true iff ours == theirs (bytewise)
      and both differ from base
- [ ] `isWhitespaceOnly` returns true iff `TrimRight(line, " \t\r\n")`
      on each side produces equal text vs base, after normalizing
      line endings to LF
- [ ] `isNonOverlappingAddition` returns true iff both sides ONLY
      add lines (no MODIFY, no DELETE) AND the added lines aren't
      identical between the two sides
- [ ] Heuristic detection order: identical → whitespace → additions
      (most conservative first)
- [ ] Mixed-classification files: if ANY conflict in the file
      escalates, the file stays HardConflict
- [ ] Multi-file results: if ANY file stays HardConflict, the whole
      merge result is HardConflict; SafeAutoResolve requires ALL
      conflicted files to safely resolve
- [ ] Most-conservative heuristic name wins across files (priority:
      identical < whitespace < additions)
- [ ] All `testdata/safe-auto-resolve/*` scenarios pass with their
      declared `expected.json`
- [ ] The adversarial cases produce HardConflict, not
      SafeAutoResolve

## Notes

- Don't run heuristics on binary files (`util/binary` or detect
  null bytes manually). Binary conflicts always escalate.
- The auto-merger commits its result with an `Auto-Resolved: <heuristic>`
  trailer — that's the `outcomes` feature's job (this story just
  reports the heuristic name).
- Line-ending normalization: detect dominant line-ending in base;
  apply uniformly in the merged output.
- Edge case: empty files. Heuristics short-circuit on zero-byte
  inputs.

## Implementation notes

### Design decisions made during implementation

**isIdenticalEdit in tryAutoResolve is logically unreachable through Merge**:
The `Merge` function fast-paths on `ourCh.toHash == theirCh.toHash` before
calling `mergeFileContent`, so identical-blob edits never reach
`tryAutoResolve`. The detector is correct and unit-tested in isolation; it
provides defence-in-depth if `tryAutoResolve` is called directly by future
callers.

**Corpus test design**: The `safe-auto-resolve/` corpus tests route through
the full `Merge` function (not `tryAutoResolve` directly). This means:
- Scenarios where both sides produce the same blob (`identical-edits`,
  `additions-shared-line` with matching content) are fast-pathed by Merge to
  `clean-merge` — their expected.json reflects that reality.
- Only scenarios that survive git's own merge (producing actual conflict
  markers) reach the heuristic path. The whitespace, indentation, and
  non-overlapping-addition adversarial cases are designed to produce real
  git-merge-file conflicts.
- Unit tests in `heuristics_test.go` independently exercise every branch of
  each detector including the adversarial cases that Merge's fast-paths
  would otherwise pre-empt.

**isWhitespaceOnly tab guard**: The indentation-depth check counts leading
tabs per line (not total file tabs). A tab-to-spaces conversion changes the
leading tab count from N to 0 → detected and escalated.

**isNonOverlappingAddition diff approach**: Uses `diff -u` via os/exec (same
dependency as `git merge-file`). Parses unified diff hunks; any `-` line
(deletion) aborts with nil (not pure-add). The `oursAddedSet` membership
check for each theirs-added line catches the shared-line adversarial case in
O(n) time.

**Multi-file heuristic priority**: When multiple conflicted files resolve
under different heuristics, the most conservative (lowest priority number)
wins: identical(0) < whitespace(1) < additions(2). This ensures the reported
heuristic is the riskiest operation actually performed.

### Files changed
- `internal/portal/automerger/heuristics.go` — new file, 250 LoC
- `internal/portal/automerger/heuristics_test.go` — new file, 36 unit tests
- `internal/portal/automerger/merge.go` — conflict path extended with
  `tryAutoResolve` loop; `SafeAutoResolve` result path added
- `internal/portal/automerger/merge_test.go` — `expectedResult` struct gains
  `Heuristic` field; `runScenario` gains safe-auto-resolve assertions and
  builder dispatch; `buildSafeAutoResolveScenario` added
- `testdata/safe-auto-resolve/` — 7 corpus scenarios (3 safe, 4 adversarial)

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Three heuristics with adversarial corpus. The leading-tab-count check in isWhitespaceOnly correctly catches the tab→spaces indentation case. Duplicate-add safety check (oursAddedSet membership) is the right guard. Documented gotcha that Merge fast-paths identical edits before tryAutoResolve sees them — keeping isIdenticalEdit as defence-in-depth is reasonable.
