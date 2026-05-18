---
id: epic-e2e-cnd-coverage-routing-layer-chaos-pod-disappears
kind: story
stage: implementing
tags: [e2e-test, testing, portal, infra]
parent: epic-e2e-cnd-coverage-routing-layer
depends_on: [epic-e2e-cnd-coverage-routing-layer-golden-consistent-hash]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — Chaos: Pod Disappears (Toxiproxy Network Partition)

## Scope

Implement `tests/e2e/chaos/router_pod_disappears_test.go`.

Two subtests:

### 1. `network_disconnect_mid_request`

**Invariant**: A Toxiproxy connection-level disconnect between the router and
one backend pod triggers the router's `ErrorHandler` (502 Bad Gateway from
`httputil.ReverseProxy`). The client receives a clean 502 or a 2xx from a
retry — NOT a hang.

**Design note on disconnect vs 503 retry path**: A network disconnect is
handled by `httputil.ReverseProxy.ErrorHandler`, which writes 502 (Bad
Gateway) — this is a different code path from the 503 retry. The 503 retry
path (`proxy.go:ServeHTTP`) only triggers on a 503 status from the pod, not
on transport-level errors. So this test verifies failover via the 502 path,
not the 503 retry path. The client may see 502 (the router's error handler
fired) or 2xx (if the router is configured to retry on transport errors in a
future enhancement). As implemented today (proxy.go), transport errors
produce 502 — the test should assert 502 and treat this as the correct, clean
outcome (not a hang, not a silent data corruption).

Steps:
1. Start a 2-pod cluster with the router. Route a Toxiproxy proxy between the
   router and pod 0 on the Docker bridge network.
2. Establish a session on pod 0 (LeaseHolder confirms).
3. Inject Toxiproxy `reset_peer` toxic on the router→pod-0 proxy: new
   connections are immediately reset.
4. Send a request for the session via the router.
5. Assert: response arrives within 5s; status is 502 (transport error,
   ErrorHandler fired) OR 2xx (if router retried on another pod — acceptable).
   NOT a hang, NOT a timeout at the caller side.
6. Remove the toxic. Assert: subsequent requests for the session return 2xx.

### 2. `network_latency_causes_timeout_failover`

**Invariant**: Toxiproxy latency on the router→pod path causes the router to
fail over (timeout or 502) rather than hanging the client connection.

Steps:
1. Start a 2-pod cluster with the router. Toxiproxy proxy between router
   and pod 0.
2. Inject Toxiproxy `latency` toxic: 5000ms latency on all connections to
   pod 0. This is well above typical HTTP server timeouts.
3. Send a request for a session that hashes to pod 0.
4. Assert: response arrives within a bounded wall-clock time (≤15s). The
   router's `ReadHeaderTimeout: 10s` (set in main.go) gates upstream reads.
5. Assert: status is 502 or 2xx (same as subtest 1 — not a hang at the
   caller side).
6. Remove the toxic. Assert recovery (2xx).

## Toxiproxy topology note

The router fixture starts with portal pod container IPs as backends
(`fmt.Sprintf("%s:8443", ip)` in cluster.go). To interpose Toxiproxy, the test
must:
- Start Toxiproxy before the cluster.
- Create a Toxiproxy proxy: `upstream = podContainerIP:8443`, listen on
  `toxiproxy_container_ip:LISTEN_PORT`.
- Start the cluster without the built-in Router option; instead, start the
  router fixture manually with the Toxiproxy proxy address as the backend for
  pod 0 and the direct container IP for pod 1.

This means the test does NOT use `portalcluster.Start(opts: Router: true)` —
it manages the router fixture directly to interpose Toxiproxy between specific
pods and the router.

```go
tp   := toxiproxy.Start(ctx, t)
// Create proxy for pod 0:
// POST /proxies with name=pod0, listen=tp.ContainerIP:21000, upstream=pod0ContainerIP:8443
// Inject latency or reset_peer toxic via tp.AdminURL/proxies/pod0/toxics
rtr  := router.Start(ctx, t, router.Options{
    Backends: []string{
        fmt.Sprintf("%s:21000", tp.ContainerIP), // pod0 via toxiproxy
        fmt.Sprintf("%s:8443", pod1ContainerIP), // pod1 direct
    },
})
```

Use the existing `tests/e2e/fixtures/toxiproxy/toxics.go` helpers (if present)
or make direct HTTP calls to the Toxiproxy admin API.

## Invariant

> A Toxiproxy-induced network failure (disconnect or latency) between the
> router and a backend pod produces a clean 502 or retried 2xx within a
> bounded wall-clock time. The client never hangs indefinitely.

## Assertion targets

- Wall-clock time from request send to response < 15s in all subtests.
- Status code is 502 or 2xx (never a client-side timeout/hang).
- After toxic removal: LeaseHolder returns a valid pod index; requests return
  2xx within normal SLO.

## Acceptance criteria

- [ ] Subtest `network_disconnect_mid_request` passes; response < 5s, no hang.
- [ ] Subtest `network_latency_causes_timeout_failover` passes; response < 15s.
- [ ] Toxiproxy service-level mock used (not an in-process network fault).
- [ ] Router started manually (not via `portalcluster.Start(Router: true)`) to
      allow Toxiproxy interposition per-pod.
- [ ] Toxic injection and removal via Toxiproxy admin API.

## Test-integrity rules

- **Park production bugs, don't hide them.** If the router hangs (no response
  within 15s) even after Toxiproxy latency injection, this surfaces a missing
  timeout in the proxy layer — park the bug. Land the test with `t.Skip` and a
  reason.
- **Never game an assertion.** Do not replace the wall-clock bound with
  `time.Sleep(20s)` and a status check. The time bound IS the resilience
  assertion.
- **Fix bad tests in-session.** If the Toxiproxy admin API integration is
  broken (e.g., wrong content-type), fix it before calling the test flaky.
