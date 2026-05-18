---
id: gate-cruft-router-kube-discovery-wired-or-deleted
kind: story
stage: implementing
tags: [cleanup, infra]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# Router advertises Kubernetes discovery mode but never wires it up

## Confidence
High

## Category
dead function / unfulfilled advertised feature

## Location
`internal/router/discovery/discovery.go:56` and
`cmd/jamsesh-router/main.go:121-138`

## Evidence
```go
// discovery.go
func Kubernetes(namespace, serviceName string, probe *readyz.Probe, interval time.Duration) Discoverer { ... }

// main.go: only the "static" branch exists
if cfg.DiscoveryMode == "static" {
    // ...
    disc := discovery.Static(cfg.StaticPods, probe, cfg.ProbeInterval)
}
// (no else branch; kubernetes mode silently falls through)
```

## Removal

**Autopilot decision (2026-05-18): delete.** The k8s-discovery wiring is
tracked at backlog story `epic-e2e-cnd-coverage-routing-layer-k8s-discovery`
(deferred from v0.1.0). Until that story is scoped active, the unused
discoverer + help-text are pure cruft.

Remove:
- The `Kubernetes` / `KubernetesWithClient` constructors and the
  `k8sDiscoverer` cluster in `internal/router/discovery/k8s.go` (and any
  related test files for that file).
- The help-text mentioning `JAMSESH_ROUTER_DISCOVERY_MODE="kubernetes"`,
  `JAMSESH_ROUTER_KUBE_NAMESPACE`, and `JAMSESH_ROUTER_KUBE_SERVICE_NAME`
  in `cmd/jamsesh-router/main.go:216-219`.
- The `DiscoveryMode`-routing logic that gates on the kubernetes branch
  (since there's now only one branch — static).
- The corresponding env-var declarations in `cmd/jamsesh-router/config.go`
  (or wherever the router config lives) for the kubernetes-only knobs.

The ARCHITECTURE.md doc-side drift was already addressed in a sibling
story (`gate-docs-arch-k8s-discovery`).
