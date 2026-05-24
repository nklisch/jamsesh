---
id: feature-refactor-automerger-decomposition
kind: feature
stage: drafting
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
