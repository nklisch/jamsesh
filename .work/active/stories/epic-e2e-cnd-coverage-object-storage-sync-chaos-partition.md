---
id: epic-e2e-cnd-coverage-object-storage-sync-chaos-partition
kind: story
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-object-storage-sync
depends_on: [epic-e2e-cnd-coverage-cluster-fixture, epic-e2e-cnd-coverage-object-storage-sync-golden-rpo0]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Object Storage — Chaos: Network Partition

Implements `tests/e2e/chaos/object_storage_partition_test.go`.

## Invariant

RPO=0 holds across bounded partitions (latency, transient disconnect). Writes
that are in-flight retry and eventually land in the bucket once the partition
heals. Permanent disconnects fail loudly — no push returns 2xx with an empty
bucket.

## Scope

`TestObjectStoragePartition` with three sub-scenarios:

### `latency_5s_writes_succeed`

- Inject 5000ms latency via Toxiproxy on the portal→MinIO path.
- Push a commit. Assert: push eventually succeeds (within 30s timeout).
- Assert: `mn.ListObjects("sessions/<id>/")` shows the objects in the bucket.
- Remove toxic. Assert: recovery.

### `transient_reset_peer_rpo0_holds`

- Inject `reset_peer` toxic on the portal→MinIO path for 3s, then remove it.
- Attempt a push during the toxic window.
- If push returned 2xx: assert objects are in the bucket (RPO=0).
- If push returned non-2xx: assert no objects leaked (consistent failure).
- The forbidden case: push returns 2xx AND bucket is empty.

### `permanent_disconnect_fails_loudly`

- Inject `reset_peer` toxic permanently (no removal during the test).
- Attempt a push. Assert: push returns a non-2xx response.
- Assert: bucket has zero objects for the session (nothing leaked silently).

## Stack setup

```go
mn := minio.Start(ctx, t, minio.Options{})
tp := toxiproxy.Start(ctx, t)
pg := postgres.Start(ctx, t, postgres.Options{})

// Toxiproxy proxy: bridge-network port 9001 → MinIO bridge IP:9000
const (
    proxyName   = "minio"
    proxyListen = "0.0.0.0:9001"
)
// stripScheme removes "http://" from mn.ContainerEndpoint to get "ip:9000"
tp.CreateProxy(ctx, t, proxyName, proxyListen,
    stripScheme(mn.ContainerEndpoint))

// Cluster: portal routes through Toxiproxy; test helpers bypass it.
cluster := portalcluster.Start(ctx, t, portalcluster.Options{
    Pods:        2,
    Postgres:    pg,
    ObjectStore: mn, // credentials + bucket name
    PortalExtraEnv: map[string]string{
        // Override the endpoint so portal writes through Toxiproxy.
        "JAMSESH_OBJECT_STORAGE_ENDPOINT_URL": "http://" + tp.ContainerIP + ":9001",
    },
})
// Direct MinIO access for test assertions bypasses Toxiproxy.
// mn.ListObjects uses mn.Endpoint (host-side, no Toxiproxy).
```

**Test integrity rules (mandatory for implementer)**:
- RPO=0 is the safety-critical invariant. The bucket inspection (via
  `mn.ListObjects`) is the assertion target, not the push HTTP status code.
- The forbidden case in `transient_reset_peer_rpo0_holds`: push=2xx AND
  bucket empty after partition heals. If this occurs, it is a production bug.
  Park it via `/agile-workflow:park`, skip the subtest with the backlog ID:
  `"RPO=0 violated under transient partition — see backlog/<id>"`. Do NOT
  change the assertion to allow this outcome.
- If the chaos test surfaces a real RPO violation under the latency scenario
  (push ACK races the upload), park it with the same protocol.
- Do not assert "the portal either succeeds or fails" — that is always true
  and tests nothing.

## Acceptance Criteria

- [ ] `TestObjectStoragePartition` compiles and runs against the cluster-fixture stack
- [ ] Toxiproxy intercept is verified: the latency subtest shows elevated push
      duration (> 5s baseline), confirming the portal routes through Toxiproxy
- [ ] `latency_5s_writes_succeed`: push eventually succeeds, objects in bucket
- [ ] `transient_reset_peer_rpo0_holds`: no case of 2xx + empty bucket
- [ ] `permanent_disconnect_fails_loudly`: push returns non-2xx, bucket empty
- [ ] Direct bucket inspection via `mn.ListObjects` used for all RPO assertions
- [ ] Any RPO=0 violations parked as production bugs, not silenced
- [ ] No in-process mocks introduced

## Notes

- The `tp.ContainerIP` is the Docker bridge IP of the Toxiproxy container —
  the portal container can reach it directly without host-port mapping.
- `PortalExtraEnv` overrides are applied after `sharedEnv` in
  `portalcluster.Start` (verified at `cluster.go:119`). The Toxiproxy
  endpoint override will take precedence over the MinIO default.
- Baseline timing: verify the stack produces fast pushes (< 2s) before
  injecting any toxic, to confirm the chaos results are meaningful.
