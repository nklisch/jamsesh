---
id: epic-cloud-native-deploy-routing-layer-discovery
kind: story
stage: implementing
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
