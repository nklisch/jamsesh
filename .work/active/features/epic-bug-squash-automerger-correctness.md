---
id: epic-bug-squash-automerger-correctness
kind: feature
stage: done
tags: [bug, portal]
parent: epic-bug-squash
depends_on: []
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
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

## Architectural choice

**Chosen: collapse the two `sync.Map`s (`queues` + `running`) into one
`w.mu`-guarded `map[string]*sessionQueue`, and make the per-session lifecycle a
single locked decision.** The bug is that "a queue exists" and "a worker is
draining it" are tracked in two independent atomics mutated outside a common
lock, so an idle-out can interleave with `enqueue` and strand a buffered event.
The fix makes map-membership itself the running flag: an entry exists IFF a
worker goroutine owns draining it, and every create / push / idle-delete happens
under `w.mu`.

Rejected: (a) keep two maps but add lock ordering — fragile, still has the
push-after-delete window; (b) drop the idle-exit entirely and keep one goroutine
per session forever — unbounded goroutines. The single-map design is the
standard correct pattern and is race-detector-clean.

For the swallowed emit: **advance the ref (git is the source of truth for the
tip), then emit at-least-once with bounded retry on transient errors**; on
exhaustion escalate to ERROR + metric rather than silently dropping. Reordering
to emit-first is rejected (it would publish "draft advanced" before it has).
Full startup reconciliation (the real `replayScan`) is the principled long-term
fix but is out of scope here — filed as a follow-up (see Risks).

## Implementation Units

### Unit 1: Unified session-queue lifecycle (lost-event race)
**File**: `internal/portal/automerger/worker.go`
**Story**: `bug-squash-automerger-strands-commit-event` (High)

Replace the `queues` + `running` `sync.Map`s with one lock-guarded map:

```go
type sessionQueue struct {
    ch chan events.Event
}

type Worker struct {
    // ...existing config fields...
    mu       sync.Mutex
    sessions map[string]*sessionQueue // guarded by mu; membership == "worker owns draining"
    wg       sync.WaitGroup
    unsub    func()
    // onIdleDecision is a test-only hook invoked under mu inside the idle case,
    // after the len(ch) re-check decision is computed; nil in production.
    onIdleDecision func(sessionID string, willExit bool)
}

// enqueue creates the queue + spawns the worker (under mu) on first event, and
// pushes non-blockingly under mu so a concurrent idle-exit cannot strand it.
func (w *Worker) enqueue(ctx context.Context, e events.Event) {
    w.mu.Lock()
    sq, ok := w.sessions[e.SessionID]
    if !ok {
        sq = &sessionQueue{ch: make(chan events.Event, w.QueueSize)}
        w.sessions[e.SessionID] = sq
        w.wg.Add(1)
        go w.processSessionQueue(ctx, e.SessionID, sq)
    }
    select {
    case sq.ch <- e: // buffered; non-blocking under the lock
        w.mu.Unlock()
    default:
        w.mu.Unlock()
        w.emitBackpressure(ctx, e)
    }
}

// processSessionQueue idle case re-checks the buffer under mu before exiting.
case <-idle.C:
    w.mu.Lock()
    willExit := len(sq.ch) == 0
    if w.onIdleDecision != nil { w.onIdleDecision(sessionID, willExit) }
    if willExit {
        delete(w.sessions, sessionID)
        w.mu.Unlock()
        return
    }
    w.mu.Unlock()
    idle.Reset(w.IdleTimeout) // events arrived during the race; keep draining
```

**Implementation Notes**:
- `ensureSessionWorker` and the `running` map are deleted entirely.
- Spawning the goroutine under `w.mu` is safe: the worker only re-acquires `mu`
  on idle, never at startup, so no deadlock.
- `Start` initializes `w.sessions = make(map[string]*sessionQueue)`.
- Pushing under the lock is non-blocking (buffered + `default`), so the lock is
  never held across a blocking send.

**Acceptance Criteria**:
- [ ] No `commit.arrived` event is dropped when a session worker idles out
      concurrently with a new enqueue (verified under `-race`, see Testing).
- [ ] `go vet` / `go test -race ./internal/portal/automerger/...` clean.
- [ ] Backpressure still emitted when the buffer is genuinely full.

---

### Unit 2: At-least-once merge.succeeded / conflict emit (swallowed emit)
**Files**: `internal/portal/automerger/outcomes.go`, `internal/portal/automerger/worker.go`
**Story**: `bug-squash-automerger-swallows-merge-emit` (High)

After the durable `SetReference` (success) / `InsertConflictEvent` (conflict) /
`MarkConflictEventResolved` (resolve), retry the `Emit` on transient errors; on
exhaustion return a typed sentinel the worker logs at ERROR (not Warn) with a
recovery hint + metric.

```go
// emitWithRetry retries l.Emit after a durable side effect. It runs on a
// detached, bounded context (so worker-ctx cancellation during shutdown cannot
// re-drop the event), classifies transience itself (Emit returns RAW store
// errors — see codex gate finding), and returns ErrEmitAfterSideEffect wrapping
// the last error when all attempts fail.
var ErrEmitAfterSideEffect = errors.New("automerger: side effect committed but event emit failed")

func (a *Applier) emitWithRetry(ctx context.Context, orgID, sessionID, typ string, data []byte) error {
    emitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), emitGraceTimeout)
    defer cancel()
    // bounded loop; retry only when deperr.WrapDBIfTransient(err) is transient.
    ...
}
```

- `applySuccess`: ref advance → `emitWithRetry(... "merge.succeeded" ...)`.
- `applyConflict`: insert row → `emitWithRetry(... "conflict.detected" ...)`.
- `tryResolveConflict`: `MarkConflictEventResolved` → `emitWithRetry(...
  "conflict.resolved" ...)` (same swallowed-emit class — codex finding #3).
- `worker.go` `processEvent`: when `Apply` returns `ErrEmitAfterSideEffect`,
  `slog.ErrorContext` with session_id + the advanced SHA + "manual recovery:
  re-push the source ref", and increment `AutoMergerOutcomes{outcome="emit_failed"}`
  (update the metric's help text — it currently lists only
  succeeded/conflict/backpressure). Other Apply errors stay Warn.

**Implementation Notes** (incorporating the codex gate):
- **Emit returns raw errors**: `events.Log.Emit` propagates the bare
  `WithTx`/store error, NOT a `deperr`-wrapped one. `emitWithRetry` must call
  `deperr.WrapDBIfTransient(err)` (or a local classifier) itself before deciding
  to retry — don't `errors.Is` a bare error against the transient sentinel.
- **Possible duplicate emit is accepted, not prevented**: a commit-phase error
  returned *after* `tx.Commit` is ambiguous — the row may have committed. A retry
  can therefore create a second `merge.succeeded`/`conflict.detected`/
  `conflict.resolved` row with a fresh seq. This is tolerated because these
  events are idempotent for consumers (keyed on `merge_commit_sha` / event `id`):
  a duplicate is a no-op replay, strictly better than the silent-drop bug.
  Acceptance includes a test asserting duplicates are consumer-idempotent.
- **Detached ctx**: post-side-effect emit runs on `context.WithoutCancel(ctx)` +
  a bounded `emitGraceTimeout`, so shutdown cancellation between the durable
  write and the emit doesn't re-introduce the drop.
- Keep retries small and bounded (e.g. 3 attempts) so a wedged DB doesn't block
  the session worker; full reconciliation (real `replayScan`) stays a parked
  follow-up (see Risks).

**Acceptance Criteria**:
- [ ] A transient Emit failure is retried (on the detached ctx); a persistent
      one returns `ErrEmitAfterSideEffect` and the worker logs ERROR + increments
      `emit_failed` (not a silent Warn).
- [ ] The same path covers `merge.succeeded`, `conflict.detected`, AND
      `conflict.resolved`.
- [ ] The draft ref / conflict row state is unchanged by the emit-failure path
      (the side effect is not rolled back; git/DB remain the source of truth).
- [ ] A duplicate emit (ambiguous-commit retry) is consumer-idempotent — a test
      asserts two `merge.succeeded` for one mergeSHA produce one effective client
      state.
- [ ] Worker-ctx cancellation after the durable write still lets the emit attempt
      run (detached ctx), within `emitGraceTimeout`.

---

### Unit 3: errors.Is for store.ErrNotFound
**Files**: `internal/portal/automerger/worker.go:338`, `internal/portal/automerger/outcomes.go:234`
**Story**: `bug-squash-errors-is-not-used-errnotfound` (Low) — depends on Units 1 & 2 (same files)

```go
// worker.go refModeForSession
if !errors.Is(err, store.ErrNotFound) { return "", fmt.Errorf("get ref mode: %w", err) }
// outcomes.go tryResolveConflict
if errors.Is(err, store.ErrNotFound) { return nil }
```

**Acceptance Criteria**:
- [ ] A wrapped `store.ErrNotFound` (via `fmt.Errorf("%w")`) is still classified
      as not-found in both sites (test with a wrapping stub store).

---

### Unit 4: classify the diff subprocess exit code
**File**: `internal/portal/automerger/heuristics.go:228`
**Story**: `bug-squash-diff-exit-code-ignored` (Low) — independent (heuristics.go only)

Extract run+classify into a testable helper; accept exit 0/1, error on exit ≥2
or non-`*exec.ExitError`:

```go
// runDiff runs `diff -u` and returns its stdout. diff exit 0 (identical) and 1
// (differences) are success; exit >= 2 (trouble) or a non-exit error is returned.
func runDiff(baseFile, otherFile string) ([]byte, error)
```

**Acceptance Criteria**:
- [ ] `runDiff` returns output for exit 0/1 and a non-nil error for exit ≥2 or a
      missing `diff` binary (tested via `classifyDiffErr` over synthetic
      `*exec.ExitError` values).
- [ ] `diffAddOnly` propagates the error instead of parsing garbage.

## Implementation Order
1. Unit 1 (lifecycle race) and Unit 2 (emit) — both touch `worker.go`; bundle
   into one worktree/agent to avoid same-file conflicts. Unit 4 (heuristics.go)
   is independent and may run in parallel.
2. Unit 3 (errors.Is) last — it edits the post-refactor `worker.go` + `outcomes.go`
   (declared via its `depends_on`).

## Testing
- **Unit 1 (race)**: `worker_race_test.go` — a `-race` stress test: N sessions,
  tiny `IdleTimeout` (~1ms), interleave `enqueue` with idle-outs over many
  iterations; assert `processed == enqueued` (count via a stub Applier/store).
  Plus a deterministic test using `onIdleDecision` to force the
  idle-decision-then-enqueue interleaving and assert no stranded event.
- **Unit 2 (emit)**: inject a `*events.Log` backed by a store whose `WithTx`
  fails transiently then succeeds (retry path), and one that always fails
  (escalation path); assert ref advanced, retry happened, sentinel returned,
  ERROR logged, metric incremented. Reuse the existing fakeClock + stub store.
- **Unit 3**: stub store returning `fmt.Errorf("wrap: %w", store.ErrNotFound)`;
  assert both call sites treat it as not-found.
- **Unit 4**: unit-test `classifyDiffErr` with synthetic exit codes 0/1/2 and a
  non-ExitError.

## Risks
- **Emit-after-advance is best-effort, not transactional.** Git (on disk) and
  the event log (DB) are separate stores; true atomicity is impossible. Bounded
  retry + ERROR escalation fixes the silent-drop bug, but a DB outage spanning
  all retries still leaves a gap recoverable only by re-push. **Follow-up filed
  conceptually**: implement the real `replayScan` (startup reconciliation:
  compare each session's draft tip to the last `merge.succeeded` seq and
  back-emit) — out of scope for this bug-fix feature; feature-design should
  surface this to autopilot to park as a new story rather than expand scope.
- **Deterministic race testing**: the `onIdleDecision` hook is the seam that
  makes the race test deterministic; without it the `-race` stress test is
  probabilistic. Both are included.

## Design decisions
- **Lost-event fix shape**: single `mu`-guarded map (membership == running),
  push under lock — over lock-ordering or perpetual-goroutine alternatives. Rationale:
  only design that closes the push-after-delete window; standard, race-clean.
- **Emit durability**: advance-then-retry-then-escalate (at-least-once,
  best-effort) over emit-first or full reconciliation. Rationale: fixes the
  actual silent-drop bug proportionately; defers the heavyweight replayScan as a
  parked follow-up rather than ballooning this feature.
- **Emit retry semantics (codex gate)**: retry on a detached
  `context.WithoutCancel`+timeout ctx; classify transience explicitly
  (`deperr.WrapDBIfTransient`, since `Emit` returns raw errors); TOLERATE a
  possible duplicate emit on ambiguous commit-phase failure (consumers are
  idempotent on `merge_commit_sha`/event `id`) rather than adding a dedup store
  surface; cover `conflict.resolved` too. Rationale: a duplicate replay is
  strictly better than the silent drop, and detached-ctx prevents shutdown from
  re-dropping.
- **diff classification**: extract a `runDiff`/`classifyDiffErr` helper for
  testability rather than inline exit-code handling. Rationale: forcing `diff`
  exit 2 in-process is awkward; a pure classifier is unit-testable.

## Other agent review

Codex (cross-model, xhigh) reviewed this feature design. Verdict: Unit 1 (race),
Unit 3 (errors.Is), Unit 4 (diff) sound as written; Unit 2 (emit) tightened
before implementation.

**Accepted & applied to Unit 2:**
- `Emit` returns raw store errors — `emitWithRetry` must classify transience
  itself via `deperr.WrapDBIfTransient` (not `errors.Is` on a bare error).
- Retry can double-emit on an ambiguous commit-phase failure — tolerate it
  (consumers idempotent on `merge_commit_sha`/event id) + test the duplicate is
  a no-op replay, rather than adding a dedup surface.
- `conflict.resolved` (`tryResolveConflict`) has the same swallowed-emit bug —
  covered by the same retry/escalation path.
- Post-side-effect emit runs on a detached `context.WithoutCancel`+timeout ctx
  so shutdown cancellation can't re-drop the event.
- nice-to-have: update the `AutoMergerOutcomes` metric help text for the new
  `emit_failed` label; add a small `diff` exit-1/2 integration case alongside
  the pure `classifyDiffErr` unit test.

**Confirmed sound (no change):** the single-`mu`-guarded `sessions` map closes
the push-after-delete stranding window; spawning the worker under `mu` is
deadlock-free; no idle re-check missed-wakeup; fanout is not an `Emit` error
source.

## Implementation summary

All 4 child stories implemented and advanced to `stage: review` (per-story `implement: bug-squash-*` commits). Each landed a failing-first regression test; the codex feature-gate findings (see `## Other agent review`) were applied during design and honored in implementation. Verification at the orchestrator level: `go build ./...` + `go vet` clean; backend `-race`/package tests and frontend `vitest` (764 passing) + `svelte-check` green; `sqlc generate` matches spec.

## Final-gate fix

**Finding 1 (BLOCKING): `conflict.resolved` emit swallowed after side effect.**
`applySuccess` in `outcomes.go` Warn-logged and continued when `tryResolveConflict`
returned `ErrEmitAfterSideEffect`. Fixed: added `errors.Is(err, ErrEmitAfterSideEffect)`
check before the existing Warn path; escalates with `slog.ErrorContext` +
`AutoMergerOutcomes{outcome="emit_failed"}` metric increment and returns the error.
Non-emit Resolves-Conflict failures remain best-effort Warn.

**Finding 6 (TEST-INTEGRITY): `emit_retry_test.go` ~:423 claimed `conflict.resolved`
coverage but only tested the `merge.succeeded` emit path.**
Added `TestApply_EmitRetry_ConflictResolved_EscalatesOnEmitFailure`: uses a
`failAfterNStore` that allows exactly 1 successful `WithTx` call (merge.succeeded)
then always fails, so the conflict.resolved emit is actually exercised. Asserts
`ErrEmitAfterSideEffect` returned AND the conflict row is marked resolved (durable
side effect committed before the failed emit).
