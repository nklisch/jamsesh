---
id: object-storage-fail-fast-clustered-startup
kind: story
stage: implementing
tags: [bug, portal, object-storage]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Bug: Portal Does Not Fail Fast on Unreachable Object Storage in Clustered Mode

## Summary

A portal started with `JAMSESH_DEPLOY_MODE=clustered` and an unreachable
`JAMSESH_OBJECT_STORAGE_ENDPOINT_URL` boots successfully and returns `/healthz
200` instead of exiting non-zero at startup.

## Root Cause

The AWS SDK v2 S3 client (`objectstore.NewS3`) is constructed lazily: it creates
the client struct and validates credentials, but does not probe the endpoint at
construction time. As a result, `objectstore.New` in `main.go` returns without
error even when the endpoint is completely unreachable. The first actual I/O
(a `PutObject` or `HeadObject` call during a git push) is where the failure
surfaces — too late for a fail-fast guarantee.

## Invariant Violated

ARCHITECTURE.md §389 / SPEC.md §476-477: the object-storage backend is a
mandatory dependency in clustered mode. A misconfigured endpoint should be
detected at deploy time (startup), not at the first push.

## Fix

Add a startup connectivity probe in `main.go` after `objectstore.New` succeeds
but before the HTTP listener starts:

```go
probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
if err := store.Probe(probeCtx); err != nil {
    return fmt.Errorf("object storage connectivity check failed: %w", err)
}
```

The `Probe` method should issue a lightweight `HeadBucket` or `ListObjectsV2`
(with `MaxKeys: 0`) against the configured bucket. This adds ~5s to the startup
path only when the endpoint is unreachable (a timeout), and returns immediately
on success.

The probe must only run when `JAMSESH_DEPLOY_MODE=clustered`.

## Discovered By

E2E test `TestObjectStorageUnreachableAtStartup/clustered_mode_fails_fast` in
`tests/e2e/failure/object_storage_unreachable_at_startup_test.go`.
The test skips with this item's ID when the invariant is not met.
