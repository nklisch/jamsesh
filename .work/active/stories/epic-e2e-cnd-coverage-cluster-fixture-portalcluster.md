---
id: epic-e2e-cnd-coverage-cluster-fixture-portalcluster
kind: story
stage: implementing
tags: [e2e-test, testing, infra]
parent: epic-e2e-cnd-coverage-cluster-fixture
depends_on: [epic-e2e-cnd-coverage-cluster-fixture-minio, epic-e2e-cnd-coverage-cluster-fixture-router-image]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# portalcluster fixture

## Scope

The orchestration fixture for clustered-mode e2e tests. Spawns N portal
containers configured for clustered mode against shared Postgres + MinIO,
optionally fronted by a router fixture. Exposes lifecycle helpers
(graceful drain, Pumba kill) and lease-introspection helpers
(LeaseHolder, WaitForLeaseMigration) that downstream test features
(lease-fencing, hydration-handoff) consume.

## Files

- `tests/e2e/fixtures/portalcluster/cluster.go` — `Start`, `Options`,
  `Cluster` struct (with `RouterURL`, `Pods []*portal.Portal`).
- `tests/e2e/fixtures/portalcluster/lifecycle.go` — `GracefulDrain`,
  `Kill`, `LeaseHolder`, `WaitForLeaseMigration`.
- `tests/e2e/fixtures/portalcluster/cluster_test.go` — self-test:
  3-pod boot + all healthy + clean cleanup.

## Implementation notes

Required env vars per portal container (verified in `internal/portal/
config/config.go`):

```
JAMSESH_DEPLOY_MODE=clustered
JAMSESH_DB_DRIVER=postgres
JAMSESH_DB_DSN=<postgres.ContainerDSN>  # shared
JAMSESH_OBJECT_STORAGE_URL=s3://<bucket>/  # set per-cluster, points at MinIO bucket
JAMSESH_OBJECT_STORAGE_ENDPOINT_URL=<minio.ContainerEndpoint>
JAMSESH_OBJECT_STORAGE_PATH_STYLE=true
JAMSESH_OBJECT_STORAGE_REGION=us-east-1
# plus whatever access-key env vars the portal expects — confirm by reading
# config.go and matching the names exactly; do NOT guess
```

Lease config defaults from config.go should suffice for the keystone
smoke test; downstream features (lease-fencing) may tune these via
`Options.PortalExtraEnv`.

### Parallel pod startup

Use `errgroup.Group` to start N pods in parallel (sequential startup of
3 pods would compound to ~90s of test latency). On any pod failure, abort
the group and t.Fatal — partial clusters waste developer time.

### Router enable/disable

When `opts.Router: true`:
1. Start pods first; collect their container IPs.
2. Then start the router fixture with `Backends:
   [<pod0.IP>:8443, ...]`.
3. Set `Cluster.RouterURL` from `router.URL`.

When `opts.Router: false`, `Cluster.RouterURL` is the empty string;
consumers address pods directly via `Cluster.Pods[i].URL`.

### LeaseHolder helper

Direct query against Postgres `pg_locks`:

```sql
SELECT objid FROM pg_locks
WHERE locktype = 'advisory'
  AND objid = hashtext($1)::int4   -- session_id
```

Cross-reference the application connection's `client_addr` (from
`pg_stat_activity`) against each pod's container IP to determine which
pod is the holder. Return the matching pod index, or -1 if no holder.

**Risk**: if `hashtext()` isn't stable across PG major versions, this
query needs adjustment. The keystone smoke test will catch this. If the
direct query is unreliable, file a follow-on story to add a portal-side
lease-debug endpoint behind a build tag.

### Kill helper

Mirror the existing Pumba pattern at
`tests/e2e/chaos/runtime_and_clock_test.go:134-267`. Read it before
writing this — there are subtle details (Pumba image tag, network
namespace sharing, command-line shape) that are easier to copy than
to re-derive.

## Acceptance criteria

- [ ] `Start` validates `Postgres != nil && ObjectStore != nil`, t.Fatal otherwise
- [ ] N pods (default 2) start in parallel via errgroup
- [ ] All pods report `/healthz` 200 within 60s of `Start` returning
- [ ] When `Router: true`, router is started and reachable
- [ ] `GracefulDrain(podIndex, timeout)` returns once the pod has cleanly
      exited, bounded by timeout
- [ ] `Kill(podIndex)` terminates the pod abruptly via Pumba
- [ ] `LeaseHolder(sessionID)` returns the holding pod's index for an
      acquired lease; -1 if none
- [ ] `WaitForLeaseMigration` polls until holder changes or timeout
- [ ] Self-test: 3-pod cluster + Router=false + all healthy + clean cleanup
- [ ] Test skips cleanly when Docker or images are unavailable
- [ ] `go test ./fixtures/portalcluster/...` is green

## Test integrity (from parent epic)

Self-test asserts on real container state (`/healthz` 200) and real
Postgres state (`pg_locks` for LeaseHolder helper validation). Not
tautological.

**Especially watch**: if `LeaseHolder` returns the wrong pod, that's a
real bug — either in the helper's query, the portal's lease wiring, or
the postgres-version `hashtext` assumption. Park as backlog item with
specifics; do not paper over with retries or loose assertions.

## References

- Parent feature body, Unit 3 — full API design
- `tests/e2e/fixtures/postgres/postgres.go` — ContainerDSN pattern
- `tests/e2e/fixtures/portal/portal.go` — single-pod start to compose
- `tests/e2e/chaos/runtime_and_clock_test.go:134-267` — Pumba pattern
- `internal/portal/config/config.go` — env-var name verification

## Dependencies on this story (downstream)

- `epic-e2e-cnd-coverage-cluster-fixture-smoke` (keystone smoke test
  uses the full fixture surface)
- Eventually consumed by lease-fencing, object-storage-sync,
  routing-layer, and hydration-handoff feature test bodies
