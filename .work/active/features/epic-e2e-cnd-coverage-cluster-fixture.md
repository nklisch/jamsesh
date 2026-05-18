---
id: epic-e2e-cnd-coverage-cluster-fixture
kind: feature
stage: drafting
tags: [e2e-test, testing, infra]
parent: epic-e2e-cnd-coverage
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E CND Coverage — Clustered Fixture (Keystone)

## Brief

The keystone feature for CND e2e coverage. Today `tests/e2e/fixtures/
portal/portal.go` is single-instance only — `Portal` is a single-container
abstraction and `buildEnv` has no object-storage, router, or cluster-
coordinator env vars. Every test in `tests/e2e/{golden,failure,chaos,fuzz}/`
calls `portal.Start(ctx, t, portal.Options{...})` exactly once.

This feature lands two new fixtures and the smoke spec that proves they
work end-to-end:

- `tests/e2e/fixtures/portalcluster/` — `Cluster.Start(ctx, t,
  ClusterOptions{Pods: N, ObjectStore: ObjectStoreFixture, Router: bool})`
  returning a `*Cluster` with `.RouterURL` (when `Router: true`),
  `.Pods []*Portal`, and helpers to gracefully drain or kill individual
  pods.
- `tests/e2e/fixtures/minio/` — Testcontainers MinIO fixture exposing
  `.Endpoint`, `.ContainerEndpoint`, `.AccessKey`, `.SecretKey`,
  `.BucketName` (pre-created), and helpers for direct bucket inspection
  from the test process.

Plus the `cmd/jamsesh-router/` binary needs to ship in a container image
for the cluster fixture to consume — analogous to `make test-portal-image`
for the portal. This is part of this feature's scope.

The smoke spec is the keystone acceptance: bring up a 3-pod cluster + MinIO
+ router, create a session via the router, push a commit on pod A's lease,
verify the object lands in MinIO, drain pod A, verify pod B acquires the
lease and can serve the session. If that test is green, the foundation is
working and the four content features can start.

## Audit findings addressed

- **F15 (Critical, journey-gap, architectural prerequisite)** — Test program
  is single-instance throughout. `tests/e2e/fixtures/portal/portal.go:81-90`
  defines `Portal` as a single-container abstraction. `buildEnv` (lines
  167-220) has no object-storage, router, or cluster-coordinator env vars.
  Every clustered-mode test depends on resolving this gap.
- **F2 fixture half (Critical)** — Object-storage-sync has zero coverage
  because no MinIO / LocalStack / GCS-emulator / Azurite fixture exists
  under `tests/e2e/fixtures/`. The MinIO fixture is the prerequisite for
  the object-storage-sync feature's test bodies (which live in their own
  feature) — but the fixture itself is shared infrastructure and belongs
  here.

## Scope

### Fixtures to add

1. **`tests/e2e/fixtures/minio/`**
   - `minio.go` — `Start(ctx, t) *MinIO` returning a struct with `Endpoint`,
     `ContainerEndpoint`, `AccessKey`, `SecretKey`, `BucketName`. Image:
     `minio/minio:RELEASE.2024-XX-XX` (pin to a known-stable tag).
   - Pre-creates a bucket per test call (random suffix); cleaned up by
     `t.Cleanup`.
   - Self-test: `minio_test.go` proving start + bucket-create + simple PUT
     + GET round-trip succeed.

2. **`tests/e2e/fixtures/router/`**
   - `router.go` — `Start(ctx, t, RouterOptions{Backends: []string}) *Router`
     exposing `.URL`. Uses a router image built from `cmd/jamsesh-router/`.
   - Static-discovery config initially (k8s-discovery testing is its own
     concern in `epic-e2e-cnd-coverage-routing-layer`).
   - Self-test: `router_test.go` proving start + reverse-proxy round-trip
     to a single backend.

3. **`tests/e2e/fixtures/portalcluster/`**
   - `cluster.go` — `Start(ctx, t, ClusterOptions{Pods int, ObjectStore
     *minio.MinIO, Router bool}) *Cluster`.
   - Brings up one shared Postgres (via existing `postgres` fixture),
     one MinIO (passed in or auto-started), N portal containers configured
     with `JAMSESH_DEPLOY_MODE=clustered`, `JAMSESH_OBJECT_STORE_URL=s3-
     compatible://...`, lease config, etc.
   - When `Router: true`, also brings up a router fixture pointing at the
     N portal containers.
   - Exposes `.RouterURL` (or empty if no router), `.Pods []*Portal`, and
     helpers `.GracefulDrain(podIndex)` (SIGTERM + wait), `.Kill(podIndex)`
     (SIGKILL via Pumba), `.WaitForLease(sessionID, podIndex, timeout)`.
   - Self-test: `cluster_test.go` proving 3-pod boot + all pods report
     `/healthz` 200 + advisory-lock-holder query against Postgres returns
     exactly one pod.

### Build pipeline

- New `make test-router-image` target — builds `cmd/jamsesh-router/`
  as a static Linux binary into a container image tagged
  `jamsesh/router:e2e`. Mirrors `make test-portal-image`.
- `make test-e2e` updated to depend on both `test-portal-image` and
  `test-router-image`.
- CI workflow (`.github/workflows/e2e.yml`) updated to build the router
  image alongside the portal image.

### Smoke spec (keystone acceptance)

- `tests/e2e/scaffolding/cluster_smoke_test.go` — single subtest
  `TestClusteredSmoke`:
  1. Start Postgres + MinIO
  2. Start 3-pod portalcluster with router enabled
  3. Create a session via the router URL (uses existing REST helpers)
  4. Push a commit on the session (uses existing `gitclient` helpers)
  5. Verify the object lands in MinIO (direct bucket inspection)
  6. Gracefully drain the pod holding the lease
  7. Verify a different pod acquires the lease within 5s
  8. Verify the new pod serves the same session (REST GET)
  Asserts on user-visible state at every step (HTTP responses, MinIO bucket
  contents, advisory-lock holder identity via direct Postgres query). No
  mock-invocation asserts.

If `TestClusteredSmoke` is green, every other CND-coverage feature is
unblocked to start.

## Mock-boundary plan

| External dep             | Service-level mock                       | Notes |
|--------------------------|------------------------------------------|-------|
| S3 / S3-compatible       | MinIO (`minio/minio:RELEASE...`)         | Off-the-shelf; canonical for S3 compat |
| Postgres (multi-pod)     | Existing `postgres` fixture (postgres:16)| Reuse; share one DB across cluster |
| Router                   | Real `cmd/jamsesh-router/` binary in container | Custom container — Go binary, language-matched to project |
| Multi-pod portal         | Multiple portal containers from `jamsesh/portal:e2e` | Existing image, just multiple instances |

No in-process mocks. The router being custom is acceptable because it's
shipping production Go code in a container, not a hand-rolled mock.

## Open questions for design

- **MinIO image tag pinning policy.** Stable monthly release tag, or the
  `latest` floating tag? Existing fixtures pin (e.g., `postgres:16-alpine`).
  Resolve in design pass.
- **Bucket lifecycle: per-test fresh bucket, or shared bucket with random
  prefixes per test?** Per-test fresh matches the existing postgres-fixture
  pattern (fresh DB per test) but adds bucket-create latency. Resolve in
  design pass.
- **Router image: do we want a separate `make test-router-image` target,
  or can we reuse a single `make test-images` umbrella?** Cosmetic; resolve
  in design pass with the project's `make` conventions.
- **3-pod default cluster size:** sufficient for most tests, but should
  `ClusterOptions.Pods` default to 2 (cheaper) or 3 (more representative)?
  Resolve in design pass.

## Acceptance criteria

- [ ] `tests/e2e/fixtures/minio/` exists with self-test green
- [ ] `tests/e2e/fixtures/router/` exists with self-test green
- [ ] `tests/e2e/fixtures/portalcluster/` exists with self-test green
- [ ] `make test-router-image` produces `jamsesh/router:e2e` deterministically
- [ ] `make test-e2e` builds both portal and router images
- [ ] `tests/e2e/scaffolding/cluster_smoke_test.go > TestClusteredSmoke`
      is green and asserts on user-visible state at every step
- [ ] CI workflow runs `TestClusteredSmoke` and the three fixture self-tests
- [ ] Existing single-instance tests still pass unchanged (no regression
      to the `portal` fixture)
- [ ] README.md updated to document the new fixtures and the cluster smoke
      entry point

## Non-goals

- The four CND content features' actual tests (those are their own features)
- K8s pod-discovery exercise for the router (lives in
  `epic-e2e-cnd-coverage-routing-layer`)
- Multi-bucket / multi-region object-storage scenarios

## Next

Once at stage:drafting and ready to design:
`/agile-workflow:e2e-test-design epic-e2e-cnd-coverage-cluster-fixture`
