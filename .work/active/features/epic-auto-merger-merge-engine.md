---
id: epic-auto-merger-merge-engine
kind: feature
stage: drafting
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

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
