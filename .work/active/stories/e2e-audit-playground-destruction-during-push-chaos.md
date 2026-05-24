---
id: e2e-audit-playground-destruction-during-push-chaos
kind: story
stage: drafting
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
