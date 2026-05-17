---
id: epic-auto-merger-worker
kind: feature
stage: drafting
tags: [portal]
parent: epic-auto-merger
depends_on: [epic-auto-merger-merge-engine, epic-auto-merger-outcomes, epic-portal-api-events-log, epic-portal-git-storage]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Auto-Merger — Worker Runtime

## Brief

The orchestration layer that turns `commit.arrived` events into merge
attempts. Subscribes to the events-log in-process notifier; for each
`commit.arrived` event on a sync-mode ref, enqueues a job into a
per-session queue; spawns a per-session worker goroutine if one isn't
already running; drains the queue by calling `merge-engine` for the
classification and `outcomes` for the side effects.

**Concurrency model** (locked at epic-design):

- One goroutine per session, created on first commit, drains a
  per-session in-memory FIFO queue.
- Bounded queue capacity (configurable; default ~256 entries). Overflow
  emits an `auto-merger.backpressure` event so peers' UIs surface the lag.
- Idle timeout: goroutine exits after N seconds of empty queue. New commit
  on that session re-spawns. No fixed pool to tune.
- Cross-session is fully parallel (one session's backpressure doesn't
  block others).

**Mode-aware filtering**: at enqueue time, look up the ref's mode from
the `ref_metadata` table (owned by `epic-portal-api-sessions-rest` via the
`fork` mode column). Skip enqueue for isolated refs entirely — the
auto-merger does not touch isolated work.

**Subscription to events-log**: at startup, the worker calls
`events-log.SubscribeCommitArrived(callback)`. The callback receives the
event and pushes onto the per-session queue. This is the same in-process
pub-sub mechanism the WebSocket gateway uses; the worker is just another
subscriber.

**Crash recovery / replay on startup** (locked at epic-design):

- On portal startup, scan the events table for `commit.arrived` events
  newer than the latest auto-merger-touched draft position per session.
  For each, idempotency-check: if the source-commit is already an
  ancestor of the current draft tip (via go-git), skip; otherwise
  enqueue.
- The "latest auto-merger-touched position" is the draft tip's sha
  (read from go-git at startup). No separate state table.
- Replay is idempotent because the ancestor check is decisive.

**Lifecycle**: worker exposes `Start(ctx)` and `Stop(ctx)` — Start
performs the replay scan, registers the events-log callback, then
returns; Stop drains queues and waits for all per-session goroutines to
finish before returning. Used by the portal binary's main.go for clean
shutdown.

Does NOT contain the merge logic (`merge-engine`). Does NOT contain the
result handling (`outcomes`). Does NOT cover mode storage — that's
sessions-rest. Does NOT cover the events-log subscription primitive
itself — that lives in `epic-portal-api-events-log`.

## Epic context

- Parent epic: `epic-auto-merger`
- Position in epic: assembly point. Subscribes to events-log; invokes
  merge-engine; routes to outcomes. The only feature in this epic that
  runs goroutines.

## Foundation references

- `docs/ARCHITECTURE.md` — The auto-merger section (workers triggered
  by post-receive events), Data flow: a turn > step 8
- `docs/SPEC.md` — Dual mode (skip isolated refs)
- `docs/PRINCIPLES.md` — Recovery is `git fetch` (the worker's replay
  story aligns: no hidden state, recovery is "scan and replay")

## Inherited epic design decisions

- **Concurrency**: per-session goroutine + bounded FIFO queue + idle
  timeout. No global pool.
- **Crash recovery**: scan + idempotent replay via go-git ancestor
  check. No merger state table.
- **Mode filtering**: at enqueue time, look up `ref_metadata.mode`;
  skip isolated.

## Decomposition risks

- **Backpressure under burst.** A session pushing many commits at once
  fills the per-session queue. The bounded-capacity overflow path
  (emitting `auto-merger.backpressure`) is the safety valve, but the
  design pass needs to confirm the event consumer (UI) handles it
  visually. Cross-feature integration check during feature-design.
- **Replay correctness depends on post-receive's transactional
  emission.** If `epic-portal-git-post-receive`'s event emit isn't in
  the same transaction as the ref update, a crash between them could
  drop a commit from replay's view. Cross-epic invariant — flag in
  post-receive's design pass.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
