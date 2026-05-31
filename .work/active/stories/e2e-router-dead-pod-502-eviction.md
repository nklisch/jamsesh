---
id: e2e-router-dead-pod-502-eviction
kind: story
stage: review
tags: [portal, infra, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-31
updated: 2026-05-31
---

# Router 502: dead pod not evicted from the ring after kill

## Brief
After a lease holder pod is SIGKILLed, a git clone routed through the router
(`<router>/git/...`) returns **502** because the router still routes to the
killed pod — the dead pod lingers in the consistent-hash ring. Surfaced by
`lease_holder_killed_test.go` (post-migration clone via the router URL); it is
also why the handoff tests deliberately assert directly against the survivor pod
to bypass the router.

## Context
- This is the long-standing `bug-router-static-discoverer-not-started`
  (released v0.1.0, `.work/releases/v0.1.0/`). The router agent in this epic
  found that `cmd/jamsesh-router/main.go` DOES start the static discoverer
  (`go disc.Run(...)`), and corrected a stale "not started" comment — yet the
  502 persists. So the residual cause is likely one of:
  - the static discoverer's eviction is too slow (probe interval) for the test's
    timing, or
  - a missing per-request failover timeout: the proxy dials the dead pod and
    hangs/502s instead of failing fast and re-sharding.
- `router_pod_disappears_test.go` references the same bug
  (`(or missing per-request timeout)`).

## Suspected area
`internal/router/discovery/static.go`, `internal/router/proxy/proxy.go`
(per-request dial timeout + failover), `cmd/jamsesh-router/main.go` (probe
interval wiring).

## Acceptance
A clone/request through the router after a pod is killed fails over to a live
pod (no 502) within a reasonable bound; `lease_holder_killed_test.go`'s
post-migration clone succeeds. Reproduce → root-cause → minimal fix → verify.
Classify product-vs-test (it may be partly a test-timing assumption) honestly.

## Root cause (confirmed — PRODUCT bug)
`internal/router/proxy/proxy.go` had a transparent-redispatch path that only
fired on an upstream **503 status code** (a live pod saying "lease held
elsewhere"). A SIGKILLed pod never returns a 503 — it refuses the TCP
connection, so `httputil.ReverseProxy` invoked its `ErrorHandler`, which wrote a
flat **502 Bad Gateway** straight into the buffered response. `ServeHTTP` only
inspected `first.status == 503` as the retry trigger, so a dial/transport error
was never treated as retryable: no hint invalidation, no failover. The client
got the 502 (`git clone` → `exit status 128`). The lagging static-discoverer
eviction was a red herring — per-request failover is the correct fix because
ring eviction is inherently delayed by the probe interval and cannot cover the
request in flight at kill time.

Secondary: `proxyTo` used `httputil.ReverseProxy`'s default transport with no
bounded dial timeout, so a black-holed pod (latency toxic / silent drop) could
hang the request past any SLO instead of failing fast into the failover path.

## Fix (`internal/router/proxy/proxy.go` only)
1. **Transport-error failover.** `bufferedResponse` gained a `transportErr`
   field. The `proxyTo` `ErrorHandler` now records the dial/connection error on
   the buffered response (via `setTransportErr`) instead of writing a 502.
   `ServeHTTP` treats a non-nil `transportErr` exactly like a 503: invalidate
   the hint, redispatch to a distinct pod (`Ring.GetNext`), present the retry's
   response. New `writeFirstFailure` helper synthesises a clean 502 only when
   failover is exhausted/unavailable and the buffer is empty; otherwise it
   flushes the buffered response (503 or a 2xx retry success). The existing
   bounded-retry (exactly one extra pod), body-replay cap, WebSocket/streaming
   commit semantics, and 503 behaviour are all preserved.
2. **Bounded dial timeout.** A shared `upstreamTransport` (`http.Transport`
   with a 3 s `DialContext`/`TLSHandshakeTimeout`) is now used by both the
   session handler and the round-robin fallback, so a dead/black-holed pod
   fails fast. `ResponseHeaderTimeout` is deliberately left unset so streaming
   git fetches / WebSocket upgrades are not cut at the transport layer.

No change to portal git/lease/objectstore code. **Product-vs-test: 100%
product.** No test-debt found. `lease_holder_killed_test.go`'s
"fall through to the survivor pod directly" workaround push stays (it triggers
`AcquireForRequest` by design), but the final router-routed recovery push now
genuinely succeeds via failover instead of depending on the router happening to
hash to a live pod.

## Verification
- New unit regression tests in `internal/router/proxy/proxy_test.go`:
  - `TestDeadPodTransportErrorFailsOver` — dead primary pod (connection
    refused) → client gets the live pod's 200, hint invalidated. FAILED before
    the fix (got 502), PASSES after.
  - `TestBothPodsDeadReturns502` — both pods dead → bounded failover → clean
    502, no hang.
- All `internal/router/...` unit tests green; `go vet` clean.
- Router image rebuilt via `make test-router-image` (portal image untouched).
- e2e `TestLeaseHolderKilled`: **PASS (21.8s)** — including
  "push via router after migration succeeded — system recovered ✓" (the 502 is
  gone), monotonic token T2(2) > T1(1).
- e2e `TestRouterPodDisappears`: **PASS (both subtests, not skipped)** —
  `network_disconnect_mid_request` now returns **200 in 4.3 ms** under the
  reset_peer toxic (failed over to the live pod, no 502);
  `network_latency_causes_timeout_failover` returns 200 within SLO. The
  previously-skipped per-request-timeout / discoverer path is now genuinely
  green.
