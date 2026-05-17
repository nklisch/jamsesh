---
id: epic-auto-merger-merge-engine-safe-auto-resolve
kind: story
stage: implementing
tags: [portal]
parent: epic-auto-merger-merge-engine
depends_on: [epic-auto-merger-merge-engine-three-way-merge]
release_binding: null
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
