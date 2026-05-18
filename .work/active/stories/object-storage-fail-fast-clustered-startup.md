---
id: object-storage-fail-fast-clustered-startup
kind: story
stage: review
tags: [bug, portal, object-storage]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-18
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

## Implementation Notes

### Design decisions

**`Probe` added to the `Backend` interface** (not just the concrete S3 type).
All three backends (S3, GCS, Azure Blob) are used in clustered mode; putting
`Probe` on the interface keeps the startup gate backend-agnostic and gives all
implementations a consistent liveness contract.

**Primitive choice per backend:**
- S3 / S3-compatible: `HeadBucket` — the canonical AWS liveness op; returns
  immediately on success, times out on unreachable endpoint, returns 404 on
  missing bucket. Well-supported on MinIO and Cloudflare R2.
- GCS: `Bucket.Attrs` — fetches bucket metadata; equivalent liveness guarantee.
- Azure Blob: `container.Client.GetProperties` — fetches container properties;
  returns fast on success, errors on unreachable or missing container.

**Call site in `cmd/portal/main.go`:** inserted immediately after the
`slog.Info("object storage backend initialised", ...)` line and before
`objSyncer` construction. The probe is naturally gated on the surrounding
`if cfg.DeployMode == "clustered" && cfg.ObjectStorageURL != ""` block, so it
never runs in single-instance mode.

**5-second timeout:** passed via `context.WithTimeout` at the call site; the
`Probe` method itself uses the caller-supplied context so the deadline is
visible to the SDK's retry logic. On success the probe returns in < 100 ms.

### Files changed

- `internal/portal/storage/objectstore/backend.go` — added `Probe(ctx) error` to `Backend` interface
- `internal/portal/storage/objectstore/s3.go` — `(*s3Backend).Probe` via `HeadBucket`
- `internal/portal/storage/objectstore/azure.go` — `(*azureBlobBackend).Probe` via `GetProperties`
- `internal/portal/storage/objectstore/gcs.go` — `(*gcsBackend).Probe` via `Bucket.Attrs`
- `internal/portal/storage/objectstore/manifest_test.go` — `(*memBackend).Probe` (always nil)
- `internal/portal/storage/objectstore/lifecycle_test.go` — `(*errBackend).Probe` (returns err)
- `internal/portal/storage/objectstore/sync_test.go` — `(*blockingBackend).Probe` (delegates to inner)
- `cmd/portal/main.go` — 5s-timeout probe call before HTTP listener starts
- `tests/e2e/failure/object_storage_unreachable_at_startup_test.go` — removed `t.Skip`; failure is now `t.Fatalf`

### Verification

`go build ./...` and `go vet ./...` clean.
`go test ./internal/portal/... -timeout 90s` — all 28 packages pass.
`go test ./cmd/portal/... -timeout 60s` — passes.
