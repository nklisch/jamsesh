---
id: feature-refactor-automerger-decomposition
kind: feature
stage: review
tags: [portal, refactor]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Decompose automerger merge/diff/apply god functions

## Brief

`internal/portal/automerger/merge.go` has grown to 660 lines with the
three-way merge engine collapsed into a small number of long functions
that each handle multiple distinct concerns (orchestration, diff
computation, blob I/O, conflict-range parsing, submodule recursion).
Reading the file end-to-end is the only way to understand any one
operation, and the deep nesting (>4 levels of switch/if/if/if in
`applyChangesPerPath` and `computeMergeDiff`) makes test coverage
brittle.

Surfaced by a discovery-mode `/agile-workflow:refactor-design` scan.

## Specific targets

| Function | Lines | Concern split |
|---|---|---|
| `Merge` | 37-90 | Already small — orchestrates child calls. Leave as the public seam. |
| `computeMergeDiff` | 133-263 | Splits into: tree-pair diff, conflict detection per path, resolution heuristic |
| `applyChangesPerPath` | 264-374 | Splits into: per-path conflict classifier + per-path apply, with the heuristic-resolution branch isolated |
| `mergeFileContent` | 610-654 | Splits into: external 3-way merge tool invocation + conflict-range parser |
| `flattenTreeInto` | 446-468 | Submodule recursion branch extracted |

Heuristics (`internal/portal/automerger/heuristics.go`, 382 lines) and
outcomes (`internal/portal/automerger/outcomes.go`, 363 lines) are
considered acceptable in their current shape; this feature is about
`merge.go` only.

## Design questions for feature-design

- Should the conflict-range parser move into a small `conflict.go`
  file inside the package, or stay co-located?
- Naming: prefer `computeXxx` / `applyXxx` / `parseXxx` verb prefixes
  consistent with the rest of `internal/portal/`?
- Test posture: the existing automerger tests are end-to-end via
  fixtures under `testdata/`. Do we want unit tests around the new
  smaller seams (parser, classifier), or is the existing fixture
  coverage enough?

## Acceptance criteria (target)

- `merge.go` ≤ ~400 LoC; no function >80 LoC.
- Maximum nesting depth ≤ 3 levels inside any extracted function.
- All existing automerger tests pass without modification (behavior
  preserving).
- `go test ./internal/portal/automerger/...` clean.
- The three-way merge contract (ours/base/theirs, fast-forward,
  conflict event payload shape) is unchanged.

## Notes

Behavior-preserving — no merge-strategy changes, no new outcomes, no
event-payload changes. This is purely structural.

## Refactor Overview

Four extractions, all in `internal/portal/automerger/merge.go`. Each
extraction is mechanical and load-bearing for a specific readability
or testability win:

1. Two near-identical diff-walking loops in `computeMergeDiff` collapse
   into one `buildSideChanges` helper (~30 LoC saved, removes the most
   obvious duplication).
2. The "both modified" branch in `applyChangesPerPath` (~65 LoC,
   nesting depth 4) extracts to `mergeBothModifiedPath`, dropping the
   caller to a clean 4-arm switch with nesting depth 2.
3. `mergeFileContent` splits along its subprocess/parsing boundary into
   `runMergeFileTool` + `interpretMergeFileExit` + thin orchestrator.
4. `flattenTreeInto`'s submodule branch extracts to `flattenSubmodule`
   if the inline body exceeds 5 LoC (verify before extracting; tiny
   inline submodule case may be a no-op).

All four touch the same file, so the child stories chain via
`depends_on` to avoid concurrent textual edits. The orchestrator will
run them as 4 sequential single-agent waves.

## Refactor Steps

### Step 1: Extract buildSideChanges
**Priority**: High  **Risk**: Low
**Files**: `internal/portal/automerger/merge.go`
**Story**: `story-refactor-automerger-decomposition-side-changes-helper`

The two `for _, ch := range baseTo{Ours,Theirs}` loops in
`computeMergeDiff` (lines ~160-202) are identical apart from the map
they populate and the error-string prefix. Collapse to a single helper
parameterised on side name.

### Step 2: Extract mergeBothModifiedPath
**Priority**: High  **Risk**: Medium
**Files**: `internal/portal/automerger/merge.go`
**Story**: `story-refactor-automerger-decomposition-both-modified-helper`
**Depends on**: Step 1

The longest and most complex branch of `applyChangesPerPath` (~65 LoC)
extracts to `mergeBothModifiedPath`, with an optional second helper
`runThreeWayMerge` if the function is still > 30 LoC after the first
extraction.

### Step 3: Split mergeFileContent
**Priority**: Medium  **Risk**: Low
**Files**: `internal/portal/automerger/merge.go`
**Story**: `story-refactor-automerger-decomposition-merge-file-split`
**Depends on**: Step 2

Separate the subprocess invocation (`runMergeFileTool`) from the
exit-code interpretation (`interpretMergeFileExit`). `mergeFileContent`
becomes a 10-line composition. Verify no overlap with the existing
`ParseConflictRanges` helper.

### Step 4: Extract flattenSubmodule (conditional)
**Priority**: Low  **Risk**: Low
**Files**: `internal/portal/automerger/merge.go`
**Story**: `story-refactor-automerger-decomposition-flatten-submodule-extract`
**Depends on**: Step 3

Submodule branch in `flattenTreeInto` extracts only if the inline body
exceeds 5 LoC. Story includes an explicit no-op escape that closes the
story without code changes if the extraction isn't worth it.

## Implementation Order

Serial chain: Step 1 → Step 2 → Step 3 → Step 4. All steps touch the
same file (`merge.go`) so concurrent edits would collide. Each step
lands as its own commit so individual rollback is possible.

## Out of scope

- `automerger/heuristics.go` (382 lines) — not touched. Heuristic-based
  conflict resolution is its own concern.
- `automerger/outcomes.go` (363 lines) — not touched.
- The behaviour-preservation invariant: no merge-strategy changes, no new
  outcomes, no event-payload changes. Pure structural decomposition.

## Implementation summary (orchestrator)

All 4 child stories advanced to `stage: review`:

- `story-refactor-automerger-decomposition-side-changes-helper` — `buildSideChanges` helper extracted; `computeMergeDiff` -24 LoC, error wording preserved exactly via `side` parameter
- `story-refactor-automerger-decomposition-both-modified-helper` — `mergeBothModifiedPath` + `runThreeWayMerge` extracted; `applyChangesPerPath` 96 → 36 LoC, nesting depth 4 → 2
- `story-refactor-automerger-decomposition-merge-file-split` — `runMergeFileTool` + `interpretMergeFileExit` extracted; agent caught git's exit-code 255 quirk for binary files and preserved original semantics (treat all non-zero exit as conflict count rather than gating on 128+)
- `story-refactor-automerger-decomposition-flatten-submodule-extract` — **no-op land**: agent found the submodule branch is already shared with `filemode.Dir` under a single 3-LoC body. Per story's own decision rule (≤5 LoC = no extraction warranted), no change to `merge.go`. Story honestly closed.

**Verification**: `go build ./...` clean, `go test ./internal/portal/automerger/...` clean (11 tests pass).
