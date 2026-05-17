---
id: epic-cloud-native-deploy-object-storage-sync-backend
kind: story
stage: implementing
tags: [portal]
parent: epic-cloud-native-deploy-object-storage-sync
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Object-Storage Sync — Backend interface + S3-compat impl

## Scope

Define the `Backend` interface in `internal/portal/storage/objectstore/`
and ship the S3-compat implementation using `aws-sdk-go-v2/service/s3`.
Covers AWS S3, Cloudflare R2, Backblaze B2, MinIO, and self-hosted Ceph.

Implements **Unit 1** of `epic-cloud-native-deploy-object-storage-sync`.
See parent feature body for interface signature, error sentinels, and
acceptance criteria.

## Files

New:
- `internal/portal/storage/objectstore/backend.go` — interface +
  `ErrNotFound`, `ErrPrecondition`, `ErrAlreadyExists`
- `internal/portal/storage/objectstore/s3.go` — S3-compat impl
- `internal/portal/storage/objectstore/s3_test.go` — integration tests
  against MinIO testcontainer OR gated on `JAMSESH_TEST_S3_*` env vars

## Acceptance criteria

- [ ] `Put` with empty `ifMatch` succeeds; returns ETag
- [ ] `Put` with stale `ifMatch` returns `ErrPrecondition`
- [ ] `PutIdempotent` succeeds on first write; same-content rewrite
  succeeds; different-content rewrite returns `ErrAlreadyExists`
- [ ] `Get` returns data + ETag + fencing token from metadata
- [ ] `Get` on missing key returns `ErrNotFound`
- [ ] `Delete` is idempotent (no error on missing)
- [ ] `List` yields all keys under prefix; callback error stops iteration
- [ ] Integration tests gated on `JAMSESH_TEST_S3_*` env vars; skip cleanly
- [ ] Tests run against at least MinIO (via testcontainer or local)

## Notes

- Pin `github.com/aws/aws-sdk-go-v2@v1` and `aws/config`, `aws/credentials`,
  `service/s3` subpackages. Avoid pulling unrelated SDK subpackages —
  estimated +8-12MB binary growth.
- Fencing token storage: `s3.PutObjectInput.Metadata` map.
- Conditional write: `s3.PutObjectInput.IfMatch`.
- `PutIdempotent` strategy: try `IfNoneMatch: "*"` (create-only); on 412,
  HEAD + content compare; return nil if equal, `ErrAlreadyExists` if differ.
- Endpoint override for non-AWS providers: `config.WithBaseEndpoint(...)`
  + `s3.Options.UsePathStyle = true` for MinIO/Ceph.
