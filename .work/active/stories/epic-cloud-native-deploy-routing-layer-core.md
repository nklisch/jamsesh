---
id: epic-cloud-native-deploy-routing-layer-core
kind: story
stage: review
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

## Implementation notes

Landed four files:

**`internal/router/extract/extract.go`** — pure session-ID extraction with no
allocations for the fast path. Routes handled:
- REST: `/api/orgs/{orgID}/sessions/{sessionID}/...` (uses the actual portal
  URL shape from `internal/portal/githttp/handler.go` — `/git/{orgID}/{sessionID}.git/...`
  rather than the `/git/sessions/...` placeholder in the design doc).
- Git smart-HTTP: `/git/{orgID}/{sessionID}.git/...` (orgID segment is present
  in the actual URL shape; the design doc's shape was simplified).
- WS: `/ws/sessions/{sessionID}`.
- MCP: `Jam-Session-Id` header (checked last, after path-based extraction).
- System routes (`/healthz`, `/readyz`, `/metrics`, `/auth/*`) return `""`.
- Trailing-slash normalisation applied before switch — no regex.

**`internal/router/extract/extract_test.go`** — 30+ subtests covering every
route variant, edge cases (trailing slash, empty segment, missing orgID), and
header-vs-path precedence.

**`internal/router/ring/ring.go`** — consistent-hash ring with:
- `atomic.Pointer[ringSnapshot]` for lock-free `Get`; `SetPods` builds a fresh
  snapshot and swaps atomically (copy-on-write).
- Vnode hash: deterministic `fnv64a("{podID}:{index}")` — same pod set always
  produces the same assignment.
- Key hash: `fnv64a(key)` → binary search on sorted vnode slice.
- Empty ring → zero `Pod` (no panic, no special-case needed by callers).

**`internal/router/ring/ring_test.go`** — covers empty ring, single-pod,
deterministic routing, atomic swap, consistent-hash invariant (remove/add one
pod from 5-pod ring moves ≤ 30% of keys — verified at 17% and 8%
respectively), concurrent `Get`+`SetPods` under `-race`, and distribution
sanity (every pod ≥ 2%, no pod > 55% — FNV at 150 vnodes has high variance
so tight uniform bounds are not asserted; the consistent-hash invariant is
the correct property to enforce).

`go test -race ./internal/router/extract/... ./internal/router/ring/...` clean.
Full `go test ./...` green with no regressions.
