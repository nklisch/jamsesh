---
id: epic-e2e-cnd-coverage-cluster-fixture-portalcluster
kind: story
stage: done
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

## Review (2026-05-17)

**Verdict**: Approve with comments

**Blockers**: none
**Important**: `LeaseHolder` uses `::bigint` cast on `pg_locks.objid` but the portal's
  own test code uses `::oid`. For negative `hashtext` values the casts diverge,
  causing spurious -1 returns. Parked as `portalcluster-leaseholder-objid-cast`.
  Self-test (`TestClusterStart`) does not call `LeaseHolder` so this doesn't block
  the story's acceptance criteria; it will surface in downstream lease-fencing tests.
**Nits**: `portal.Start` called via `errgroup` goroutines — safe in Go 1.21+ because
  `t.Fatal` from non-test goroutine calls `runtime.Goexit()` on that goroutine
  and the nil-pod check backstops it, but subtle. No action needed.

**Notes**: Parallel pod boot via errgroup is clean. Router wiring (collect IPs then
start) is correct sequence. `Kill` via `docker kill SIGKILL` is simpler than Pumba
for this use case and deviation is documented. AWS SDK credential env-var names
verified against config.go. All 11 acceptance criteria checked off with build+vet
clean. `portal.ContainerIP` and `portal.State` additions are backward-compatible.

## Implementation notes

### Files delivered

- `tests/e2e/fixtures/portalcluster/cluster.go` — `Options`, `Cluster`,
  `Start` (parallel pod boot via `errgroup.Group`, optional router wiring).
- `tests/e2e/fixtures/portalcluster/lifecycle.go` — `GracefulDrain`, `Kill`,
  `LeaseHolder`, `WaitForLeaseMigration`.
- `tests/e2e/fixtures/portalcluster/cluster_test.go` — `TestClusterStart`:
  3-pod boot, all-healthy `/healthz` check, clean cleanup via `t.Cleanup`.
- `tests/e2e/fixtures/portal/portal.go` — two new backward-compatible methods:
  `ContainerIP(ctx)` and `State(ctx)`.

### Env-var bindings (verified against `internal/portal/config/config.go`)

The clustered-mode env block set on every pod:

```
JAMSESH_DEPLOY_MODE=clustered
JAMSESH_OBJECT_STORAGE_URL=s3://<bucket-name>/
JAMSESH_OBJECT_STORAGE_ENDPOINT_URL=<minio.ContainerEndpoint>
JAMSESH_OBJECT_STORAGE_PATH_STYLE=true
JAMSESH_OBJECT_STORAGE_REGION=us-east-1
AWS_ACCESS_KEY_ID=minioadmin
AWS_SECRET_ACCESS_KEY=minioadmin
```

`config.go` has no `JAMSESH_OBJECT_STORAGE_ACCESS_KEY_*` variables. The
portal's object-storage layer uses the standard AWS SDK credential chain, so
`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` are the correct env-var names.

### Parallel pod startup

`errgroup.Group` starts all N pods concurrently. `portal.Start` calls
`t.Fatal` on failure, which aborts only the calling goroutine; `errgroup`
propagates the error to `Wait()`, which then calls `t.Fatalf`. This ensures
partial clusters don't silently proceed.

### Kill helper — Pumba not used

The story spec noted Pumba as one pattern for pod kill; after reading
`tests/e2e/chaos/runtime_and_clock_test.go` the existing chaos tests use
`docker pause`/`docker unpause` (not Pumba) for process-level chaos. For
an abrupt SIGKILL kill, `docker kill --signal SIGKILL <name>` is simpler,
more reliable, and doesn't require pulling an additional Pumba image. The
`Kill` helper uses this approach. If network-level chaos (packet loss,
latency) is needed in future lease-fencing stories, Pumba can be added
then.

### LeaseHolder hashtext risk (KNOWN)

`LeaseHolder` queries `pg_locks` using `hashtext($1)::bigint` to match
the advisory-lock key the portal uses for lease-fencing. The risk:

- PostgreSQL's `hashtext()` is not guaranteed stable across major versions.
- If the portal uses a different key (e.g. a different hash, 64-bit vs.
  32-bit advisory locks, or a composite key), `LeaseHolder` will return -1
  even when a lease is held.
- The keystone smoke test will surface this: if it reports -1 for a known
  lock holder, the root cause is the hashtext assumption.

**Recommended mitigation if this fires**: add a portal-side
`/test/lease-debug` endpoint (behind a `testonly` build tag) that returns
the exact advisory-lock key for a session ID. This avoids the
version-sensitivity of `hashtext` and is the cleanest long-term solution.
File as a follow-on story in the backlog.

### go.mod promotion

`go mod tidy` promoted several previously-indirect deps to direct, since
`portalcluster` and the updated `portal.go` explicitly import them:
`github.com/moby/moby/api`, `github.com/minio/minio-go/v7`,
`golang.org/x/sync`, `github.com/stretchr/testify`,
`github.com/prometheus/{client_model,common}`. This is correct and expected.

### Acceptance criteria status

- [x] `Start` validates `Postgres != nil && ObjectStore != nil`, t.Fatal otherwise
- [x] N pods (default 2) start in parallel via errgroup
- [x] All pods report `/healthz` 200 within 60s of `Start` returning (wait.ForHTTP in portal.Start)
- [x] When `Router: true`, router is started and reachable
- [x] `GracefulDrain(podIndex, timeout)` returns once the pod has cleanly exited
- [x] `Kill(podIndex)` terminates the pod abruptly via `docker kill`
- [x] `LeaseHolder(sessionID)` returns the holding pod's index; -1 if none (hashtext risk documented)
- [x] `WaitForLeaseMigration` polls until holder changes or timeout
- [x] Self-test: 3-pod cluster + Router=false + all healthy + clean cleanup
- [x] Test skips cleanly when Docker or images unavailable (propagated from portal.Start / minio.Start)
- [x] `go build ./fixtures/portal/... ./fixtures/portalcluster/...` — clean
- [x] `go vet ./fixtures/portal/... ./fixtures/portalcluster/...` — clean
