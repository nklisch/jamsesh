---
id: epic-e2e-cnd-coverage-routing-layer-k8s-discovery
kind: story
stage: implementing
tags: [e2e-test, testing, portal, infra]
parent: null
depends_on: [epic-e2e-cnd-coverage-cluster-fixture]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — Golden: K8s Discovery (Deferred)

## Why deferred

Deferred from the `epic-e2e-cnd-coverage-routing-layer` design pass. Static-
mode coverage (consistent-hash, hint-cache, 503 retry, chaos) covers all of
the router's behavioral invariants. K8s-mode adds only discovery-layer
coverage — that a new pod coming into the k8s endpoints API gets picked up and
routed to. The engineering effort to stand up an envtest or kube-apiserver
container for a single test is disproportionate relative to the marginal
invariant covered.

Promote from backlog when:
- A k8s-mode production bug surfaces (then a test is the bug's proof).
- The project adds envtest or a lightweight k8s API mock for other purposes
  (amortizes setup cost).
- K8s-mode is enabled in CI (currently only static mode is tested end-to-end).

## Proposed scope (when promoted)

1. Stand up a WireMock stub for the k8s `GET /api/v1/namespaces/{ns}/endpoints`
   and `WATCH` endpoints — the specific APIs `internal/router/discovery/k8s.go`
   uses. Cheaper than full envtest.
2. Start the router in k8s mode (`JAMSESH_ROUTER_DISCOVERY_MODE=kubernetes`,
   `_KUBE_NAMESPACE`, `_KUBE_SERVICE_NAME`) pointing at WireMock.
3. Prime WireMock with 2 portal pod IPs. Assert router serves traffic.
4. Update WireMock to add a 3rd pod IP (simulate k8s endpoint update).
5. Assert: within SLO, the router discovers the new pod and starts routing to
   it (verified via `cluster.LeaseHolder` after establishing a session).

## Notes for the designer

- `internal/router/discovery/k8s.go` uses `client-go` informers. WireMock
  must serve the correct `resourceVersion` fields for the watch re-list to
  work. Alternatively, use a minimal k8s-apiserver container (via
  `bitnami/kube-apiserver` or `k3s`).
- WireMock approach is lighter; try it first.
- The test must run with `KUBECONFIG` pointed at a fake server or with the
  router configured to use the in-cluster rest.Config override — see
  `internal/router/discovery/k8s.go` for config entry points.
