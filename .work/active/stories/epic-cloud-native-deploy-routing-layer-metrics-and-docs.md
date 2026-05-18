---
id: epic-cloud-native-deploy-routing-layer-metrics-and-docs
kind: story
stage: done
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

## Implementation notes

### Metrics wiring

- Added four new metric handles to `internal/portal/metrics/metrics.go`:
  `RouterDecisionsTotal` (CounterVec, label `result`), `RouterRingSize`
  (Gauge), `RouterRingRebalancesTotal` (Counter), `RouterProbeFailuresTotal`
  (CounterVec, label `addr`). Registered unconditionally in `New()` — both
  binaries get all handles; portal just never increments router fields.

- `internal/router/proxy/proxy.go` — added `incDecision(result)` nil-safe
  helper. Emits `hit_cache`, `hit_ring`, `fallback`, `empty_ring`, `retry`,
  and `error_503` at the correct decision points in `ServeHTTP`.

- `internal/router/readyz/probe.go` — added `Metrics *metrics.Registry` field
  (nil-safe). Added `incProbeFailure(addr)` helper called on HTTP error, non-OK
  status, and connection failure.

- `cmd/jamsesh-router/main.go` — constructs `metrics.New()`, passes it to
  `proxy.Handler` and `readyz.Probe`. Wraps the static-mode seed path with
  direct ring-size + rebalance counter updates. Mounts `/metrics` on an
  `http.ServeMux` in front of the proxy handler so Prometheus scraping doesn't
  interfere with proxy traffic. The `publishWithMetrics` wrapper is ready for
  wiring to the discovery `Discoverer.Run` callback when kubernetes-mode
  discovery is wired in `main.go` (the discoverer itself stays pure per
  Option B).

### Test coverage added

- `internal/portal/metrics/metrics_test.go` — six new tests covering all four
  router metric families: label values, gauge mutation, scalar zero-emission,
  and the CounterVec lazy-emission semantics (vec metrics need a first
  observation to appear in output).
- `internal/router/readyz/probe_test.go` — two new tests:
  `TestProbeCheck_FailureCounterIncrements` (non-200 response increments
  counter for that addr, not for healthy addr) and
  `TestProbeCheck_UnreachableIncrementsCounter` (connection failure also
  increments).

### Docs

- `docs/SELF_HOST.md` §14 — "Clustered mode (preview)": when to use, Postgres
  prerequisite, object-storage caveat, architecture diagram, full k8s YAML
  (portal Deployment + Service + router Deployment + Service + RBAC), config
  knob table, metrics table, and explicit limitations section naming the three
  missing capabilities (fencing, object-storage-sync, hydration-handoff).
- `docs/ARCHITECTURE.md` — "Horizontal scaling (clustered mode)" section added
  before "Data layer". Covers: router as consistent-hash proxy, per-session
  Postgres advisory locks, fencing token design intent, object-storage-sync and
  hydration-handoff (to come). Framed accurately as preview — no "previously"
  prose, rolling-foundation principle respected.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Clean delivery. 4 new metric handles appended to Registry (RouterDecisionsTotal, RouterRingSize, RouterRingRebalancesTotal, RouterProbeFailuresTotal) — nil-safe emission via helpers in proxy.go and probe.go. `cmd/jamsesh-router/main.go` mounts /metrics on a ServeMux in front of the proxy handler (clean separation; metrics scrape doesn't interfere with proxy traffic). publishWithMetrics wrapper is ready for kubernetes-mode wiring.

Docs are thorough. SELF_HOST §14 "Clustered mode (preview)" has full k8s YAML (portal + router Deployments, Services, RBAC), config knob table, metrics table, explicit "preview limitations" naming the three missing capabilities. ARCHITECTURE "Horizontal scaling (clustered mode)" describes the topology accurately — router as consistent-hash proxy, advisory-lock leases, fencing tokens (intent), and notes object-storage-sync + hydration-handoff are still to come. Foundation-doc principle honored — no "previously" prose.

Cardinality safety verified: `addr` label bounded by pod count (typically ≤20), `result` is a fixed enum. No unbounded labels introduced.
