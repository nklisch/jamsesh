---
id: epic-auto-merger-worker
kind: feature
stage: done
tags: [portal]
parent: epic-auto-merger
depends_on: [epic-auto-merger-merge-engine, epic-auto-merger-outcomes, epic-portal-api-events-log, epic-portal-git-storage]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
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

## Design decisions

- **In-process pub/sub addition**: extend `internal/portal/events/` with a `Subscriber` channel API. `Log.Subscribe(ch chan<- Event) (unsubscribe func())` and an internal subscriber list that `Emit`/`EmitBatch` fan out to. This is the minimal cross-feature surface needed to make worker reactive. Add as an additive change in THIS story (worker owns the design + impact).
- **Per-session goroutine**: `manager` struct keeps `map[sessionID]chan Event`. On first event for a session: spawn worker + queue. Idle timeout (default 30s): worker exits cleanly. Bounded queue (default 256): overflow emits `auto-merger.backpressure` event.
- **Mode filtering**: worker reads `ref_modes` table at enqueue time. Falls back to `session.default_mode`. Skips isolated refs.
- **Merge orchestration**: for each commit.arrived:
  1. Parse `ref` and `sha` from event payload
  2. If ref mode != "sync": skip
  3. Open repo via storage; resolve sourceCommit, draftTipCommit (the current `jam/<sess>/draft` ref tip)
  4. Compute merge base via go-git `object.MergeBase`
  5. Call `automerger.Merge(ctx, repo, source, draft, ancestor)`
  6. Call `automerger.Applier.Apply(ctx, ApplyInput{...})` with the result
  7. Log + carry on
- **Replay on startup**: on Start(ctx): for every active session, list `commit.arrived` events with `seq > <last-seen-seq>`. The "last-seen" baseline is computed by scanning the draft ref's history: any commit reachable from draft tip is already-processed. For unprocessed source commits, enqueue. Idempotency via the ancestor check.
- **Single story**: `auto-merger-worker-manager-and-subscriber`.

## Implementation Units

### Unit 1: events.Log Subscribe extension

**File**: `internal/portal/events/log.go` (edit)

Add:
```go
type Subscriber chan Event

// Subscribe returns an unsubscribe func. Buffer 64 events on the
// channel; if the consumer falls behind, events are dropped silently
// (the worker scans on startup, so transient drops are tolerable).
func (l *Log) Subscribe(typeFilter string) (Subscriber, func())
```

Internally, `Log` keeps `[]subscriber{ch, filter}`. `Emit`/`EmitBatch` non-blocking-send to each matching subscriber.

### Unit 2: Worker manager

**File**: `internal/portal/automerger/worker.go`
**Story**: `epic-auto-merger-worker-manager-and-subscriber`

```go
package automerger

type Worker struct {
    Store        store.Store
    Storage      storage.Service
    Log          *events.Log
    Applier      *Applier
    PortalHost   string
    IdleTimeout  time.Duration // default 30s
    QueueSize    int           // default 256
}

func (w *Worker) Start(ctx context.Context) error {
    // 1. Subscribe to commit.arrived events
    // 2. Replay scan (across all active sessions)
    // 3. Spawn dispatch goroutine that fans events to per-session queues
    // 4. Return; dispatch + per-session goroutines run until ctx cancelled
}

func (w *Worker) Stop(ctx context.Context) error {
    // Wait for all queues to drain or ctx timeout
}
```

### Unit 3: Per-session worker loop

Internal helper. On receiving an event from the queue:
1. Decode payload to extract ref + sha
2. Check mode (ref_modes -> session.default_mode); skip if isolated
3. Open repo
4. Run merge + apply
5. Log errors but keep draining queue

## Implementation Order

Single story.

## Testing

- Subscribe round-trip: emit → subscriber receives
- Worker happy path: emit commit.arrived → merge succeeds → draft advanced + merge.succeeded emitted
- Isolated ref: emit → worker skips (verified no draft change)
- Replay: pre-populate events table with unprocessed commit; Start; verify worker processes it
- Backpressure: fill queue beyond capacity; verify auto-merger.backpressure event emitted
- Stop: pending events in queue drain before Stop returns

## Risks

- **Channel-drop subscriber semantics**: dropping events is fine because the replay scan on next startup catches them, BUT in a long-running session a missed event could lag indefinitely. Mitigation: log every drop at Warn level; consider a periodic resync pass in v0.x.
- **Per-session goroutine churn**: many short-lived sessions could spawn/exit frequently. The 30s idle timeout amortizes; if profiling shows issues, switch to a fixed worker pool.

## Implementation summary

Single child story done. The orchestration layer is live: subscribes to events.Log, fans events to per-session goroutines, calls merge-engine + outcomes.Apply for each sync-mode commit.

## Review

**Verdict**: Approve. Auto-merger epic has all 3 features done (merge-engine, outcomes, worker). The auto-merger runs end-to-end.
