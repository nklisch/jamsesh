---
id: object-storage-write-rejected-silent-acceptance
kind: story
stage: implementing
tags: [bug, portal, object-storage, durability]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Bug: Portal Silently Accepts Git Push When S3 Bucket Does Not Exist (RPO=0 Violation)

## Summary

When the portal is configured with `JAMSESH_DEPLOY_MODE=clustered` and an
`JAMSESH_OBJECT_STORAGE_URL` that references a bucket that does not exist on
MinIO (or any S3-compatible backend), a git push that triggers a `PutObject`
call may return 2xx to the client while the object is silently lost. This
violates the RPO=0 durability invariant: the portal must not acknowledge a
write it cannot persist.

## Root Cause

The AWS SDK v2 `PutObject` call returns a `NoSuchBucket` error. If the
pack-storage layer or receive-pack handler swallows this error (e.g., logs it
but does not propagate it to the HTTP response writer), the git client sees
`200 OK` on the smart-HTTP `POST /git/…/git-receive-pack` endpoint while the
pack data is never written to S3.

## Invariant Violated

RPO=0: every acknowledged write must be durable. A push that returns 2xx must
have persisted the pack object to object storage before responding. Silent
acceptance of a failed `PutObject` is a critical durability bug.

## Reproduction

E2E test `TestObjectStorageWriteRejected` in
`tests/e2e/failure/object_storage_write_rejected_test.go` exercises this path
(PATH B: portal boots with lazy AWS S3 client, then a git push triggers the
first `PutObject`). If the push returns exit 0 (2xx), the test skips with this
item's ID.

## Fix

Ensure `NoSuchBucket` (and all other S3 write errors) from `PutObject` in the
pack-storage layer are propagated as non-2xx responses on the smart-HTTP
receive-pack endpoint. Specifically:

1. The receive-pack handler must check the error returned by the object-storage
   backend's `Write` or `Put` method.
2. If the error is non-nil, the handler must abort the response with HTTP 500
   (or a more specific error code) and log the error with enough context to
   identify the session and pack.
3. The git client must receive a non-zero exit from `git push`.

A regression test already exists: once this fix is in place, remove the
`t.Skip` branch from `TestObjectStorageWriteRejected` PATH B (the skip is the
audit trail; the working test is the proof).
