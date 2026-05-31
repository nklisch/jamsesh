---
id: e2e-cloud-native-multipod-suite-red-router-redispatch
kind: feature
stage: drafting
tags: [portal, infra, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Router redispatch and metrics

## Brief
Two router defects keep failure/golden router tests red:

1. **Transparent redispatch on 503** — `router_lease_unavailable_test.go`
   (transparent_redispatch_on_503, bounded retry → 503) does not redispatch to a
   healthy backend / exhaust the bounded retry correctly when a backend reports
   lease-unavailable.
2. **Metric counters** — `router_consistent_hash_test.go` asserts a metric
   counter and gets "-1 not >= 0", and `router_hint_cache_test.go` checks a
   routing-decisions counter that is wrong.

This feature roots-causes and fixes the redispatch path and the router metric
counters across `internal/portal/router/`, `internal/router/`, and
`cmd/jamsesh-router`.

It does NOT cover object-storage sync, lease migration, or the scaffolding clone
gate. Note the golden suite's prometheus parse panic was already fixed in
`ed32b562`; the remaining router failures are real counter/redispatch defects.
Per the parent epic's design decisions this is never-green stabilization —
root-cause forward, no bisect.

## Epic context
- Parent epic: `e2e-cloud-native-multipod-suite-red`
- Position in epic: independent subsystem fix — parallel with objectstore,
  lease, and fuzz. The cluster-smoke integration gate depends on this feature.

## Foundation references
- `docs/ARCHITECTURE.md` — router (consistent-hash ring, hint cache) component
- Primary packages: `internal/portal/router/`, `internal/router/`, `cmd/jamsesh-router`
- Representative red tests (feature-design confirms the exact owned set):
  failure `router_lease_unavailable_test.go`, `router_backend_dead_test.go`;
  golden `router_consistent_hash_test.go`, `router_hint_cache_test.go`
