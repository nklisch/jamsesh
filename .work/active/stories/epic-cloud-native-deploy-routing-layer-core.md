---
id: epic-cloud-native-deploy-routing-layer-core
kind: story
stage: implementing
tags: [infra]
parent: epic-cloud-native-deploy-routing-layer
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — Core (extract + ring)

## Scope

Two pure-algorithmic packages with no I/O:

1. `internal/router/extract` — `SessionID(*http.Request) string` covering
   REST / Git / WS path shapes and the `Jam-Session-Id` header for MCP.
   Returns `""` for `/healthz`, `/readyz`, `/metrics`, `/auth/*`.
2. `internal/router/ring` — consistent-hash ring with virtual nodes,
   atomic SetPods replacement via copy-on-write, concurrent-safe Get.

Implements **Unit 1** of `epic-cloud-native-deploy-routing-layer`. See
parent feature body for interfaces, route shapes, and acceptance criteria.

## Files

New:
- `internal/router/extract/extract.go`
- `internal/router/extract/extract_test.go`
- `internal/router/ring/ring.go`
- `internal/router/ring/ring_test.go`

## Acceptance criteria (mirror parent design)

- [ ] `extract.SessionID` returns session id for REST/Git/WS paths and
  MCP header
- [ ] Returns `""` for `/healthz`, `/readyz`, `/metrics`, `/auth/*`
- [ ] `ring.New(vnodes int)` builds a ring; `SetPods` replaces atomically
- [ ] Same key → same pod across calls
- [ ] Adding/removing 1 pod from a 5-pod ring re-routes ≤ 1/5 ± 10% keys
- [ ] Empty ring → `Get` returns zero `Pod`
- [ ] `go test -race ./internal/router/...` clean

## Notes

- Hash: stdlib `hash/fnv.New64a` (no external dep)
- Ring concurrency: `atomic.Pointer[ringSnapshot]` for lock-free Get
- Vnode count default 150
