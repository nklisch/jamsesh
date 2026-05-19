---
id: epic-e2e-cnd-coverage-lease-fencing-chaos
kind: story
stage: done
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-lease-fencing
depends_on: [epic-e2e-cnd-coverage-lease-fencing-golden]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E CND Lease Fencing — Chaos

## Scope

Implement two chaos tests:
- `tests/e2e/chaos/lease_holder_killed_test.go` (F13)
- `tests/e2e/chaos/cross_pod_clock_skew_test.go` (F14)

These verify the graceful-degradation and correctness invariants of the
lease system under pod-kill and cross-pod clock-skew failure injection.

## Implementation units

### Unit 1: `tests/e2e/chaos/lease_holder_killed_test.go`

**Invariant:** when the pod holding a session lease is killed (SIGKILL),
the Postgres advisory lock auto-releases (connection drop triggers PG cleanup),
a second pod acquires the lease with a strictly higher fencing token, and any
subsequent writes from the killed pod (with its stale token) are rejected.

The lease-migration half of this test overlaps in spirit with
`epic-e2e-cnd-coverage-hydration-handoff`. Design boundary: this test owns
the lease-ownership invariants (lock release, monotonic token, stale-write
rejection). The handoff feature owns the hydration and client-continuity
invariants. Do NOT duplicate hydration assertions here.

**Stack:** 2-pod cluster with router. `PortalExtraEnv` sets
`JAMSESH_LEASE_HEARTBEAT_INTERVAL_S=2` for fast lease settling.

**SLO:** lease must migrate within 30s of pod kill (3× heartbeat interval
as a conservative bound). Implement as a timeout parameter passed to
`c.WaitForLeaseMigration(ctx, t, sessionID, 0 /*pod 0*/, 30*time.Second)`.

**Chaos mechanism:** `c.Kill(0)` — uses `docker kill --signal SIGKILL
<container-name>`. No Pumba needed (the Kill helper in `lifecycle.go`
already implements this pattern).

**Defensive cleanup:** register `c.Kill` failure recovery in `t.Cleanup`
as done in `testAutomergerPause`. The container may already be dead — ignore
errors in cleanup.

```go
package chaos_test

func TestLeaseHolderKilled(t *testing.T) {
    ctx := context.Background()

    pg := postgres.Start(ctx, t, postgres.Options{})
    mn := minio.Start(ctx, t, minio.Options{})
    mh := mailhog.Start(ctx, t)
    c := portalcluster.Start(ctx, t, portalcluster.Options{
        Pods: 2, Postgres: pg, ObjectStore: mn, Router: true,
        PortalExtraEnv: map[string]string{
            "JAMSESH_LEASE_HEARTBEAT_INTERVAL_S": "2",
        },
    })

    alice := authflow.SignInViaMagicLink(ctx, t, c.Pods[0], mh, leaseChaosEmail(t, "alice-kill"))
    orgID  := authflow.CreateOrg(ctx, t, c.Pods[0], alice.AccessToken, "Lease Chaos Org")
    sessionID := createLeaseChaosSession(ctx, t, c.Pods[0], alice.AccessToken, orgID, "lease-kill-chaos")

    // --- BEFORE-CHAOS BASELINE ---
    // Push via router. Confirm pod 0 holds the lease and T1 > 0.
    pushViaRouter(ctx, t, c.RouterURL, orgID, sessionID, alice)
    holder0 := c.RequireLeaseHolder(ctx, t, sessionID, 10*time.Second)
    if holder0 != 0 {
        t.Fatalf("expected pod 0 to hold lease before chaos, got pod %d", holder0)
    }
    t1 := c.FencingTokenForSession(ctx, t, sessionID)
    if t1 <= 0 {
        t.Fatalf("baseline fencing token %d <= 0", t1)
    }

    // --- CHAOS: kill pod 0 ---
    c.Kill(ctx, t, 0)

    // --- ASSERT: lease migrates to pod 1 within SLO ---
    holder1 := c.WaitForLeaseMigration(ctx, t, sessionID, 0 /*from pod 0*/, 30*time.Second)
    if holder1 < 0 {
        // Lease did not migrate within the SLO. This is a critical correctness
        // failure (the session is now without a holder, rejecting all writes).
        // It may be a real bug or an SLO miscalibration.
        // If it's consistently reproducible: park the bug via /agile-workflow:park.
        t.Fatalf("lease_holder_killed: lease did not migrate within 30s SLO after pod kill")
    }

    // --- ASSERT: monotonic token (T2 > T1) ---
    t2 := c.FencingTokenForSession(ctx, t, sessionID)
    if t2 <= t1 {
        t.Fatalf("fencing token not monotonic: T2=%d <= T1=%d after pod re-acquisition", t2, t1)
    }

    // --- ASSERT: push via router succeeds on pod 1 (system recovered) ---
    pushViaRouter(ctx, t, c.RouterURL, orgID, sessionID, alice)
}
```

**Notes on the "stale-pod-A writes rejected" assertion:** pod A is dead
after `Kill(0)`, so it cannot issue writes. This assertion is covered by
`stale_fencing_token_rejected_test.go`. Do not duplicate it here — keep this
test focused on the migration and monotonicity invariants.

---

### Unit 2: `tests/e2e/chaos/cross_pod_clock_skew_test.go`

**Invariant:** when pod A's system clock is skewed forward (simulating clock
drift under NTP failure), the lease system must remain consistent. The lock
is advisory and connection-scoped — the heartbeat uses `time.NewTicker` which
relies on the local clock. This test probes whether clock skew causes the
heartbeat ticker to fire at the wrong rate, potentially triggering spurious
lease loss detection.

**Expected behavior (from ARCHITECTURE.md + lease implementation):**
- Advisory locks are scoped to the Postgres connection, not to a wall-clock
  TTL. The lock is lost only when the connection drops.
- The heartbeat goroutine (`runHeartbeat`) pings the dedicated connection on
  each tick. If the ticker fires more rapidly under clock skew, more pings
  occur but the lock is not released — this is benign.
- If the ticker's `PingContext` timeout (`timeout = interval`) is shortened
  by clock skew, it may expire before the ping returns, causing spurious
  heartbeat failure. This IS the bug path.

**Clock-skew injection:** libfaketime via `LD_PRELOAD` at the container
level, following the existing pattern from `testClockSkewTokenExpiry`
(which uses `p.AdvanceClock`). The cluster fixture's `PortalExtraEnv` can
set the libfaketime offset for pod A only.

**WARNING — this test may surface a real bug.** If the portal's
`runHeartbeat` uses `time.NewTicker` with the same duration as the
`PingContext` timeout, a clock skew that accelerates the ticker will cause
the timeout to expire before the ping, closing `Lost()` and causing spurious
lease eviction. This is a real split-brain risk under NTP clock jump.

If this test fails consistently:
1. Park the bug via `/agile-workflow:park` with title "Clock skew
   accelerates heartbeat ticker causing spurious lease loss".
2. Land the test with `t.Skip("<backlog-id>: clock skew causes heartbeat
   timeout under local-clock-anchored ticker")`.
3. The skipped test is the audit trail. Do NOT change the assertion.

```go
func TestCrossPodClockSkew(t *testing.T) {
    if testing.Short() {
        t.Skip("chaos: long-running, skip under -short")
    }
    ctx := context.Background()

    // Start 2-pod cluster where pod 0's clock is advanced 2× the heartbeat
    // interval. This simulates a clock jump that would cause a local-clock
    // anchored ticker to fire at double rate.
    const heartbeatS = 2
    const skewSeconds = heartbeatS * 4 // skew to trigger accelerated ticks

    pg := postgres.Start(ctx, t, postgres.Options{})
    mn := minio.Start(ctx, t, minio.Options{})
    mh := mailhog.Start(ctx, t)

    c := portalcluster.Start(ctx, t, portalcluster.Options{
        Pods: 2, Postgres: pg, ObjectStore: mn, Router: true,
        PortalExtraEnv: map[string]string{
            "JAMSESH_LEASE_HEARTBEAT_INTERVAL_S": fmt.Sprintf("%d", heartbeatS),
        },
    })

    // Establish sessions on both pods, confirm distinct lease holders.
    alice := authflow.SignInViaMagicLink(ctx, t, c.Pods[0], mh, leaseChaosEmail(t, "alice-skew"))
    orgID    := authflow.CreateOrg(ctx, t, c.Pods[0], alice.AccessToken, "ClockSkew Org")
    sessionA := createLeaseChaosSession(ctx, t, c.Pods[0], alice.AccessToken, orgID, "skew-session-a")
    sessionB := createLeaseChaosSession(ctx, t, c.Pods[0], alice.AccessToken, orgID, "skew-session-b")

    pushViaRouter(ctx, t, c.RouterURL, orgID, sessionA, alice)
    pushViaRouter(ctx, t, c.RouterURL, orgID, sessionB, alice)

    holderA := c.RequireLeaseHolder(ctx, t, sessionA, 10*time.Second)
    holderB := c.RequireLeaseHolder(ctx, t, sessionB, 10*time.Second)

    // --- BEFORE-CHAOS BASELINE ---
    // Both sessions have holders. Confirm at least one uses pod 0.
    t.Logf("baseline: sessionA held by pod %d, sessionB held by pod %d", holderA, holderB)

    // --- INJECT CLOCK SKEW on pod 0 ---
    // Advance pod 0's clock by skewSeconds.
    c.Pods[0].AdvanceClock(ctx, t, time.Duration(skewSeconds)*time.Second)

    // Wait skewSeconds + 2 heartbeats to see if the skew destabilizes the lease.
    time.Sleep(time.Duration(skewSeconds+heartbeatS*2) * time.Second)

    // --- ASSERT: leases are still held (no spurious loss) ---
    // If pod 0's lease was lost due to accelerated ticker timeout, RequireLeaseHolder
    // will find the session on a different pod (pod 1 re-acquired) or return -1.
    // Either case means the clock skew caused a problem.
    newHolderA := c.LeaseHolder(ctx, t, sessionA)
    newHolderB := c.LeaseHolder(ctx, t, sessionB)

    // If any session lost its holder entirely (-1), that's a definite bug path.
    if newHolderA < 0 || newHolderB < 0 {
        // Clock skew caused lease loss (advisory lock dropped due to heartbeat
        // timeout under accelerated ticker). Park the bug.
        //
        // TO IMPLEMENTER: if this path fires consistently, do NOT change the
        // assertion. Park the bug:
        //   /agile-workflow:park "Clock skew causes heartbeat timeout —
        //   PingContext deadline equals ticker interval, local clock accelerates"
        // Then: t.Skip("<backlog-id>: clock-skew causes spurious lease loss")
        t.Fatalf("cross_pod_clock_skew: clock skew caused lease loss "+
            "(sessionA holder=%d, sessionB holder=%d); this is a real bug — park it",
            newHolderA, newHolderB)
    }

    // If leases are still held: the implementation is robust to clock skew.
    t.Logf("cross_pod_clock_skew: leases stable after %ds clock skew "+
        "(sessionA pod=%d, sessionB pod=%d)", skewSeconds, newHolderA, newHolderB)
}
```

## Acceptance criteria

- [ ] `TestLeaseHolderKilled`: lease migrates within 30s SLO, T2 > T1,
      system recovers (push succeeds after migration).
- [ ] `TestCrossPodClockSkew`: either green (implementation is clock-robust)
      or `t.Skip` with a backlog-id reference (clock skew surfaces a bug).
      Must NOT be silently deleted or assertion-gamed to pass.
- [ ] Defensive `t.Cleanup` registrations for container-kill scenarios.
- [ ] No in-process mocks; real Postgres advisory locks, real MinIO.

## Test integrity

**This is the most safety-critical test file in the lease-fencing suite.**

**Park production bugs, don't hide them.**
- `TestLeaseHolderKilled`: if T2 <= T1, the monotonic invariant is broken.
  Park immediately via `/agile-workflow:park` and `t.Fatal`. Do not widen
  the assertion to `T2 >= T1`.
- `TestCrossPodClockSkew`: if the implementation uses local clocks for
  heartbeat timeouts, this test WILL fail. That failure is a security
  finding — clock skew can cause split-brain. Park it with the exact
  mechanism in the park item body, then `t.Skip` with the backlog id.

**Fix bad tests in-session.** The `AdvanceClock` method exists on
`*portal.Portal` (from `tests/e2e/fixtures/portal/clockadvance.go`). If
it is not hoisted to the cluster fixture's per-pod accessor, add a thin
wrapper — that is test infrastructure debt, not a production bug.

**Never game an assertion.** The monotonic token check (`T2 > T1`) and the
lease-present check (`holder >= 0`) must not be weakened. A weakened
assertion produces a green test that lies about the system's safety.

## Implementation notes (2026-05-17)

Both tests implemented and verified with `go build ./chaos/... && go vet ./chaos/...`.

### `lease_holder_killed_test.go`

- 2-pod cluster + router, `JAMSESH_LEASE_HEARTBEAT_INTERVAL_S=2`.
- Auth via pod 0 directly; pushes via router to trigger lease acquisition.
- `c.RequireLeaseHolder` confirms the holder before kill, capturing T1.
- `c.Kill(holderPod)` SIGKILLs the lease-holding pod (whichever pod the
  router's consistent-hash chose — not assumed to be pod 0).
- `c.WaitForLeaseMigration` asserts migration within 30s SLO.
- Second push to the surviving pod's direct URL triggers re-acquisition.
- `c.FencingTokenForSession` captures T2; asserts T2 > T1 with a
  diagnostic Fatal that names the Critical bug if violated.
- Final push via router asserts system recovery.
- Defensive `t.Cleanup` best-effort-kills the container (ignores errors
  on already-dead container).

### `cross_pod_clock_skew_test.go`

- 2-pod cluster + router, 2s heartbeat, skewSeconds = 8 (4× heartbeat).
- Establishes leases on two sessions before injecting skew.
- Calls `c.Pods[0].AdvanceClock(ctx, t, 8s)` — POSTs to
  `/test/clock-advance` on pod 0 (requires `-tags e2etest` build).
- Waits `skewSeconds + 2×heartbeat` real wall-clock seconds (not affected
  by the portal-internal clock advance).
- `c.LeaseHolder` checks both sessions. If either returns -1, the test
  fatals with a Critical diagnostic message and park instructions.
- If both holders are >= 0: logs success (implementation is clock-robust).
- Includes detailed escape-hatch comment for the park+skip pattern if the
  clock-dependence bug fires consistently.

### Design decisions

- **Holder pod not pinned to index 0.** The router's consistent-hash
  chooses the holder. `RequireLeaseHolder` returns the actual holder so
  `Kill` and `WaitForLeaseMigration` always target the correct pod.
- **`leaseChaosEmail` defined once in `lease_holder_killed_test.go`.**
  Both test files are in `package chaos_test` so helpers in one file are
  visible to the other. `leaseChaosEmail` wraps `randEmail` (from
  `network_and_provider_test.go`). `cross_pod_clock_skew_test.go` calls
  it for consistent naming.
- **No `ReleaseLeaseForcibly` in the kill test.** The kill test focuses on
  the advisory-lock auto-release invariant, not re-acquisition from a
  clean state. `ReleaseLeaseForcibly` is used in the golden monotonic test
  where a deliberate clean-state re-acquisition is needed. The kill test
  triggers re-acquisition by pushing directly to the surviving pod.
- **`testing.Short()` guard on the skew test.** The wait period
  (`skewSeconds + 2×heartbeat` = ~12s) plus cluster startup makes this
  test slow. `-short` skips it for fast local iteration.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: Clock-skew test uses `time.Sleep(waitDur)` in test process — acceptable here since this is measuring real wall-clock effects, not a polling loop that could use Monitor.

**Notes**: `TestLeaseHolderKilled` correctly targets the actual holder pod (via `RequireLeaseHolder`) rather than assuming pod 0 wins, which is a concrete improvement over the story design sketch. Monotonicity assertion is `t2 <= t1` (strict). Defensive `t.Cleanup` for best-effort container kill is present. `TestCrossPodClockSkew` includes detailed, actionable escape-hatch instructions (park + t.Skip with backlog-id) directly in the source. The clock-skew assertion correctly uses `newHolderA < 0 || newHolderB < 0` to catch either session losing its holder. No t.Skip calls without documented reasons. The `testing.Short()` guard is appropriate for the ~12s wall-clock wait. No assertion gamification found.
