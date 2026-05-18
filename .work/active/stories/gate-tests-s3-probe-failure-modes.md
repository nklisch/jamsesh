---
id: gate-tests-s3-probe-failure-modes
kind: story
stage: implementing
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
