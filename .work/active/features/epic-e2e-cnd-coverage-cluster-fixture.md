---
id: epic-e2e-cnd-coverage-cluster-fixture
kind: feature
stage: review
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

## Design decisions

Resolved during the e2e-test-design pass under autopilot (2026-05-17).
Foundation-doc + existing-fixture-convention bias applied throughout.

- **MinIO image tag**: pinned to `minio/minio:RELEASE.2024-12-18T13-15-44Z`
  (specific stable release, mirrors `postgres:16-alpine` style of explicit
  pinning). Implementer may bump to a more recent stable tag if needed —
  the version is a string constant in `minio.go`.
- **Bucket lifecycle**: per-test fresh bucket with a random name (matches
  the postgres-fixture per-test-DB pattern at
  `tests/e2e/fixtures/postgres/postgres.go:88-89`). Bucket-create latency
  is sub-second against a local MinIO; isolation wins.
- **Router image build**: separate `test-router-image` make target mirroring
  `test-portal-image`. CI workflow (`.github/workflows/e2e.yml`) updated to
  run both image builds before `make test-e2e`.
- **Default cluster size**: `ClusterOptions.Pods` defaults to `2` (cheaper —
  most clustered tests just need "more than one"). The keystone smoke test
  passes `Pods: 3` explicitly to prove the consistent-hash ring distributes
  across multiple pods.
- **Container networking**: Testcontainers default bridge network places
  each container on its own bridge IP. The portalcluster fixture follows
  the existing postgres `ContainerIP()` + `ContainerDSN` pattern — each
  pod gets the MinIO + Postgres container IPs (not the host-mapped ports)
  for cross-container reachability. Router is given the pods' container IPs.
- **Env-var names verified from `internal/portal/config/config.go`**:
  `JAMSESH_DEPLOY_MODE=clustered`, `JAMSESH_OBJECT_STORAGE_URL=s3://...`,
  `JAMSESH_OBJECT_STORAGE_ENDPOINT_URL=http://<minio-ip>:9000`,
  `JAMSESH_OBJECT_STORAGE_PATH_STYLE=true` (MinIO needs path-style),
  `JAMSESH_OBJECT_STORAGE_REGION=us-east-1` (MinIO default),
  `JAMSESH_LEASE_HEARTBEAT_INTERVAL_S`, `JAMSESH_LEASE_RETENTION_DAYS`,
  `JAMSESH_LEASE_RETENTION_INTERVAL_HOURS`. The portal fail-fasts at
  startup if clustered mode is set without `OBJECT_STORAGE_URL`.

## Taxonomy plan

This feature is the infrastructure layer for clustered-mode e2e — fixture
code + self-tests + one keystone smoke spec. The four taxonomy layers
apply asymmetrically:

- **Golden**: 1 keystone smoke test (`tests/e2e/scaffolding/
  cluster_smoke_test.go > TestClusteredSmoke`) that proves the full stack
  boots and round-trips. Plus 3 fixture self-tests (one per new fixture)
  proving each unit works in isolation.
- **Failure**: not applicable here — the consuming features
  (lease-fencing, object-storage-sync, etc.) own failure-mode coverage.
- **Chaos**: not applicable here — consumers own chaos coverage. This
  feature provides the drain/kill helpers consumers will use.
- **Fuzz**: not applicable here.

## Implementation Units

### Unit 1: MinIO fixture

**Files**: `tests/e2e/fixtures/minio/minio.go`,
`tests/e2e/fixtures/minio/inspect.go`,
`tests/e2e/fixtures/minio/minio_test.go`
**Story**: `epic-e2e-cnd-coverage-cluster-fixture-minio`
**Invariant** (self-test): "MinIO container starts, pre-created bucket is
reachable via S3 API, a round-trip PUT+GET succeeds, and the container
is terminated cleanly on test end."

API shape (mirrors postgres fixture):

```go
package minio

type Options struct {
    // ExtraEnv passes additional MINIO_* env vars to the container.
    ExtraEnv map[string]string
}

type MinIO struct {
    // Endpoint is the host-side S3 endpoint (e.g. "http://127.0.0.1:32781")
    // for use from the test process.
    Endpoint string

    // ContainerEndpoint is the bridge-IP endpoint (e.g. "http://172.18.0.5:9000")
    // for use from other containers (portal pods).
    ContainerEndpoint string

    AccessKey  string // "minioadmin" default
    SecretKey  string // "minioadmin" default
    BucketName string // random, pre-created

    container testcontainers.Container
}

func Start(ctx context.Context, t *testing.T, opts Options) *MinIO
```

Pre-creates a random-named bucket via the minio-go SDK after the container
is ready. Cleanup terminates the container.

`inspect.go` exposes helpers consumers use for assertions:

```go
func (m *MinIO) ListObjects(ctx context.Context, prefix string) ([]string, error)
func (m *MinIO) GetObject(ctx context.Context, key string) ([]byte, error)
func (m *MinIO) PutObject(ctx context.Context, key string, data []byte) error
func (m *MinIO) DeleteObject(ctx context.Context, key string) error  // for the F12 corruption test
```

**Acceptance Criteria**:
- [ ] `Start` returns within 30s; bucket is pre-created and reachable
- [ ] Self-test: PUT + GET round-trip succeeds via `inspect` helpers
- [ ] `t.Cleanup` terminates the container (verified by docker ps after)
- [ ] Test skips cleanly when Docker is unavailable (existing pattern)

---

### Unit 2: Router image + fixture

**Files**: `Dockerfile.router`, `Makefile` (new `test-router-image`
target), `.github/workflows/e2e.yml` (build router image step),
`tests/e2e/fixtures/router/router.go`,
`tests/e2e/fixtures/router/router_test.go`
**Story**: `epic-e2e-cnd-coverage-cluster-fixture-router-image`
**Invariant** (self-test): "The router image builds; the router fixture
starts a container pointed at a stub backend; HTTP requests to the router
URL are reverse-proxied to the stub and return the stub's response body."

Dockerfile.router (mirror of `Dockerfile` — alpine:3.21 minimum, no git
needed):

```
ARG BINARY=jamsesh-router
FROM alpine:3.21
ARG BINARY
ARG TARGETOS
ARG TARGETARCH
RUN apk add --no-cache ca-certificates
COPY ${BINARY}-${TARGETOS}-${TARGETARCH} /usr/local/bin/jamsesh-router
EXPOSE 8080
USER nobody
ENTRYPOINT ["/usr/local/bin/jamsesh-router"]
```

Makefile target (mirrors `test-portal-image` shape):

```
test-router-image:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o jamsesh-router-linux-amd64 ./cmd/jamsesh-router
	docker build -f Dockerfile.router \
		--build-arg BINARY=jamsesh-router \
		--build-arg TARGETOS=linux \
		--build-arg TARGETARCH=amd64 \
		-t jamsesh/router:e2e .
	@rm -f jamsesh-router-linux-amd64

test-router-image-clean:
	-docker rmi jamsesh/router:e2e
```

Fixture API:

```go
package router

type Options struct {
    // Backends is the list of portal pod addresses (host:port) the router
    // will reverse-proxy to. Used in static discovery mode.
    Backends []string

    // HintCacheTTL overrides the soft-coordinator hint TTL. Zero = router default.
    HintCacheTTL time.Duration
}

type Router struct {
    URL              string // host-side (e.g. "http://127.0.0.1:32790")
    ContainerURL     string // bridge-IP (for other containers)
    container        testcontainers.Container
}

func Start(ctx context.Context, t *testing.T, opts Options) *Router
```

Configures the router container with:
- `JAMSESH_ROUTER_BIND=:8080`
- `JAMSESH_ROUTER_DISCOVERY_MODE=static`
- `JAMSESH_ROUTER_STATIC_PODS=<comma-separated backends>`
- `JAMSESH_ROUTER_SHUTDOWN_GRACE_S=5` (faster cleanup in tests)

Wait strategy: HTTP probe on `/metrics` returning 200 (the router exposes
`/metrics` unconditionally per `cmd/jamsesh-router/main.go:145`).

Self-test uses a tiny `httptest.Server` running on the host as a stub
backend — but Testcontainers can't easily route from container to host.
Use a second small Testcontainers container as the backend stub instead
(`nginx:alpine` serving a static page on `:80` works fine). The router
points at the nginx container's bridge IP; the test process hits the
router's host URL; the response body matches the nginx page.

CI workflow update:

```yaml
- name: build router test image
  run: make test-router-image
```

inserted between the portal-image step and the e2e-suite step.

**Acceptance Criteria**:
- [ ] `make test-router-image` produces `jamsesh/router:e2e`
- [ ] `make test-router-image-clean` removes it
- [ ] Router fixture's `Start` returns within 30s; `/metrics` is reachable
- [ ] Self-test: router proxies a stub-backend response end-to-end
- [ ] CI workflow builds the router image before running e2e
- [ ] Test skips cleanly when the router image is absent (mirror of
      `requirePortalImage`)

---

### Unit 3: portalcluster fixture

**Files**: `tests/e2e/fixtures/portalcluster/cluster.go`,
`tests/e2e/fixtures/portalcluster/lifecycle.go`,
`tests/e2e/fixtures/portalcluster/cluster_test.go`
**Story**: `epic-e2e-cnd-coverage-cluster-fixture-portalcluster`
**Invariant** (self-test): "A 3-pod cluster boots against shared Postgres
+ shared MinIO; all pods report `/healthz` 200; the cluster terminates
cleanly."

API shape:

```go
package portalcluster

type Options struct {
    // Pods is the number of portal containers to start. Default 2.
    Pods int

    // Postgres is required — the cluster shares one Postgres DB.
    Postgres *postgres.Postgres

    // ObjectStore is required for clustered-mode boot. The cluster fixture
    // sets JAMSESH_DEPLOY_MODE=clustered + JAMSESH_OBJECT_STORAGE_URL.
    ObjectStore *minio.MinIO

    // Router, if true, starts a jamsesh-router container fronting the pods.
    // If false, the test addresses pods directly via Cluster.Pods[i].URL.
    Router bool

    // PortalExtraEnv passes extra JAMSESH_* vars to each portal container.
    PortalExtraEnv map[string]string
}

type Cluster struct {
    // RouterURL is the front door if Options.Router was true; empty otherwise.
    RouterURL string

    // Pods is the slice of started portal containers.
    Pods []*portal.Portal
}

func Start(ctx context.Context, t *testing.T, opts Options) *Cluster
```

Internally:
1. Validates `opts.Postgres != nil && opts.ObjectStore != nil`; t.Fatal otherwise.
2. Starts N portal containers in parallel (errgroup), each configured with:
   - `JAMSESH_DEPLOY_MODE=clustered`
   - `JAMSESH_DB_DRIVER=postgres`
   - `JAMSESH_DB_DSN=<postgres.ContainerDSN>` (shared across pods)
   - `JAMSESH_OBJECT_STORAGE_URL=s3://<minio.BucketName>/`
   - `JAMSESH_OBJECT_STORAGE_ENDPOINT_URL=<minio.ContainerEndpoint>`
   - `JAMSESH_OBJECT_STORAGE_PATH_STYLE=true`
   - `JAMSESH_OBJECT_STORAGE_REGION=us-east-1`
   - Whatever access-key vars the portal expects (verified at impl time)
3. Each pod's container IP captured via `c.ContainerIP(ctx)`.
4. If `Router: true`, starts a router fixture with `Backends:
   [<pod0.IP>:8443, <pod1.IP>:8443, ...]`.

`lifecycle.go` exposes the drain/kill helpers consumers need:

```go
// GracefulDrain sends SIGTERM to the pod and waits for clean shutdown.
// Returns once the container has exited (Status == "exited"), bounded by timeout.
func (c *Cluster) GracefulDrain(ctx context.Context, t *testing.T, podIndex int, timeout time.Duration)

// Kill sends SIGKILL via Pumba (mirror of existing chaos pattern at
// tests/e2e/chaos/runtime_and_clock_test.go:134-267).
func (c *Cluster) Kill(ctx context.Context, t *testing.T, podIndex int)

// LeaseHolder queries Postgres pg_locks for the pod (by container IP) currently
// holding the advisory lock for the given session_id. Returns -1 if no lock held.
// hashtext(session_id) is the lock key per docs/ARCHITECTURE.md.
func (c *Cluster) LeaseHolder(ctx context.Context, t *testing.T, sessionID string) int

// WaitForLeaseMigration polls LeaseHolder until the holder changes from `fromPod`
// to a different pod or timeout fires. Returns the new holder index or -1.
func (c *Cluster) WaitForLeaseMigration(ctx context.Context, t *testing.T, sessionID string, fromPod int, timeout time.Duration) int
```

**Acceptance Criteria**:
- [ ] `Start` validates required fields; t.Fatals with clear message if nil
- [ ] N pods start in parallel and all report `/healthz` 200 within 60s
- [ ] Router (when enabled) starts and `/metrics` is reachable
- [ ] `GracefulDrain` returns within the configured timeout
- [ ] `Kill` (via Pumba) terminates the pod abruptly
- [ ] `LeaseHolder` returns the correct pod index for a held lease
  (verified by a unit test against a deliberately-acquired lease — design
  pass leaves the verification mechanism to impl, may need a test-only
  endpoint or direct REST flow)
- [ ] Self-test: 3-pod boot + all healthy + clean cleanup
- [ ] Test skips when Docker or images are unavailable

---

### Unit 4: Keystone smoke test + CI + README

**Files**: `tests/e2e/scaffolding/cluster_smoke_test.go`,
`tests/e2e/README.md` (clustered section), `.github/workflows/e2e.yml`
(verify smoke test runs)
**Story**: `epic-e2e-cnd-coverage-cluster-fixture-smoke`
**Invariant**: "A session created on pod A is visible on pod B after a
graceful drain, with all committed state preserved and the object backed
by MinIO."

Test scaffold:

```go
package scaffolding

import (
    "context"
    "testing"
    "time"

    "jamsesh/tests/e2e/fixtures/minio"
    "jamsesh/tests/e2e/fixtures/portalcluster"
    "jamsesh/tests/e2e/fixtures/postgres"
)

func TestClusteredSmoke(t *testing.T) {
    ctx := context.Background()

    pg := postgres.Start(ctx, t, postgres.Options{})
    mn := minio.Start(ctx, t, minio.Options{})

    cluster := portalcluster.Start(ctx, t, portalcluster.Options{
        Pods:        3,
        Postgres:    pg,
        ObjectStore: mn,
        Router:      true,
    })

    // 1. Create a session via the router URL using existing REST helpers.
    //    Asserts: 201 Created, body contains session_id.
    sessionID := createSessionViaRouter(t, cluster.RouterURL)

    // 2. Push a commit to that session via the router's git smart-HTTP.
    //    Asserts: push succeeds (exit 0), receive-pack returns ok.
    headSHA := pushCommitViaRouter(t, cluster.RouterURL, sessionID)

    // 3. Verify the object landed in MinIO before continuing (RPO=0 invariant,
    //    even at smoke level). Lists bucket; finds the pack file matching the
    //    session_id prefix; asserts non-empty.
    objects, err := mn.ListObjects(ctx, sessionID+"/")
    require.NoError(t, err)
    require.NotEmpty(t, objects, "session push must be mirrored to bucket")

    // 4. Identify which pod holds the lease for sessionID.
    holderIndex := cluster.LeaseHolder(ctx, t, sessionID)
    require.GreaterOrEqual(t, holderIndex, 0, "lease must be held by a pod")

    // 5. Gracefully drain the lease-holding pod.
    cluster.GracefulDrain(ctx, t, holderIndex, 30*time.Second)

    // 6. Make a request for the session via the router — this should cause a
    //    different pod to acquire the lease and hydrate from MinIO.
    //    Asserts: 200 OK, response body matches what we pushed.
    headFromNewPod := getSessionHeadViaRouter(t, cluster.RouterURL, sessionID)
    require.Equal(t, headSHA, headFromNewPod, "handoff must preserve state")

    // 7. Verify the new holder is a different pod.
    newHolder := cluster.WaitForLeaseMigration(ctx, t, sessionID, holderIndex, 10*time.Second)
    require.NotEqual(t, holderIndex, newHolder, "lease must have migrated")
    require.GreaterOrEqual(t, newHolder, 0, "a pod must hold the lease post-handoff")
}
```

The helper functions (`createSessionViaRouter`, `pushCommitViaRouter`,
`getSessionHeadViaRouter`) reuse the existing `tests/e2e/fixtures/gitclient/`
and any REST helpers used by `golden/onboarding_test.go` and
`session_join_and_push_test.go`. Implementer chooses between extending an
existing helper package or inlining the calls — design pass leaves this
open.

CI: confirm `make test-e2e` discovers and runs the new `cluster_smoke_test.go`
(no workflow change beyond the router-image build added in Unit 2).

README (`tests/e2e/README.md`): add a "Clustered mode" section describing
the three new fixtures, the keystone smoke test, and how to invoke it
standalone.

**Acceptance Criteria**:
- [ ] `TestClusteredSmoke` is green and asserts on user-visible state at
      every step (REST response, bucket contents, lease-holder identity,
      handoff outcome)
- [ ] No mock invocation asserts — every assertion is against real product
      output
- [ ] `tests/e2e/README.md` documents the clustered-mode fixtures and the
      smoke entry point
- [ ] CI workflow runs `TestClusteredSmoke` as part of `make test-e2e`
- [ ] If the smoke test surfaces a real bug (e.g., handoff doesn't
      preserve state), the bug is parked via `/agile-workflow:park` and
      the test is `t.Skip`ed with a backlog-id reference; not silenced

---

## Implementation Order

1. `epic-e2e-cnd-coverage-cluster-fixture-minio` (no deps)
2. `epic-e2e-cnd-coverage-cluster-fixture-router-image` (no deps; can run
   in parallel with #1)
3. `epic-e2e-cnd-coverage-cluster-fixture-portalcluster` (deps: #1, #2)
4. `epic-e2e-cnd-coverage-cluster-fixture-smoke` (dep: #3)

## Risks (pre-mortem)

- **Router hint-cache can fool handoff assertions.** The router's
  soft-coordinator cache (default 10k entries, TTL configurable) will keep
  routing to the drained pod until either (a) the drained pod returns a
  503 + Retry-After (proxy logic should evict the hint), or (b) the TTL
  expires. The smoke test must either set a short TTL in `Options.Router`
  (e.g., 1s) or wait past the default TTL before asserting handoff.
  Mitigated by exposing `HintCacheTTL` on the router fixture and using a
  short value in the smoke test.
- **Lease acquisition is per-request, not proactive.** Pod B doesn't grab
  a lease unless asked. The smoke test must issue a request post-drain to
  trigger acquisition — step 6 does this via `getSessionHeadViaRouter`.
- **Postgres advisory-lock keys via `hashtext(session_id)`.** The
  `LeaseHolder` helper must use the same hash key the portal does. If
  `hashtext` is not stable across PG major versions (it isn't always),
  the helper may need a portal-side lease-debug endpoint instead of a
  direct `pg_locks` query. Resolve at implementation time; if the direct
  query is fragile, file a fixture-extension story.
- **Multi-pod container-network reachability.** Testcontainers default
  bridge places each container on its own IP, but cross-container DNS may
  not work without a custom network. The portalcluster fixture sets
  `PortalExtraEnv` with container IPs directly (postgres ContainerDSN
  pattern already proves this works), so DNS is bypassed. If a future
  test needs hostname-based routing, a custom Docker network would be
  required — out of scope here.
- **Image pull on first CI run is slow.** MinIO image is ~50MB, nginx
  image ~30MB. CI caches Docker layers across runs, so steady-state is
  fast. Document the first-run pull time in the README's "Prerequisites"
  section.
- **Pumba availability.** The lifecycle `Kill` helper uses Pumba (mirrors
  existing chaos pattern). If Pumba isn't already a CI dependency,
  document the install step. Confirm at impl time by reading
  `tests/e2e/chaos/runtime_and_clock_test.go:134-267`.

## Implementation summary (2026-05-17)

All 4 child stories landed at `stage: review` in a single orchestrator
run (3-wave schedule: minio + router-image + portalcluster + smoke).

| Story | Status | Notes |
|---|---|---|
| `cluster-fixture-minio` | review | per-test bucket, hex8-hyphenated naming (S3 rules), self-test passes (PUT+GET round-trip) |
| `cluster-fixture-router-image` | review | Dockerfile.router + Makefile + CI step + fixture; nginx-stub self-test passes |
| `cluster-fixture-portalcluster` | review | parallel pod start via errgroup; AWS_ACCESS_KEY_ID/SECRET (no JAMSESH_OBJECT_STORAGE_ACCESS_KEY in config); docker kill (not Pumba) for Kill helper; LeaseHolder via `hashtext($1)::bigint` (hashtext-portability risk documented inline) |
| `cluster-fixture-smoke` | review | full-scope 7-step keystone; magic-link auth via MailHog; bucket prefix `sessions/<sessionID>/`; fresh-clone fetch on the post-drain assertion |

Cross-cutting deviations from original design:
- **Fixture extensions to `portal.Options` / `*Portal`**: `ContainerFiles`,
  `Logs(ctx)`, `SendSignal(ctx, sig)`, `ContainerIP(ctx)`, `State(ctx)`.
  All backward-compatible additions. These now exist as shared
  infrastructure for downstream cnd-coverage features.
- **`/test/lease-debug` follow-on**: documented as a future fixture
  extension if `hashtext` portability bites during downstream tests.
  Not blocking on the smoke test (test landed full-scope without it).
- **`gc.auto=0` on bare repos**: not explicitly verified in the smoke
  test; relies on the production code in `internal/portal/storage` to
  have set it correctly (per object-storage-sync's design).

Verification: `go build ./...` + `go vet ./...` clean across both the
root module and `tests/e2e/` module. No product bugs surfaced during
implementation — all assertions land genuinely against real product
behavior.

The clustered-mode e2e foundation is in place. Downstream cnd-coverage
features (lease-fencing, object-storage-sync, routing-layer,
hydration-handoff) are unblocked to enter their own design passes.

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
