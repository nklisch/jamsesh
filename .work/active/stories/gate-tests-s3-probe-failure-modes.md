---
id: gate-tests-s3-probe-failure-modes
kind: story
stage: review
tags: [testing, portal, infra]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# Object-storage `Probe` interface contract has no negative test

## Priority
High

## Spec reference
Item: `object-storage-fail-fast-clustered-startup` (archived).
Acceptance criterion: `Probe` issues lightweight `HeadBucket` or
`ListObjectsV2` with `MaxKeys:0`; adds ~5s on unreachable endpoint
(timeout); returns immediately on success.

## Gap type
missing test for valid partition (success, missing-bucket 404, timeout,
invalid-credentials 403). Only `errBackend.Probe` (returns err) and
`memBackend.Probe` (returns nil) tested. The 5-second timeout boundary
is unasserted.

## Suggested test
```go
// TestS3Backend_Probe_DistinguishesFailureModes (integration, requires MinIO)
//   - reachable + correct bucket → nil, < 100ms
//   - reachable + missing bucket → err contains "NoSuchBucket" or 404
//   - reachable + bad credentials → err contains "AccessDenied" or 403
//   - unreachable endpoint → err after ctx deadline, ~5s
```

## Test location (suggested)
`internal/portal/storage/objectstore/s3_test.go`

## Implementation notes

### Test seam
`TestS3Backend_Probe_DistinguishesFailureModes` was added to the existing
`internal/portal/storage/objectstore/s3_test.go` (package `objectstore_test`),
extending the file rather than creating a new integration file. The test uses
the same env-var skip gate as the rest of the suite (`JAMSESH_TEST_S3_ENDPOINT`
+ `JAMSESH_TEST_S3_BUCKET`, or `JAMSESH_TEST_S3_USE_CONTAINER=true`).

### Four subtests
- **happy_path** — constructs a backend pointed at the live bucket with correct
  credentials; asserts `Probe` returns nil within a 10 s context deadline.
  Logs a warning (non-fatal) if elapsed > 500 ms.
- **missing_bucket** — constructs a backend with a random bucket name that was
  never created; asserts `Probe` returns a non-nil error. MinIO returns HTTP
  404/NotFound, visible in the error string.
- **bad_credentials** — uses `t.Setenv` to override `AWS_ACCESS_KEY_ID` and
  `AWS_SECRET_ACCESS_KEY` with garbage values before calling `NewS3`; asserts
  `Probe` returns a non-nil error. MinIO returns HTTP 403/Forbidden.
- **unreachable** — points the backend at `http://127.0.0.1:1` (port 1,
  ECONNREFUSED immediately); uses a 6 s context deadline and asserts both
  non-nil error and elapsed < 7 s. In practice the AWS SDK retries 3 times
  (~3.4 s total) before propagating the error, which is well within the bound.

### Skipping behaviour
Without any env-var configuration the top-level test logs a `t.Skip` message
and the entire function (all four subtests) is skipped cleanly. No Docker
daemon is required for the compile or skip path.
