---
id: epic-e2e-cnd-coverage-object-storage-sync-failure-write-rejected
kind: story
stage: done
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-object-storage-sync
depends_on: [epic-e2e-cnd-coverage-cluster-fixture, epic-e2e-cnd-coverage-object-storage-sync-failure-startup]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Object Storage — Failure: Write Rejected

Implements `tests/e2e/failure/object_storage_write_rejected_test.go`.

## Invariant

When the portal cannot persist objects to object storage (bucket does not
exist → `NoSuchBucket` from MinIO), a git push fails with a documented error
response. The push does NOT return 2xx while silently losing the object. RPO=0
means an un-persistable write must surface as failure.

## Scope

`TestObjectStorageWriteRejected` — one test (no subtests required):

- Start MinIO via `minio.Start` but configure the cluster with a
  `JAMSESH_OBJECT_STORAGE_URL` that references a **bucket name that was never
  created**. The MinIO container is reachable; only the named bucket is absent.
- Start `portalcluster` with the missing-bucket URL. If the portal fails at
  startup (bucket validation at boot), treat that as the "loud failure" case
  and skip the runtime-failure assertions — both cases satisfy the invariant.
- If the portal boots, attempt a git push.
- Assert: push returns a non-2xx HTTP status (git smart-HTTP error response).
- Assert: `mn.ListObjects("")` (all objects in the bucket that DOES exist, if
  any, or confirm zero objects in any reachable bucket) — nothing leaked.
- Assert: portal response body (if any) contains a machine-readable error code
  (not a generic 500 without detail).

**Implementation note on startup vs runtime**:
The portal may validate bucket existence at startup. If so, the container
exits non-zero (use the `startFailingPortal` pattern) and no push is possible.
The test should handle both outcomes with a branch:

```go
if containerIsRunning(ctx, cluster) {
    // runtime failure path: attempt push, assert non-2xx
} else {
    // startup failure path: assert container exited with non-zero code
}
```

Either path satisfies the invariant: the portal did NOT silently accept the
write and pretend it succeeded.

**Test integrity rules (mandatory for implementer)**:
- If the portal returns 2xx on a push but the object is absent from the bucket,
  this is an RPO=0 violation — a production bug. Park it via
  `/agile-workflow:park`, skip the assertion with the backlog ID. Do NOT
  change the assertion to accept 2xx.
- Do not use `expect(true).toBe(true)` equivalents — e.g., do not assert
  "the response is either 2xx or non-2xx" (always true, never useful).

## Acceptance Criteria

- [ ] `TestObjectStorageWriteRejected` compiles and runs
- [ ] Push to a portal with a non-existent bucket returns non-2xx (or portal
      exits at startup — both are accepted)
- [ ] No objects land silently in any reachable MinIO bucket
- [ ] Portal does not panic (no unhandled nil-pointer on the error path)
- [ ] Any production RPO=0 violation parked as a backlog bug with a
      `t.Skip` referencing the backlog ID
- [ ] No in-process mocks introduced

## Notes

- The missing-bucket approach was chosen over MinIO IAM policy manipulation
  to avoid adding the `madmin-go` admin SDK to the test fixture. MinIO returns
  `NoSuchBucket` on PutObject when the bucket doesn't exist — this is the
  error path we need.
- The depends_on on `failure-startup` is for the shared `startFailingPortal`
  helper, which should be extracted to a `helpers_test.go` file in the
  `failure/` package if not already done by that story.

## Implementation notes (2026-05-17)

- Implemented as `TestObjectStorageWriteRejected` in
  `tests/e2e/failure/object_storage_write_rejected_test.go`.
- Uses a single portal pod (via `startFailingPortal` from `config_and_deps_test.go`)
  rather than `portalcluster.Start` — this avoids t.Fatal on startup and lets the
  test branch on PATH A (startup exit) vs PATH B (lazy boot + push).
- PATH A: portal validates bucket existence at startup → exits non-zero → asserting
  non-zero exit code and empty real bucket. Invariant satisfied.
- PATH B: AWS S3 client is lazy (bucket existence deferred to first write). Portal
  boots, test signs in, creates org+session, clones repo, commits, and pushes.
  Push is executed via `exec.CommandContext` (not `gitclient.Push`) to capture
  exit status without t.Fatal. If push fails (non-zero) → invariant satisfied.
  If push returns 0 (silent acceptance) → `t.Skip` with backlog reference
  `object-storage-write-rejected-silent-acceptance`.
- Real MinIO bucket (mn.BucketName, the one the fixture creates) is verified
  empty under `sessions/<sessionID>/` after any failure path — no partial
  writes leaked to the reachable bucket.
- `go build ./failure/... && go vet ./failure/...` passes clean.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none — `.work/backlog/object-storage-write-rejected-silent-acceptance.md`
created during this review. The `t.Skip` at line 246 names the ID correctly;
the substrate item now exists as the audit trail.

**Important**: none
**Nits**: none

**Notes**: PATH A / PATH B branching is correct and honest: no tautology,
no silent acceptance of 2xx. Direct bucket inspection via `writeRejectedAssertBucketEmpty`
in both paths verifies no objects leaked to the reachable bucket. The lazy-init
escape hatch skips with a real backlog item (`object-storage-write-rejected-silent-acceptance`)
rather than masking the gap. No in-process mocks.
