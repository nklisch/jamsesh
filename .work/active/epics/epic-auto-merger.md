---
id: epic-auto-merger
kind: epic
stage: implementing
tags: [portal]
parent: null
depends_on: [epic-portal-git]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Auto-merger

## Brief

The continuous-integration heart of jamsesh. A server-side worker (Go
goroutines triggered by post-receive events) that processes every commit
arriving on a sync-mode ref:

1. Resolves the commit's parent and the current `draft` tip.
2. Finds the common ancestor.
3. Runs a three-way merge in-process using `go-git`.
4. On success: creates a merge commit (with the new commit and the draft
   tip as parents) carrying `Auto-Merger: true` and `Source-Commit: <sha>`
   trailers, advances `draft`, emits `merge.succeeded`.
5. On conflict: does not advance `draft`, emits `conflict.detected` with
   structured payload (file paths, line ranges, all three SHAs).

Parses `Resolves-Conflict: <event-id>` trailers on incoming commits — when
such a commit succeeds in the auto-merger, the named conflict event is
marked resolved automatically.

Respects mode: isolated refs are skipped entirely. The auto-merger only
operates on sync refs.

This epic does NOT cover the in-portal-UI conflict-resolver (there isn't
one — humans resolve conflicts in their own CC environment); it does NOT
cover finalize-time cherry-picks (those happen locally on the human's
machine).

## Foundation references

- `docs/ARCHITECTURE.md` — The auto-merger section
- `docs/SPEC.md` — Ref structure (`draft` ref semantics)
- `docs/PROTOCOL.md` — Conflict event schema, Commit trailer conventions
- `docs/PRINCIPLES.md` — Liveness via continuous integration

## Design decisions

- **Conflict-resolution heuristics**: auto-resolve safe cases; escalate
  everything else. "Safe" is precisely defined to keep the auto-merger
  honest. Auto-resolve only:
  - **Whitespace-only conflicts** — trailing whitespace differences,
    line-ending differences (LF/CRLF), tab-vs-space changes that don't
    affect indentation depth
  - **Non-overlapping additions** within the same conflict hunk where
    both sides ADD different lines and neither modifies or deletes a
    shared line (interleave both sides in the order they appear)
  - **Identical edits** — both sides made the same change (textually
    equal post-merge)
  Escalate to `conflict.detected` for any conflict involving:
  - Both sides modifying the same line(s) differently
  - One side deleting + other side modifying
  - Rename + modification interactions (let git's rename detection
    surface these as conflicts)
  - Any case where the resolution would be a judgment call

  Auto-resolved merge commits carry an extra trailer:
  `Auto-Resolved: whitespace` / `additions` / `identical` so the
  resolution heuristic is auditable from `git log`.

- **Merge-commit author identity**: author = the source commit's author
  (the human whose work is being integrated); committer = synthetic
  `jamsesh auto-merger <auto-merger@<portal-host>>` identity; trailer
  `Auto-Merger: true` + `Source-Commit: <sha>`. This uses git's
  canonical author/committer distinction correctly: Alice wrote the
  change, auto-merger applied it to draft. `git log` reads naturally
  ("alice: Add refresh-token revocation endpoint") while still being
  machine-distinguishable as auto-merger-generated via the committer
  field and the trailers. The auto-merger is invisible plumbing — its
  identity surfaces only in the committer field and trailers, never as
  the author of the integration work.

Locked at epic-design time (this pass):

- **Worker concurrency model**: per-session goroutine, spawned on first
  commit, draining a bounded per-session in-memory FIFO queue. Idle
  timeout exit (re-spawn on next commit). No global pool to tune. Cross-
  session is fully parallel; within-session is serialized (required —
  concurrent draft mutations would race).
- **Crash recovery**: on startup, scan `events` for `commit.arrived`
  newer than the latest auto-merger-touched draft position per session;
  idempotency-check via go-git ancestor (if source-commit is already in
  draft, skip); otherwise enqueue. No separate auto-merger state table.
  Rationale: git's ancestor check is decisive and cheap; aligns with
  PRINCIPLES.md "Recovery is `git fetch`."
- **`Resolves-Conflict` mismatch handling**: silent no-op if the
  event-id doesn't match an open conflict event (safe under replays).
  Log a warning on conflict with a closed event holding a different
  `resolving_commit_sha`. Rationale: no security impact, just a missed
  auto-closure.
- **`Source-Ref` trailer**: every auto-merger merge commit carries
  `Source-Ref: jam/<session>/<user>/<branch>` alongside `Auto-Merger:
  true` and `Source-Commit: <sha>`. Rationale: makes `git log` readable
  for humans investigating; trivially cheap.
- **Auto-resolved cases emit only `merge.succeeded`**: never
  `conflict.detected`. The merge commit's `Auto-Resolved: <heuristic>`
  trailer is the audit trail in `git log`. Rationale: `conflict.detected`
  signals "human attention required"; auto-resolved cases don't.

## Decomposition

Three child features along the Ports & Adapters cut. `merge-engine` is
the pure core (inputs are git tree handles + ancestor + source commit;
output is a classified result; no IO except go-git object reads).
`outcomes` is the IO half — given a result, performs the merge commit
creation, ref advance, event emission, and conflict-event auto-closure.
`worker` is the assembly point that subscribes to the events-log,
runs the per-session goroutine loop, and routes results through outcomes.

Critical path: `merge-engine → outcomes → worker`. 3 deep. The
pure-vs-IO cut lets `merge-engine` be designed, implemented, and
tested in isolation (against fixture trees) before any other feature in
the epic exists.

### Child features

- `epic-auto-merger-merge-engine` — pure three-way merge via go-git,
  result classification (clean / safe-auto-resolve / hard-conflict),
  safe-auto-resolve heuristic detection (whitespace-only,
  non-overlapping additions, identical edits) — depends on: `[]`
- `epic-auto-merger-outcomes` — merge-commit creation with author/
  committer/trailer composition, draft ref advance, conflict_events
  row insert, `Resolves-Conflict` auto-closure, event emission — depends
  on: `[epic-auto-merger-merge-engine, epic-portal-api-events-log,
  epic-portal-git-storage]`
- `epic-auto-merger-worker` — per-session goroutine queue, events-log
  subscription, mode-aware filter, crash-replay on startup — depends on:
  `[epic-auto-merger-merge-engine, epic-auto-merger-outcomes,
  epic-portal-api-events-log, epic-portal-git-storage]`

### Decomposition risks

- **Safe-auto-resolve heuristics are the highest-correctness-sensitivity
  surface in the epic.** A wrong "whitespace-only" or "non-overlapping
  additions" detection silently corrupts user content. Mitigation:
  `merge-engine`'s design pass produces a canonical adversarial test
  corpus (real-world conflict shapes that should NOT auto-resolve but
  a naive implementation might) and locks it as a regression suite.
- **Per-session backpressure under burst.** A session pushing many
  commits at once fills the per-session queue. The bounded-capacity
  overflow emits an `auto-merger.backpressure` event so peers' UIs
  surface the lag. Cross-feature integration: the UI consumer must
  handle that event visually — flag during UI feature designs.
- **Replay correctness depends on `epic-portal-git-post-receive`'s
  transactional event emission.** If the event-emit isn't in the same
  transaction as the ref update, a crash between them could drop a
  commit from replay's view. Cross-epic invariant — flag during
  post-receive's design pass.
- **The auto-merger has privileged write access to `draft`.** This is a
  trust boundary (per SECURITY.md). The `outcomes` feature is the only
  place this privilege is exercised; design pass keeps the codepath
  narrow and auditable.
