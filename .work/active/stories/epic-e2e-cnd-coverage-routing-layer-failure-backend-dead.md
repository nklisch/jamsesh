---
id: epic-e2e-cnd-coverage-routing-layer-failure-backend-dead
kind: story
stage: implementing
tags: [e2e-test, testing, portal, infra]
parent: epic-e2e-cnd-coverage-routing-layer
depends_on: [epic-e2e-cnd-coverage-cluster-fixture]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — Failure: Backend Pod Dead

## Scope

Implement `tests/e2e/failure/router_backend_dead_test.go`.

One subtest: **`dead_pod_removed_from_routing_pool`**

**Invariant**: When a backend pod is killed (SIGKILL), new requests for
sessions that the ring would have routed to the dead pod are re-sharded to a
surviving pod within the router's health-check SLO.

Steps:
1. Start a 3-pod cluster with the router.
2. Establish a session on pod 0 (confirm via `cluster.LeaseHolder`). Send N
   successful requests; verify all route to pod 0.
3. Kill pod 0 via `cluster.Kill(ctx, t, 0)` (docker SIGKILL).
4. Wait for the router's static-mode health-check to detect the dead pod
   (the router probes backends on a configurable interval; default interval
   needs to be confirmed from config). Poll: send requests for the session and
   check that they start succeeding on a surviving pod. Use a polling timeout
   of 15s (generous but bounded; SLO for static-mode detection is determined
   by the health-check interval + 1-2 probe cycles).
5. Assert: after detection, requests for the same session return 2xx from a
   surviving pod (pod 1 or pod 2). `cluster.LeaseHolder` will return -1 for
   dead pod 0 (no lock holder on that pod); response from surviving pod is
   sufficient evidence.
6. Assert: the router does NOT continue routing to the dead pod after health-
   check eviction (no timeout/connection-reset errors propagating to the client
   after SLO).

**Discovery mode note**: The router fixture starts in static mode. The static
discoverer (`internal/router/discovery/static.go`) probes backends. Read the
probe interval from `config.go` defaults. If the interval is long (>10s), the
test must set a shorter interval via `PortalExtraEnv` or a router config
override. If the router doesn't expose a config env-var for probe interval,
file a follow-on story and use the test's natural timeout with a comment.

**SLO note**: The test's 15s polling window is an explicit SLO assertion — if
the router takes more than 15s to evict a dead pod, the test fails. This is
intentional: a router that takes 5 minutes to notice a dead backend is
operationally broken. If the test fails due to the probe interval being longer
than 15s by default, that is a design finding to park and address.

## Setup

```go
pg  := postgres.Start(ctx, t, postgres.Options{})
mn  := minio.Start(ctx, t, minio.Options{})
c   := portalcluster.Start(ctx, t, portalcluster.Options{
    Pods:        3,
    Postgres:    pg,
    ObjectStore: mn,
    Router:      true,
})
```

## Invariant

> After a pod is killed, the router detects its absence within the health-check
> SLO and re-shards affected sessions to surviving pods. Clients see at most a
> transient error window, not a permanent outage.

## Assertion targets

- All requests for the session after the SLO window return 2xx.
- No request returns a connection-reset or timeout error after the eviction
  window.
- `cluster.LeaseHolder` for the session returns a surviving pod index (>= 0
  and != 0) after the SLO window.

## Acceptance criteria

- [ ] Subtest `dead_pod_removed_from_routing_pool` passes within 15s SLO.
- [ ] Kill uses `cluster.Kill(ctx, t, podIndex)` (docker SIGKILL — NOT Pumba,
      which was rejected in the cluster-fixture design).
- [ ] No in-process portal or router mock.

## Test-integrity rules

- **Park production bugs, don't hide them.** If the router never evicts the
  dead pod (requests to dead pod continue to fail beyond 15s), park the bug.
  Land the test with `t.Skip` and a reason referencing the backlog item.
- **Fix bad tests in-session.** If the test is flaky due to probe interval
  variation, adjust the SLO window or add `t.Logf` tracing — never remove
  the SLO assertion.
- **Never game an assertion.** Do not swap the SLO check for `time.Sleep(20s)`
  followed by a single status check — the polling loop IS the SLO contract.
