---
id: epic-auto-merger-merge-engine
kind: feature
stage: implementing
tags: [portal]
parent: epic-auto-merger
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Auto-Merger — Merge Engine

## Brief

The pure three-way merge library at the heart of the auto-merger. Given a
source commit, a draft tip, and a common ancestor (all from go-git), runs
the merge and classifies the outcome into one of:

- **clean-merge** — three-way merge succeeded without conflicts
- **safe-auto-resolve** — three-way merge produced conflicts, but every
  conflict falls into a locked-list safe category (whitespace-only,
  non-overlapping additions, identical edits) and can be resolved
  deterministically without human judgment
- **hard-conflict** — three-way merge produced conflicts requiring human
  judgment; structured payload describing each conflicted file and line
  range

**Safe-auto-resolve heuristics** (locked at epic-design — the
exhaustive list, nothing else qualifies):

- **Whitespace-only**: trailing whitespace differences, line-ending
  differences (LF/CRLF), tab-vs-space changes that don't affect
  indentation depth
- **Non-overlapping additions**: both sides ADD different lines within
  the same hunk; neither modifies nor deletes a shared line; resolution
  interleaves both sides in the order they appear
- **Identical edits**: both sides made the same textual change

Escalate to hard-conflict for: both sides modifying the same line(s)
differently; one side deleting + other side modifying; rename +
modification interactions; anything where the resolution is a judgment
call.

**Pure-library design** (Ports & Adapters principle): this feature does
NOT touch the DB, does NOT emit events, does NOT update refs. Inputs are
git tree handles + the ancestor + the source commit; output is a
classified result the caller (the `outcomes` feature) consumes. The only
IO is go-git's object reads against the bare repo.

**Return shape** (precise contract for the outcomes consumer):

```go
type MergeResult struct {
    Kind          ResultKind  // CleanMerge | SafeAutoResolve | HardConflict
    MergedTreeSha string      // populated for CleanMerge and SafeAutoResolve
    Heuristic     string      // "whitespace" | "additions" | "identical" — only when SafeAutoResolve; mixed cases use the most conservative label
    Conflicts     []Conflict  // populated only for HardConflict
}

type Conflict struct {
    File   string
    Ranges []LineRange  // {Start, End} pairs, 1-indexed
}
```

Does NOT include the merge-commit creation (that's `outcomes` — it
needs the author/committer identity logic and trailer composition).
Does NOT include the worker orchestration (that's `worker`).

## Epic context

- Parent epic: `epic-auto-merger`
- Position in epic: pure core; `outcomes` and `worker` consume it.

## Foundation references

- `docs/PRINCIPLES.md` — Liveness via continuous integration; (also the
  Ports & Adapters principle from the auto-loaded principles skill —
  this feature is the pure side of the cut)
- `docs/ARCHITECTURE.md` — The auto-merger section (the three-way
  merge mechanics + the conflict-event payload's `conflicts` array shape)
- `docs/PROTOCOL.md` — Conflict event schema (the `conflicts` array of
  `{file, ranges}`)

## Inherited epic design decisions

- **Safe-auto-resolve heuristics**: whitespace-only, non-overlapping
  additions, identical edits. Nothing else qualifies.
- **Audit trail for auto-resolve**: the resolution heuristic is
  returned in `MergeResult.Heuristic` so the `outcomes` feature can
  stamp it into the merge commit's `Auto-Resolved:` trailer.

## Decomposition risks

- Safe-auto-resolve heuristics are the highest-correctness-sensitivity
  surface in the epic. A wrong "whitespace-only" or "non-overlapping
  additions" detection silently corrupts user content. Mitigation:
  design pass produces a canonical adversarial test corpus (real-world
  conflict shapes that should NOT auto-resolve but a naive
  implementation might).

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Library**: `github.com/go-git/go-git/v5` (already in research as the
  locked choice). Use `merge.Merger` or compose with `object.Tree`'s
  `Diff` plus a custom three-way driver — depends on what v5 actually
  ships. The `go-git` skill (auto-loaded) carries the verified API
  surface; the implementation will likely use lower-level helpers
  (`merge-base` lookup via `Object.MergeBase`, tree diffing via
  `object.DiffTree`, three-way merge via custom logic over the three
  trees because go-git's high-level merge API has limitations).
- **Diff3 algorithm**: when go-git's built-in three-way merge isn't
  enough, fall back to invoking the system `git merge-file --diff3
  -p <ours> <base> <theirs>` via `os/exec` for individual files. The
  storage feature already requires the `git` binary for `git init
  --bare`; reusing it here is consistent.
- **Result struct location**: `internal/portal/automerger/`. Public
  API: `Merge(ctx, repo, srcCommit, draftTipCommit, ancestor *object.Commit)
  (MergeResult, error)`. Inputs are go-git Commit pointers; the
  caller (`worker` feature) opens the bare repo and resolves these.
- **Heuristic detection ordering** (story 2): when classifying a
  conflicted file, try heuristics in this order — identical edits →
  whitespace-only → non-overlapping additions. The most conservative
  classification wins if multiple apply (e.g., a hunk that's both
  whitespace AND additions is reported as "whitespace").
- **Adversarial test corpus**: a fixed directory
  `internal/portal/automerger/testdata/conflicts/` with subdirectories
  per scenario, each containing `base.txt`, `ours.txt`, `theirs.txt`,
  and an `expected.json` declaring the expected MergeResult kind and
  heuristic. Includes the obvious safe cases AND the adversarial
  cases (e.g., "non-overlapping at first glance but actually
  overlapping because they share a line"). The test runner walks the
  directory and asserts each case.
- **Whitespace-only detection**: compare lines after
  `strings.TrimRight(line, " \t\r\n")`. If `ours` and `theirs`
  produce the same trim-result vs `base`, they're whitespace-only.
- **Line-ending normalization**: detect CRLF vs LF by inspecting
  the file bytes; if the only difference is line endings,
  classify whitespace-only and use the dominant line ending in
  `base`.
- **Identical edits detection**: if `ours == theirs` (byte-exact)
  but both differ from `base`, the change is identical → keep
  `ours`.
- **Non-overlapping additions**: parse each conflict hunk into
  hunks-of-hunks. If both sides only ADD lines (no MODIFY, no
  DELETE), and the added lines don't share content (or even if they
  do — they're additive), interleave in the order they appear. The
  "share a line" exception: if both sides try to add an IDENTICAL
  line, that's a duplicate-add concern — escalate to hard-conflict
  for safety. (This is one of the adversarial test cases.)
- **Story decomposition**: 2 stories.
  1. `three-way-merge` — go-git wrapper, MergeResult contract,
     clean-merge + hard-conflict classification. depends_on: []
  2. `safe-auto-resolve` — the 3 heuristic detectors layered onto
     story 1's hard-conflict path; conflicted-file analysis returns
     resolution + heuristic name. depends_on: [three-way-merge]

## Architectural choice

**Pure library at `internal/portal/automerger/`. Inputs are go-git
Commit pointers; output is `MergeResult`. No DB, no events, no
side-effects beyond reading from the underlying ObjectStorer.
The package depends on `go-git/v5` and the system `git` binary.**

This is the canonical Ports & Adapters cut: the engine doesn't know
or care about ref names, session IDs, event log, or auto-merger
worker semantics. Those concerns belong to sibling features.

## Implementation Units

### Unit 1: MergeResult contract

**File**: `internal/portal/automerger/types.go`
**Story**: `epic-auto-merger-merge-engine-three-way-merge`

```go
package automerger

type ResultKind string

const (
    CleanMerge       ResultKind = "clean-merge"
    SafeAutoResolve  ResultKind = "safe-auto-resolve"
    HardConflict     ResultKind = "hard-conflict"
)

type MergeResult struct {
    Kind          ResultKind
    MergedTreeSHA string     // populated for CleanMerge and SafeAutoResolve
    Heuristic     string     // "whitespace" | "additions" | "identical" — only when SafeAutoResolve
    Conflicts     []Conflict // populated only for HardConflict
}

type Conflict struct {
    File   string
    Ranges []LineRange
}

type LineRange struct {
    Start int  // 1-indexed
    End   int  // 1-indexed, inclusive
}
```

### Unit 2: Three-way merge driver

**File**: `internal/portal/automerger/merge.go`

```go
package automerger

import (
    "context"

    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/plumbing/object"
)

// Merge runs a three-way merge between source and draftTip, using
// ancestor as the base. Returns a classified result.
//
// Caller is responsible for opening the repo and resolving the three
// commits. Caller is also responsible for finding the merge-base —
// callers can use object.MergeBase via go-git directly.
func Merge(ctx context.Context, repo *git.Repository, source, draftTip, ancestor *object.Commit) (MergeResult, error)
```

Implementation pattern:
1. Resolve all three trees (`source.Tree()`, etc.)
2. Diff base→ours and base→theirs via `object.DiffTree`
3. For each file affected by either side:
   - If only one side modified → take that side's content directly
   - If both sides modified identically → take ours, mark identical
   - If both sides modified differently → invoke `git merge-file` on the three blobs; capture stdout as merged content + exit-code as conflict-or-not
4. Compose the merged tree object (write blobs + tree)
5. If no file had conflicts, return CleanMerge with the new tree's
   SHA
6. If any file had conflicts, return HardConflict with structured
   Conflicts. (Story 2 layers heuristics on this; story 1 stops
   at hard-conflict for conflicted files.)

### Unit 3: Conflict line-range extraction

**File**: `internal/portal/automerger/conflicts.go`

Given a file with conflict markers (`<<<<<<<`, `=======`, `>>>>>>>`),
parse out the 1-indexed line ranges of each conflict region. This
is the data shape the `outcomes` feature consumes for the
`conflict.detected` event payload.

### Unit 4: Tests + adversarial corpus

**File**: `internal/portal/automerger/merge_test.go`,
`internal/portal/automerger/testdata/`

Test corpus organization:

```
testdata/
├── clean/
│   ├── disjoint-files/        ours adds file A, theirs adds file B
│   └── disjoint-lines/        same file, but non-overlapping line ranges
├── hard-conflict/
│   ├── same-line-different/   both modify same line differently
│   ├── delete-vs-modify/      ours deletes file X, theirs modifies X
│   └── rename-vs-modify/      go-git rename-detection cases
```

Each scenario directory contains `base.txt`, `ours.txt`,
`theirs.txt`, and `expected.json`. The test runner walks the
directory, builds a synthetic git repo, runs Merge, and asserts the
result matches `expected.json`.

Story 2 adds the `safe-auto-resolve/` subtree with the three
heuristic categories + adversarial cases.

### Unit 5: Safe-auto-resolve detectors (story 2)

**File**: `internal/portal/automerger/heuristics.go`
**Story**: `epic-auto-merger-merge-engine-safe-auto-resolve`

```go
// For each conflicted file from the hard-conflict path, attempt to
// classify under one of the safe heuristics. If ALL conflicts in the
// file fall under safe heuristics, return the resolution + name.
// If ANY conflict in the file escalates, the file remains
// hard-conflict.
//
// Returns: (resolvedContent, heuristic, ok).
func tryAutoResolve(base, ours, theirs []byte) (resolved []byte, heuristic string, ok bool)
```

Internal helpers:
- `isWhitespaceOnly(base, ours, theirs []byte) (resolved []byte, ok bool)`
- `isIdenticalEdit(base, ours, theirs []byte) (resolved []byte, ok bool)`
- `isNonOverlappingAddition(base, ours, theirs []byte) (resolved []byte, ok bool)`

Each helper is small (≤ 50 LoC) and independently tested.

Detection ordering in `tryAutoResolve`:
1. Identical edits (cheapest, most conservative)
2. Whitespace-only
3. Non-overlapping additions

The Merge function (Unit 2) calls `tryAutoResolve` for each
conflicted file from Unit 2's output. If all files auto-resolve,
the result becomes SafeAutoResolve with the most-conservative
heuristic name; otherwise it stays HardConflict.

### Unit 6: Adversarial test corpus (story 2)

Adds under `testdata/safe-auto-resolve/`:

- `whitespace-trailing/` — both sides remove trailing whitespace differently → whitespace-only
- `whitespace-tabs-to-spaces/` — ours converts tabs to spaces, theirs adds new lines at different points → non-overlapping additions (the tab conversion is a separate heuristic decision; this case tests interaction)
- `identical-edits/` — both sides make the same change → identical
- `additions-disjoint/` — ours adds line "foo" at top, theirs adds line "bar" at bottom → additions
- `additions-shared-line/` — both sides add the SAME line → MUST escalate to hard-conflict (duplicate-add safety)
- `mixed-modify-and-add/` — ours modifies a line + adds a new line, theirs only adds a new line → hard-conflict (modify involved)
- `whitespace-but-indentation-changes/` — looks whitespace-only but changes indentation depth → hard-conflict (might be a syntactic change in some languages)

## Story decomposition

1. `epic-auto-merger-merge-engine-three-way-merge` — Units 1-4
2. `epic-auto-merger-merge-engine-safe-auto-resolve` — Units 5-6
   (depends on three-way-merge)

## Implementation Order

1. three-way-merge
2. safe-auto-resolve (refines hard-conflict path)

## go.mod additions

- `github.com/go-git/go-git/v5@latest` — locked decision per SPEC.md
  and the auto-loaded `go-git` skill

## Testing

The test corpus IS the test suite. Each scenario tests an
end-to-end Merge call. Additional unit tests on the heuristic
detectors directly (with crafted byte inputs, no repo needed).

## Risks

- **`go-git` v5's merge API**: actually exposes `merge.MergeBase`,
  `merge.MergeOptions`, but the high-level three-way merger is
  limited. Mitigation: combine `object.DiffTree` (file-level diff)
  with per-file `git merge-file` subprocess invocation for the
  byte-level three-way merge. The `go-git` skill carries verified
  patterns.
- **Safe-auto-resolve correctness**: the adversarial corpus is the
  protection. Every heuristic must pass its positive cases AND
  every adversarial case (where a naive implementation would
  misclassify).
- **Performance**: every push triggers a three-way merge. Per-file
  subprocess invocation of `git merge-file` adds latency. v0
  acceptable; if hot, switch to an in-process diff3 library
  (`github.com/sergi/go-diff`?) — re-evaluate post-launch.
- **Line-ending heuristic + binary files**: don't run text
  heuristics on binary files. Use `binary` package to detect.
