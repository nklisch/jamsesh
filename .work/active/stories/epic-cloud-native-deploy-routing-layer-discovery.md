---
id: epic-cloud-native-deploy-routing-layer-discovery
kind: story
stage: done
tags: [infra]
parent: epic-cloud-native-deploy-routing-layer
depends_on: [epic-cloud-native-deploy-routing-layer-core]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — Pod discovery (static + k8s) with /readyz probe

## Scope

`Discoverer` interface + two implementations:
1. Static — polls configured addresses, probes `/readyz`, publishes
   healthy subset.
2. Kubernetes — watches pods backing a service via `k8s.io/client-go`
   informer, probes `/readyz`, publishes healthy subset.

Plus a `readyz.Probe` helper for parallel readiness checks.

Implements **Unit 3** of `epic-cloud-native-deploy-routing-layer`.

## Files

New:
- `internal/router/discovery/discovery.go`
- `internal/router/discovery/static.go`
- `internal/router/discovery/k8s.go`
- `internal/router/discovery/static_test.go`
- `internal/router/discovery/k8s_test.go`
- `internal/router/readyz/probe.go`
- `internal/router/readyz/probe_test.go`

## Acceptance criteria

- [ ] Static Discoverer polls configured addrs, probes `/readyz`,
  publishes healthy subset on each `ProbeInterval`
- [ ] Static + healthy + unhealthy addr → publish contains only healthy
- [ ] Static + addr becomes healthy → published on next pass
- [ ] k8s Discoverer integrates with client-go informer (use the
  informer fake for tests)
- [ ] `Probe.Check` parallelizes; N addrs ≤ probe-timeout total
- [ ] `go test -race ./internal/router/discovery/...` clean

## Notes

- New dep: `k8s.io/client-go` (~5MB binary growth — acceptable for a
  deployment binary). Pull a recent stable version.
- For k8s informer testing use `k8s.io/client-go/informers` with the
  `fake.NewSimpleClientset()` from `k8s.io/client-go/kubernetes/fake`.
- Pod URLs constructed as `http://<addr>` (TLS terminated at ingress).
- Publish callback signature per parent feature design:
  `func([]ring.Pod)`. Discoverer never holds the ring directly.

## Implementation notes

- Added `k8s.io/client-go@v0.36.1` and `k8s.io/api@v0.36.1` to go.mod.
- `readyz.Probe.Check` runs all probe requests in parallel with goroutines
  and a `sync.WaitGroup`; total latency is bounded by the client timeout, not
  by `len(addrs)`.
- Change-detection uses a `neverPublished` sentinel (`"\x00"`) so the first
  probe pass always calls `publish` — even when the healthy set is empty —
  giving the ring a clean initial state.
- Static discoverer: Pod ID == address (`host:port`) — deterministic and
  stable for static config.
- k8s discoverer: uses `informers.NewSharedInformerFactoryWithOptions` with
  `WithNamespace`; Pod add/update/delete events notify a buffered channel
  (de-bounced to avoid thundering-herd on batch deploys); a ticker also
  triggers re-probe on the configured interval.
- `KubernetesWithClient` exported for test injection of a
  `fake.NewSimpleClientset()` without exposing internal fields.
- `go test -race ./internal/router/discovery/... ./internal/router/readyz/...`
  passes clean.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- k8s informer + ticker dual-trigger design is more complex than strictly necessary (the informer alone would suffice for membership changes; the ticker primarily catches transient probe failures that don't generate informer events). Reasonable defensiveness for v1; could simplify later.

**Notes**: Discoverer interface clean (`Run(ctx, publish func([]ring.Pod)) error`). Static uses ticker + change-detection with `neverPublished` sentinel (`"\x00"`) so first probe pass always publishes — this fixes the bug the parallel service agent caught. K8s uses `informers.NewSharedInformerFactoryWithOptions(WithNamespace(...))` with buffered-channel event debounce + parallel probe of pod IPs. `KubernetesWithClient` constructor allows test injection of `fake.NewSimpleClientset()` without exposing internal fields — good pattern.

Probe is straightforward: parallel goroutines + WaitGroup, default 2s timeout per pod, only 200 responses count as healthy. Pod URL always `http://<addr>` (TLS terminated at ingress).

New deps: `k8s.io/client-go@v0.36.1` and `k8s.io/api@v0.36.1`. ~5MB binary growth on the router; acceptable for a deployment binary. Transitive deps reasonable for the client-go stack.
