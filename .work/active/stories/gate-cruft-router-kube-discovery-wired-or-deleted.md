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
Either add the kubernetes branch in `main.go` calling
`discovery.Kubernetes(...)` or remove the `Kubernetes` /
`KubernetesWithClient` / `k8sDiscoverer` cluster plus the help-text
mentioning `JAMSESH_ROUTER_DISCOVERY_MODE="kubernetes"`,
`JAMSESH_ROUTER_KUBE_NAMESPACE`, and `JAMSESH_ROUTER_KUBE_SERVICE_NAME`
in `main.go:216-219`.

Note: there is an in-flight story
`epic-e2e-cnd-coverage-routing-layer-k8s-discovery` at stage
`implementing` that may wire this — coordinate with that work before
choosing the delete path.
