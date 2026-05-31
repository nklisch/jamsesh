---
id: epic-bug-squash-worker-lifecycle
kind: feature
stage: review
tags: [bug, portal]
parent: epic-bug-squash
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Background-worker lifecycle & shared-state concurrency

## Brief

The portal runs several long-lived background workers — pg advisory-lease
heartbeat/retention, the WS-gateway ticket janitor, the object-store lifecycle
LRU/idle reaper, and the rate limiter — each holding shared mutable state behind
locks, channels, and tickers. The bug-scan found six concurrency/lifecycle
defects across them: a `Release`-vs-heartbeat race on one `*sql.Conn`, a
retention cutoff frozen at startup, a `Stop`-double-close panic, a slow-consumer
close that leaves a dead conn subscribed, an LRU pass that evicts just-active
sessions off a stale snapshot, and an inconsistent rate-limit reservation
cancel.

This feature delivers correct lifecycle and shared-state handling for these
workers: no double-close panics, no frozen time references in ticker loops, no
unsynchronized concurrent use of a pooled connection, and eviction/idle
decisions re-validated at decision time. It covers correctness of the existing
workers only — it does NOT add new workers, change lease/storage semantics, or
alter the rate-limit policy values.

## Epic context
- Parent epic: `epic-bug-squash`
- Position in epic: independent backend feature — parallelizable with the other
  backend features (distinct packages: lease, wsgateway, objectstore, ratelimit).

## Foundation references
- `docs/ARCHITECTURE.md` — Portal § WS gateway, Playground destroyer, storage
- Patterns: `ticker-sweep-loop`, `per-package-clock-interface`

## Child stories (pre-existing, from bug-scan — re-parented here)
- `bug-squash-pghandle-heartbeat-conn-race` — Medium, concurrency — `internal/portal/lease/postgres.go:219`
- `bug-squash-lease-retention-frozen-now` — Medium, time-numbers — `internal/portal/lease/retention.go:25`
- `bug-squash-lru-evicts-hot-sessions` — Medium, concurrency — `internal/portal/storage/objectstore/lifecycle.go:350`
- `bug-squash-ticketstore-stop-double-close` — Medium, concurrency — `internal/portal/wsgateway/tickets.go:92`
- `bug-squash-gateway-slow-consumer-close` — Low, concurrency — `internal/portal/wsgateway/gateway.go:127`
- `bug-squash-ratelimit-reservation-cancel` — Low, concurrency — `internal/portal/ratelimit/store.go:106`

## Architectural choice

**Per-worker local fixes — no shared abstraction.** Each defect lives in a
distinct file/package (`lease/postgres.go`, `lease/retention.go`,
`wsgateway/tickets.go`, `wsgateway/gateway.go`, `objectstore/lifecycle.go`,
`ratelimit/store.go`) with its own lifecycle idiom. The right fix is local to
each; manufacturing a common "worker lifecycle" helper would couple unrelated
workers. The 6 stories are independent (different files) and fully
parallelizable. Concurrency fixes get `go test -race`; time fixes get a fake
clock.

## Implementation Units

### Unit 1: pgHandle Release waits for the heartbeat goroutine (conn race)
**File**: `internal/portal/lease/postgres.go`
**Story**: `bug-squash-pghandle-heartbeat-conn-race` (Medium)

`Release` closes `h.done` then immediately uses `h.conn` (advisory unlock +
`conn.Close()`), but the heartbeat goroutine may be mid-`PingContext` on the
same `*sql.Conn` — `database/sql` forbids concurrent use of one `*sql.Conn`.

Add `heartbeatDone chan struct{}`; `runHeartbeat` does `defer close(h.heartbeatDone)`;
`Release` waits for it before touching the conn:

```go
h.once.Do(func() {
    close(h.done)
    <-h.heartbeatDone // wait for the ping goroutine to exit (bounded by one ping interval)
    // ...now sole owner of h.conn: advisory unlock, MarkLeaseReleased, conn.Close()
})
```

**Implementation Notes**: after `close(h.done)` the goroutine returns either
immediately (blocked in select) or after finishing an in-flight ping (bounded by
the ping ctx timeout = interval), so the wait is bounded. The `lost`-path exit
also closes `heartbeatDone` via the same defer. Initialize `heartbeatDone` where
`pgHandle` is constructed inline in `Acquire` (there is no `newPgHandle` helper
today), alongside `done`/`lost`, and before `go h.runHeartbeat(...)`.

**Acceptance Criteria**:
- [ ] No concurrent use of `h.conn` by Release and the heartbeat (verified under
      `-race` with the postgres testcontainer: acquire → let heartbeat tick →
      Release concurrently).
- [ ] Release remains idempotent and bounded (no hang if the heartbeat is wedged
      beyond one interval — the ping ctx timeout guarantees progress).

---

### Unit 2: Lease retention recomputes the cutoff each tick (frozen now)
**File**: `internal/portal/lease/retention.go` (+ call site `cmd/portal/main.go`)
**Story**: `bug-squash-lease-retention-frozen-now` (Medium)

Replace the fixed `now time.Time` param with a time source read each tick:

```go
func RunRetention(ctx context.Context, s store.LeaseStore, interval, retentionAfter time.Duration, nowFn func() time.Time) error {
    // ...
    case <-ticker.C:
        cutoff := nowFn().Add(-retentionAfter) // recomputed every tick
        ...
}
```

**Implementation Notes**: a `func() time.Time` is the minimal change (no new
interface). `main.go` passes `func() time.Time { return time.Now().UTC() }`;
tests pass a fake-clock-backed fn and advance it between ticks. Update the
doc-comment ("now is the reference time…") to reflect per-tick evaluation.

**Acceptance Criteria**:
- [ ] On a long-running loop, the cutoff advances with wall time (test: advance
      the injected clock across two ticks, assert `DeleteReleasedLeasesOlderThan`
      is called with an advancing cutoff).

---

### Unit 3: TicketStore.Stop is idempotent (double-close panic)
**File**: `internal/portal/wsgateway/tickets.go`
**Story**: `bug-squash-ticketstore-stop-double-close` (Medium)

Guard the channel close with `sync.Once` so a second `Stop()` is a no-op
instead of a `close of closed channel` panic:

```go
type TicketStore struct { /* ... */ stopOnce sync.Once }

func (ts *TicketStore) Stop() {
    ts.mu.Lock(); defer ts.mu.Unlock()
    if !ts.started { return }
    ts.stopOnce.Do(func() { close(ts.stopCh) })
}
```

**Implementation Notes**: Start→Stop→Start reuse is out of scope (Start is
documented idempotent-once; the store is constructed fresh per server). The Once
fixes the reported repeated-Stop panic minimally.

**Acceptance Criteria**:
- [ ] Calling `Stop()` twice does not panic; the janitor still exits after the
      first Stop.

---

### Unit 4: Gateway unregisters a slow-consumer conn on close
**File**: `internal/portal/wsgateway/gateway.go`
**Story**: `bug-squash-gateway-slow-consumer-close` (Low)

When `fanout` closes a slow consumer, proactively `unregister` it so subsequent
fanout passes stop iterating a dead conn (instead of waiting up to ~30s for the
handler's next heartbeat to fail):

```go
default:
    c.closeOnce.Do(func() {
        c.ws.Close(websocket.StatusPolicyViolation, "subscriber too slow")
    })
    g.unregister(c) // idempotent; handler's deferred unregister still runs harmlessly
```

**Implementation Notes**: `unregister` takes `g.mu`; calling it here is safe —
fanout iterates a pre-taken snapshot `list`, not the live map. Double-unregister
(here + handler defer) is a no-op (map delete on absent key). `unregister`
should guard against deleting into a nil/absent inner map.

**Acceptance Criteria**:
- [ ] A conn whose `send` buffer overflows is removed from `g.subs` promptly
      (test: fill a conn's buffer, fanout one event, assert the conn is no longer
      in the subscription set).

---

### Unit 5: Rate-limit reservation cancelled consistently on the !OK path
**File**: `internal/portal/ratelimit/store.go`
**Story**: `bug-squash-ratelimit-reservation-cancel` (Low)

The `!r.OK()` early return omits a cancel; AND (codex gate) every cancel must be
`CancelAt(now)` with the store's injected clock — bare `Cancel()` is
`CancelAt(time.Now())` (wall clock), which restores tokens incorrectly under the
fake clock used in tests and is inconsistent with `ReserveN(now, 1)`:

```go
now := s.clock.Now()
r := e.minuteLimiter.ReserveN(now, 1)
if !r.OK() { r.CancelAt(now); return false, 60 * time.Second }
if d := r.DelayFrom(now); d > 0 { r.CancelAt(now); return false, d }
// hourly branch: rh.CancelAt(now); r.CancelAt(now)
```

**Implementation Notes**: change ALL existing `r.Cancel()`/`rh.Cancel()` to
`CancelAt(now)` (lines ~118/126/131) plus add the missing `!OK` cancel. A `!OK`
reservation is a documented no-op, so the deny decision is unchanged; the
`CancelAt(now)` switch is the correctness fix for clock-injected token
restoration. The two-limiter sequence stays best-effort under concurrency.

**Acceptance Criteria**:
- [ ] All early-return paths in `Allow` cancel any reservation they hold; the
      allow/deny decision is unchanged (existing ratelimit tests still pass).

---

### Unit 6: LRU eviction re-validates the victim at decision time (evicts hot)
**File**: `internal/portal/storage/objectstore/lifecycle.go`
**Story**: `bug-squash-lru-evicts-hot-sessions` (Medium)

The LRU loop evicts from a stale `active` snapshot; a session touched by
`AcquireForRequest` after the snapshot is still evicted. A naive "re-read
lastActive then release" still has a TOCTOU (codex gate): `AcquireForRequest`
checks `!releasing` then touches+returns the handle, and that can interleave
with the LRU re-read either side of the `releasing` CAS in `releaseWithReason`.
The correct fix is a **double-checked claim**, splitting `releaseWithReason`
into the CAS + the post-CAS work, and re-checking on the acquire side too:

```go
// releaseWithReason stays the public entry: Load -> CAS releasing -> releaseClaimed.
// New internal helper does steps 2-5 assuming the claim is already held:
func (m *LifecycleManager) releaseClaimed(ctx context.Context, sessionID string, entry *sessionEntry, reason string) error

// LRU loop: claim FIRST, then validate the claimed entry.
victim := active[lruIdx]
raw, ok := m.sessions.Load(victim.sessionID)
if !ok { drop victim; continue }
entry := raw.(*sessionEntry)
if !entry.releasing.CompareAndSwap(false, true) { drop victim; continue } // already releasing
if entry.lastActive().After(victim.lastActive) {
    entry.releasing.Store(false) // touched since the snapshot — unclaim, not a cold victim
    drop victim; continue
}
_ = m.releaseClaimed(ctx, victim.sessionID, entry, "lru")
drop victim
```

```go
// AcquireForRequest: re-check releasing AFTER touching, so a claim that landed
// concurrently with our touch is honored (back off instead of using the handle).
if !entry.releasing.Load() {
    entry.touchLastActive(m.now())
    if entry.releasing.Load() { // an eviction claimed us during the touch — wait/retry
        select { case <-ctx.Done(): return nil, ctx.Err(); case <-time.After(10*time.Millisecond): }
        continue
    }
    return entry.handle, nil
}
```

**Implementation Notes**: once the LRU CAS sets `releasing=true`, no new acquire
will touch the entry (acquire waits on `releasing`); the post-CAS `lastActive`
re-check catches a touch that landed *before* the claim; the acquire-side
post-touch re-check catches a touch that landed *after* the claim. Together they
close both orderings — this is the standard double-checked claim. `releaseClaimed`
must NOT re-CAS (the caller already claimed). Idle eviction (first pass) keeps
calling `releaseWithReason` (which still CASes).

**Acceptance Criteria**:
- [ ] A session touched (lastActiveAt bumped) after the snapshot but before the
      LRU decision is NOT evicted (test: seed two sessions over cap, bump the
      LRU one's lastActive, assert the other is evicted instead).

## Implementation Order

All 6 units are independent (distinct files/packages) and parallelizable. No
story-level `depends_on`. `implement-orchestrator` can fan them out; the only
same-package pairs (`lease/`: Units 1+2; `wsgateway/`: Units 3+4) touch
different files so they don't conflict.

## Testing
- **Unit 1**: postgres testcontainer + `-race`; acquire, let the heartbeat tick,
  Release concurrently; assert no race and conn returned cleanly.
- **Unit 2**: fake `nowFn` advanced between ticks; assert advancing cutoff.
- **Unit 3**: `Stop()` twice → no panic; janitor goroutine exits.
- **Unit 4**: stub conn with a size-0/full `send`; fanout one event; assert
  unregistered.
- **Unit 5**: existing ratelimit table tests re-run; add a burst-exceeded case
  asserting deny + Retry-After unchanged.
- **Unit 6**: two oversize sessions; bump the LRU candidate's lastActive between
  snapshot and decision (via a hook or direct entry mutation); assert the other
  is the victim.

## Risks
- **Unit 1 wait bound**: if the heartbeat ping hangs longer than its ctx timeout
  (shouldn't — `PingContext` honors ctx), Release waits one interval. Acceptable;
  documented.
- **Unit 6 soft cap for hot sets**: LRU never force-evicts an actively-used
  session. If every candidate keeps being touched each tick, the cache can stay
  over `CacheMaxBytes` indefinitely (not just "until next tick"). This is
  intended — the cap is soft for the live working set; eviction targets cold
  sessions only. Documented so it isn't mistaken for a leak.

## Design decisions
- **No shared worker abstraction**: per-file local fixes over a unifying helper —
  the workers are unrelated; coupling them would be over-abstraction (codex epic
  gate confirmed "worker-lifecycle broad but acceptable if per-worker").
- **Unit 1 fix shape**: wait-for-heartbeat-exit over a conn mutex — simpler, no
  lock held across a ping; bounded by the ping timeout.
- **Unit 3 fix shape**: `sync.Once` over `started=false` reset — minimal,
  directly kills the double-close panic without inventing a restart lifecycle.
- **Unit 6 fix shape**: re-validate victim at decision time over a full
  live-read rewrite — proportionate; matches the idle pass's existing
  `releasing` discipline.

## Other agent review

Codex (cross-model, xhigh) reviewed this design. Verdict: sound after two
must-fixes. Units 1/2/3/4 confirmed sound (Unit 1 deadlock-free across normal /
immediate / in-flight-ping / Lost exits; Unit 4 unregister-from-fanout safe; no
double-close).

**Accepted & applied:**
- **Unit 6 (TOCTOU)**: a bare re-read still races acquire's check-touch-return.
  Replaced with a double-checked claim — LRU CASes `releasing` first then
  re-validates `lastActive` (via a new `releaseClaimed` helper), and
  `AcquireForRequest` re-checks `releasing` after `touchLastActive`. Closes both
  interleavings.
- **Unit 5 (`CancelAt`)**: the store uses an injected clock, so all reservation
  cancels must be `CancelAt(now)` (bare `Cancel()` = wall-clock `time.Now()`,
  wrong under the fake clock) — applied to all branches plus the new `!OK` cancel.
- **Unit 6 over-cap (nice-to-have)**: documented honestly — the cap is soft for
  the live working set; hot sessions are never force-evicted.
- **Unit 1 wording**: `pgHandle` is built inline in `Acquire` (no `newPgHandle`);
  init `heartbeatDone` there.

**Confirmed sound (no change):** Unit 1 wait is bounded + deadlock-free; Unit 4
fanout→unregister is lock-safe; Unit 3 `sync.Once`; Unit 2 `nowFn`.

## Implementation summary

All 6 child stories implemented and advanced to `stage: review` (per-story `implement: bug-squash-*` commits). Each landed a failing-first regression test; the codex feature-gate findings (see `## Other agent review`) were applied during design and honored in implementation. Verification at the orchestrator level: `go build ./...` + `go vet` clean; backend `-race`/package tests and frontend `vitest` (764 passing) + `svelte-check` green; `sqlc generate` matches spec.
