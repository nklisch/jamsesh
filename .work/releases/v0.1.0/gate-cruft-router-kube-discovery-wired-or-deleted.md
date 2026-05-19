---
id: gate-cruft-router-kube-discovery-wired-or-deleted
kind: story
stage: done
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

## Implementation notes

### Deleted files
- `internal/router/discovery/k8s.go` — `k8sDiscoverer` type, `Kubernetes` and `KubernetesWithClient` constructors, informer loop, `listRunningAddrs`, `k8sPodsToPods`
- `internal/router/discovery/k8s_test.go` — 4 tests exercising the k8s discoverer via a fake clientset

### Simplified / edited files
- `internal/router/discovery/discovery.go` — removed `Kubernetes` constructor function and updated package doc to drop the k8s mode bullet; updated `Discoverer.Run` comment to drop k8s-informer wording
- `cmd/jamsesh-router/main.go` — removed `if cfg.DiscoveryMode == "static"` guard (static is now unconditional); removed `discovery_mode` from the startup log attrs; removed kube env-var lines from `printUsage`
- `internal/router/config/config.go` — removed `DiscoveryMode`, `KubeNamespace`, `KubeServiceName` struct fields; removed their env-var parsing in `applyEnv`; simplified `Validate` from a `switch` on DiscoveryMode to a direct `len(StaticPods)==0` check; updated package doc
- `internal/router/config/config_test.go` — removed `TestEnvKubeFields`; removed `"unknown discovery mode"` test case from `TestValidate`; updated `TestLoadYAML` YAML fixture and assertions to drop `DiscoveryMode`; updated `TestValidate` base config and `TestEnvOverlay` fixture

### go.mod change
Removed 3 direct dependencies and ~13 indirect dependencies. Direct removals:
- `k8s.io/api v0.36.1`
- `k8s.io/apimachinery v0.36.1`
- `k8s.io/client-go v0.36.1`

### No e2e test impact
No files under `tests/e2e/` imported k8s discovery symbols. The two k8s mentions in `tests/e2e/` are incidental comments about Kubernetes deployment patterns, not related to the discovery package.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Mechanical change matching the gate finding spec. Implementation notes accurately describe what was changed. Global `go build ./...` and `go test ./internal/portal/...` pass after the wave landed.
