---
id: gate-docs-arch-k8s-discovery
kind: story
stage: review
tags: [documentation, infra]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: docs
created: 2026-05-18
updated: 2026-05-18
---

# ARCHITECTURE.md §14 claims pluggable Kubernetes discovery is wired in the router; only static discovery is started

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/ARCHITECTURE.md:420-426`
- Code: `cmd/jamsesh-router/main.go:121-143` — `if cfg.DiscoveryMode ==
  "static"` branches start the discovery loop, but no corresponding
  branch starts the kubernetes informer; `internal/router/discovery/k8s.go`
  exists but is not wired.

## Current doc text
> Pod discovery is pluggable:
> - **Static mode** (`JAMSESH_ROUTER_DISCOVERY_MODE=static`) — a fixed list …
> - **Kubernetes mode** (`JAMSESH_ROUTER_DISCOVERY_MODE=kubernetes`) —
>   watches the pod IPs backing a named Kubernetes Service via client-go
>   informers, probes each Running pod's `/readyz`, and publishes only the
>   healthy subset to the ring.

## Reality
The router binary supports only `static` discovery at runtime. `--help`
output advertises a `kubernetes` mode and the k8s discoverer package
exists, but `main.go` never instantiates it. Setting
`JAMSESH_ROUTER_DISCOVERY_MODE=kubernetes` results in an empty ring.

## Required edit
Either (a) describe only static discovery in ARCHITECTURE.md §14 and
SELF_HOST.md §14 until the kubernetes branch is wired in `main.go`, or
(b) hold the edit until the wiring is shipped before v0.1.0 cuts.

Note: overlaps `gate-cruft-router-kube-discovery-wired-or-deleted` and
the in-flight `epic-e2e-cnd-coverage-routing-layer-k8s-discovery`. If
that story lands the wiring, this doc-drift item closes by being
already-correct.

## Implementation notes

Confirmed `cmd/jamsesh-router/main.go:121-143` has no kubernetes branch — only
`if cfg.DiscoveryMode == "static"` is wired. `internal/router/discovery/k8s.go`
exists but is not instantiated.

Edited `docs/ARCHITECTURE.md` §14: replaced the "Pod discovery is pluggable"
lead-in and both mode bullets with a single paragraph describing only the static
discovery that is actually wired. Dropped `JAMSESH_ROUTER_DISCOVERY_MODE` and
`JAMSESH_ROUTER_KUBE_*` env var references from the section. No other sections
touched.
