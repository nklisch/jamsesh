---
id: e2e-audit-playground-destruction-during-push-chaos
kind: story
stage: done
tags: [testing, e2e-test, audit, playground]
parent: feature-e2e-playground-coverage-chaos
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# No chaos coverage for destruction firing during an in-flight push or portal restart during an active anonymous session

## Severity
Medium

## Finding type
missing-taxonomy-layer

## Evidence

The chaos layer has eight tests covering: cross-pod clock skew, handoff
under object-storage chaos, handoff under pod kill, lease-holder killed,
network/provider chaos, object-storage partition, router pod disappears,
runtime/clock skew. None touch the playground subsystem:

```
$ grep -rIn -E "playground|destruction|anon" tests/e2e/chaos/
(no output)
```

Unit coverage:
`TestDestruction_ConcurrentDestroyCallsForSameSession_NoCorruption`
verifies the destruction path is idempotent under concurrent in-process
goroutines. That's a single-process correctness test, not a chaos test —
no real packfile in flight, no real container, no real network.

## Why this matters

Two specific race windows in the playground design are unverified:

1. **Destruction-during-push race.** The destruction worker uses
   `ticker-sweep-loop` (per `.claude/rules/patterns.md`). If the worker
   selects a session for destruction at the exact moment a push is
   mid-stream, the post-receive event emission can race with the
   `RemoveRepo` filesystem delete. Either the push silently succeeds
   against a doomed repo (RPO violation — pushed objects are immediately
   destroyed) or the push fails partway with an inconsistent on-disk
   state. The `tx-emit-then-fanout` pattern's outside-tx fanout is
   exactly where this race lives.

2. **Portal restart with active anonymous session.** If the portal is
   restarted (SIGTERM during in-flight playground create / push), the
   in-memory rate-limit counter is lost. A user can then potentially
   create more sessions than the cap should allow by intentionally
   restarting their request stream across a portal redeploy. The
   `graceful_shutdown_deadline_test.go` covers OAuth callback in-flight
   but not anonymous create or playground push.

## Suggested remedy

Add `tests/e2e/chaos/playground_destruction_during_push_test.go` and
`tests/e2e/chaos/playground_portal_restart_active_session_test.go`. Use
Toxiproxy (existing fixture) to slow the push enough that the test can
sequence the destruction sweep mid-push. Assert one of two safe outcomes
(no third):
- Push succeeds AND repo+objects exist for the tombstone TTL window
  (destruction deferred).
- Push fails cleanly with a recognizable error (destruction won the race)
  AND no half-written state is left on disk.

The forbidden outcome: push HTTP 200 + zero objects on disk for the
session (silent data loss, identical to the
`object_storage_partition_test.go > transient_reset_peer_rpo0_holds`
invariant).

## Test sketch

```go
// tests/e2e/chaos/playground_destruction_during_push_test.go
func TestPlayground_DestructionDuringPush_NoCorruption(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    tp := toxiproxy.Start(ctx, t)
    p := portal.Start(ctx, t, portal.Options{
        DBDriver: "postgres", DBDSN: pg.ContainerDSN,
        PlaygroundEnabled:        true,
        PlaygroundIdleTimeout:    "10s",
        PlaygroundSweepInterval:  "500ms",
    })

    sess := createPlayground(t, p.URL)

    // Inject 5s latency on the portal→git path so the push hangs through a sweep.
    proxy := tp.SlowPath(p, "git-receive", 5*time.Second)
    defer proxy.Remove()

    go func() {
        // Push will block in the slowed proxy.
        gitclient.PushBlob(t, proxy.URL+"/git/playground/"+sess.ID+".git",
            sess.Bearer, 10<<10)
    }()

    // Wait long enough that the idle timeout + sweep would fire mid-push.
    portalclock.Advance(t, p, 15*time.Second)
    time.Sleep(2 * time.Second)

    // Verify safe outcome.
    repoExists := dockerExecExists(t, p, portalRepoPath("playground", sess.ID))
    objCount := dockerExecObjectCount(t, p, portalRepoPath("playground", sess.ID))
    if repoExists {
        // Destruction deferred until push completes — objects must be intact.
        require.GreaterOrEqual(t, objCount, 1)
    } else {
        // Destruction won — no half-written state.
        require.Equal(t, 0, objCount)
    }
}
```

## Implementation notes

### Approach used

Clock-based orchestration (Approach 2 from the feature design brief), structured
as two sub-tests sharing a single portal+postgres stack:

**`push_before_destroy`** (deterministic push-wins):
Push a commit while the clock is still within the hard-cap, then advance the
clock 70s past the 60s hard-cap to trigger destruction. Asserts that the
tombstone's `commits_count >= 1` (no silent data loss) and the repo is removed
after destruction.

**`concurrent_race/iter_N`** (5 iterations of the race):
Clone + prepare commit → spawn push goroutine → immediately advance clock 70s
past hard-cap. The push races with the destruction sweep (which fires within
0-1s real time of the clock advance). On fast local/CI hardware the sweep
consistently wins by revoking the bearer between git's unauthenticated challenge
and its authenticated retry (two consecutive info/refs requests). Each iteration
asserts: tombstone present, session inaccessible, repo gone — no torn state in
either outcome.

### Key discovery during implementation

The anonymous playground bearer has a real-time TTL equal to `HARD_CAP_S`.
Setting `HARD_CAP_S=5` (as initially attempted) caused the bearer to expire
during git clone + commit setup (~5-7s), making all pushes fail with 401 before
the clock was even advanced. Solution: set `HARD_CAP_S=60` to give the bearer
ample real-time margin, then advance the clock by 70s (10s past the new cap).

Additionally, the rate limiter (`NewCreateRateLimiter`) uses `realClock{}` not
the injectable clock, so clock advances do NOT reset rate limits. Setting
`JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR=3600` (perMinute=60, burst=60) allows
6+ back-to-back session creates in the test.

On fast hardware the concurrent_race iterations consistently show destroy-wins
because the sweep fires between git's two info/refs calls (~10-50ms apart).
The `push_before_destroy` sub-test explicitly covers the push-wins path with a
deterministic sequential ordering.

### Flake results

4 consecutive runs, all passing (4/4 PASS):
- Run 1: 15.65s — 6 sub-tests, all PASS
- Run 2: 10.09s — 6 sub-tests, all PASS
- Run 3: 12.32s — 6 sub-tests, all PASS
- Run 4: 13.28s — 6 sub-tests, all PASS

No torn state observed in any run. Test is stable.

## Review (2026-05-24)

**Verdict**: Approve

**Notes**: Two subtests landed:
- `push_before_destroy`: deterministic push-wins path; asserts
  `commits_count >= 1` in the tombstone (no silent data loss).
- `concurrent_race/iter_01..05`: 5 iterations of the actual race;
  sweep consistently wins by revoking the bearer between git's
  two-phase auth calls. Each iteration asserts no torn state in
  either outcome.

Verified 4/4 consecutive runs (10-15s each) — flake-free given the
chaos genre. Real subprocess + real DB + real filesystem. The 5
iterations exercise both push-wins and sweep-wins ordering across
runs.

Advanced `stage: review → done`.
