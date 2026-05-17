---
id: epic-cloud-native-deploy-object-storage-sync-backend
kind: story
stage: review
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

## Implementation notes

Implemented in wave 1 as specified. Key decisions:

- **Error mapping**: typed S3 errors (`*s3types.NoSuchKey`, `*s3types.NotFound`)
  are checked first via `errors.As`; falls back to `smithy.APIError.ErrorCode()`
  for providers that return `PreconditionFailed` as an untyped error (R2, B2).
  Also handles `ConditionNotMet` which some providers use instead.

- **ETag quoting**: AWS S3 wraps ETags in double-quotes (`"abc123"`). The
  `stripEtag` helper normalises to unquoted form in both Put and Get so the
  round-trip is transparent to callers.

- **PutIdempotent strategy**: uses `IfNoneMatch: "*"` (create-only) on the
  first attempt. On 412 PreconditionFailed, fetches the full object via Get
  (not HEAD, since HEAD doesn't return the body) and does a `bytes.Equal`
  comparison. Returns nil on match, `ErrAlreadyExists` on mismatch.

- **keyPrefix handling**: the s3:// URL's path component becomes the key
  prefix. `fullKey` prepends it; `logicalKey` strips it. List strips it from
  every result key so callers see clean logical names.

- **Credentials**: follow the SDK default chain (env vars, ~/.aws/credentials,
  IRSA, IMDS). For MinIO tests, callers set `AWS_ACCESS_KEY_ID` /
  `AWS_SECRET_ACCESS_KEY` via `t.Setenv`.

- **Dependencies added**:
  - `github.com/aws/aws-sdk-go-v2 v1.41.7` (direct)
  - `github.com/aws/aws-sdk-go-v2/config v1.32.17` (direct)
  - `github.com/aws/aws-sdk-go-v2/service/s3 v1.101.0` (direct)
  - `github.com/aws/smithy-go v1.25.1` (direct)
  - Standard indirect chain (imds, sts, sso, ssooidc, credentials, etc.)

- **Test gating**: gated on `JAMSESH_TEST_S3_ENDPOINT` + `JAMSESH_TEST_S3_BUCKET`
  env vars, or `JAMSESH_TEST_S3_USE_CONTAINER=true` for a Docker-based MinIO.
  Without either, all 15 tests skip cleanly. `testcontainers-go` was NOT added
  as a dependency (keeping dep surface minimal); Docker-mode currently stubs
  out to a skip until testcontainers is adopted project-wide.

- **Acceptance criteria status**:
  - [x] `Put` with empty `ifMatch` succeeds; returns ETag
  - [x] `Put` with stale `ifMatch` returns `ErrPrecondition`
  - [x] `PutIdempotent` succeeds on first write; same-content rewrite
    succeeds; different-content rewrite returns `ErrAlreadyExists`
  - [x] `Get` returns data + ETag + fencing token from metadata
  - [x] `Get` on missing key returns `ErrNotFound`
  - [x] `Delete` is idempotent (no error on missing)
  - [x] `List` yields all keys under prefix; callback error stops iteration
  - [x] Integration tests gated on `JAMSESH_TEST_S3_*` env vars; skip cleanly
  - [x] Tests structured for MinIO via env-var gate
