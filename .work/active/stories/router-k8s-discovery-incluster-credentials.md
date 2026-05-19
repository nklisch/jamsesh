---
id: router-k8s-discovery-incluster-credentials
kind: story
stage: implementing
tags: [infra, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Router k8s discoverer should auto-load in-cluster CA + service-account token

## Origin

Found during review of `epic-e2e-cnd-coverage-routing-layer-k8s-discovery`.
That story re-introduced `internal/router/discovery/k8s.go` (a plain-HTTP
Endpoints watcher with no client-go dep) and wired it into config. The test
exercises it against an unauthenticated `httptest.Server`, so the
production-deployment credential paths were never required.

For an actual in-cluster deployment, two things are missing:

1. **In-cluster CA cert**. `K8sConfig.HTTPClient` is a vanilla `http.Client`
   by default. Requests to `https://kubernetes.default.svc` will fail with
   x509-verification errors because the in-cluster CA at
   `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt` is not in the
   system trust store.

2. **Service-account bearer token**. `K8sConfig.BearerToken` is empty by
   default. The k8s API server requires authentication for Endpoints reads
   even with RBAC granting the SA the right verbs. The token lives at
   `/var/run/secrets/kubernetes.io/serviceaccount/token` and rotates.

## Fix direction

Add a constructor `K8sInCluster() (K8sConfig, error)` in
`internal/router/discovery/k8s.go` that:

- Reads the SA token file once at startup; populates `BearerToken`.
  Optionally re-reads on each request (the token rotates every few hours;
  read-on-request keeps the watcher alive across rotation).
- Reads the CA cert and constructs an `*http.Client` with a `tls.Config`
  whose `RootCAs` includes it.
- Sets `APIServerURL` to `https://kubernetes.default.svc` if env var
  `KUBERNETES_SERVICE_HOST` is set, otherwise leaves the caller to provide
  it.

Wire this in `cmd/jamsesh-router/main.go` when `DiscoveryMode = kubernetes`
and no explicit `APIServerURL` / `BearerToken` overrides are present.

Add a test that mocks the SA mount via a temp directory + a `K8sInCluster`
variant that takes the mount path as an arg (matching how other in-cluster
patterns in the codebase are testable — check `internal/portal/config/` for
existing precedents).

## Acceptance

- `K8sInCluster` reads CA + SA token from the standard mount path.
- Default `K8sConfig.HTTPClient` (when constructed via `K8sInCluster`) trusts
  the in-cluster CA.
- All requests carry `Authorization: Bearer <token>`.
- Token rotation does not require process restart (read-on-request, or
  goroutine refresh — pick the simpler pattern).
- A unit test mocks the mount path and verifies CA + token are wired
  correctly.
- `docs/SELF_HOST.md` k8s section documents the required RBAC verbs:
  `get`, `list`, `watch` on `endpoints` in the portal namespace.

## Out of scope

- Switching to client-go (the plain-HTTP watcher is intentional — keep it).
- Watching additional resources beyond Endpoints (e.g. EndpointSlice in
  newer clusters — separate story if needed).
