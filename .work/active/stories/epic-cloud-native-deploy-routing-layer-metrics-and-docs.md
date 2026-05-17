---
id: epic-cloud-native-deploy-routing-layer-metrics-and-docs
kind: story
stage: implementing
tags: [infra, documentation]
parent: epic-cloud-native-deploy-routing-layer
depends_on: [epic-cloud-native-deploy-routing-layer-service, epic-cloud-native-deploy-routing-layer-discovery]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — Metrics + clustered-deploy docs

## Scope

Add routing-decision metrics to `internal/portal/metrics` (re-used cross-
binary by the router) and expose `/metrics` on the router service.
Author the clustered-mode deploy section in `docs/SELF_HOST.md` and the
Horizontal Scaling subsection in `docs/ARCHITECTURE.md`.

Implements **Unit 6** of `epic-cloud-native-deploy-routing-layer`.

## Files

Edit:
- `internal/portal/metrics/metrics.go` — add router metric handles
- `internal/router/proxy/proxy.go` — emit metrics on each routing
  decision
- `cmd/jamsesh-router/main.go` — wire up `/metrics` endpoint
- `docs/SELF_HOST.md` — new "Clustered mode (preview)" section
- `docs/ARCHITECTURE.md` — new "Horizontal scaling" subsection

## Acceptance criteria

- [ ] `jamsesh_router_decisions_total{result}` increments per request
  with `result` in {hit_cache, hit_ring, fallback, empty_ring, retry, 503}
- [ ] `jamsesh_router_ring_size` gauge reflects current pod count
- [ ] `jamsesh_router_ring_rebalances_total` counter increments on
  SetPods replacement
- [ ] `jamsesh_router_probe_failures_total{addr}` counter
- [ ] `/metrics` on the router exposes Prometheus text format
- [ ] SELF_HOST.md clustered-mode section is concrete and notes
  prerequisites (lease-fencing + object-storage-sync + hydration —
  not yet shipped at the time this lands, document as "preview")
- [ ] ARCHITECTURE.md has the Horizontal scaling subsection

## Notes

- Reusing `internal/portal/metrics` cross-binary is fine — it's a Go
  package. The router and portal both link it in.
- Cardinality safety: `addr` label is bounded by pod count (typically
  ≤ 20), `result` is a fixed enum. No unbounded labels.
- Docs section should be marked "preview" since the clustered mode is
  not yet fully shippable — other features in the epic
  (lease-fencing, object-storage-sync, hydration-handoff) must land
  for end-to-end clustered serving.
