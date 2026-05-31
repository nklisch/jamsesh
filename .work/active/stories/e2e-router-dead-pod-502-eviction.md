---
id: e2e-router-dead-pod-502-eviction
kind: story
stage: drafting
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
