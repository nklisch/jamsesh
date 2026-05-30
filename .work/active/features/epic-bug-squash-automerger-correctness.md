---
id: epic-bug-squash-automerger-correctness
kind: feature
stage: drafting
tags: [bug, portal]
parent: epic-bug-squash
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Auto-merger correctness & error classification

## Brief

The auto-merger (`internal/portal/automerger/`) is the background machinery that
three-way-merges incoming session commits onto the draft ref and emits
`merge.succeeded`/conflict events. The bug-scan found four correctness defects
clustered here, two of them High: a lost-event race in the per-session
worker/queue lifecycle, and a swallowed post-commit emit that leaves the git
system-of-record and the event log divergent. Two Low error-classification gaps
(`==` vs `errors.Is` for `ErrNotFound`, and an ignored `diff` exit code) round
out the cluster.

This feature delivers a correct, durable auto-merge path: the worker never
strands a queued `commit.arrived` event, a failed event emit after a durable
git ref move is recoverable (not silently dropped), and error classification is
wrapping-safe. It covers only the auto-merger package's correctness — it does
NOT change the merge heuristics' semantics, the conflict-resolution UX, or the
event schema.

## Epic context
- Parent epic: `epic-bug-squash`
- Position in epic: independent backend feature — no cross-feature dependency;
  the highest-blast-radius hotspot (silent missed merges / state divergence).

## Foundation references
- `docs/ARCHITECTURE.md` — Portal § Auto-merger workers
- `docs/SPEC.md` — go-git in-process operations, event-emission discipline
- Pattern: `tx-emit-then-fanout` (event emission ordering)

## Child stories (pre-existing, from bug-scan — re-parented here)
- `bug-squash-automerger-strands-commit-event` — High, concurrency — `internal/portal/automerger/worker.go:130`
- `bug-squash-automerger-swallows-merge-emit` — High, error-handling — `internal/portal/automerger/outcomes.go:155`
- `bug-squash-errors-is-not-used-errnotfound` — Low, error-handling — `internal/portal/automerger/worker.go:338`
- `bug-squash-diff-exit-code-ignored` — Low, error-handling — `internal/portal/automerger/heuristics.go:228`

<!-- feature-design fills in interfaces, the worker/queue lifecycle redesign,
and the per-story test approach. The two Highs share worker.go/outcomes.go and
should be designed together. -->
