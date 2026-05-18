---
id: epic-e2e-cnd-coverage-object-storage-sync-golden-rpo0
kind: story
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-object-storage-sync
depends_on: [epic-e2e-cnd-coverage-cluster-fixture]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Object Storage — Golden RPO=0

Implements `tests/e2e/golden/object_storage_rpo0_test.go`.

## Invariant

After a successful git push, every produced object (loose objects, pack files,
refs) is queryable in the MinIO bucket via direct S3 API before the push ACK
is returned to the client. RPO=0: ACK implies durable.

## Scope

`TestObjectStorageRPO0` with four subtests:

- **`small_commit`** — one small commit; assert `sessions/<id>/objects/`
  prefix has ≥1 key in bucket immediately after push.
- **`multi_pack_push`** — push enough commits to trigger a repack; assert
  pack keys appear under `sessions/<id>/packs/` in bucket.
- **`refs_only_update`** — force-push an existing ref to a new target
  (refs-only, no new objects); assert manifest refs map is updated in bucket.
- **`tag_creation`** — create an annotated tag; assert tag ref appears in
  manifest.

**Test integrity rules (mandatory for implementer)**:
- NEVER assert on push HTTP response code alone. The assertion sequence is:
  1. Execute the push.
  2. Call `mn.ListObjects("sessions/<sessionID>/")` directly against MinIO.
  3. Assert that keys exist.
  4. Only then may you also check the push status code.
- If any subtest finds RPO=0 violated (push returns 2xx but bucket is empty
  or missing the expected keys), this is a production bug. Park it via
  `/agile-workflow:park`, land the subtest with a `t.Skip` linked to the
  backlog ID and a one-line reason (`"RPO=0 violation: push ACKed but objects
  not in bucket — see backlog/<id>"`). Do NOT loosen the assertion to
  "eventually consistent" unless `docs/ARCHITECTURE.md` or `docs/SPEC.md`
  explicitly says RPO is not zero.
- Fix bad fixtures in-session. Never game an assertion to make it pass.

## Acceptance Criteria

- [ ] `TestObjectStorageRPO0` compiles and runs against the cluster-fixture stack
- [ ] Each subtest performs direct bucket inspection via `mn.ListObjects` /
      `mn.GetObject` — not only an HTTP status check
- [ ] Four subtests pass (small_commit, multi_pack_push, refs_only_update,
      tag_creation)
- [ ] Any production bugs (RPO=0 violations) are parked, not silenced
- [ ] No in-process mocks introduced

## Setup pattern

```go
mn := minio.Start(ctx, t, minio.Options{})
pg := postgres.Start(ctx, t, postgres.Options{})
cluster := portalcluster.Start(ctx, t, portalcluster.Options{
    Pods:        2,
    Postgres:    pg,
    ObjectStore: mn,
    Router:      false,
})
// Address cluster.Pods[0].URL for pushes.
// Use mn.ListObjects / mn.GetObject for bucket inspection.
```
