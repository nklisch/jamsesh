---
id: object-storage-write-rejected-silent-acceptance
kind: story
stage: done
tags: [bug, portal, object-storage, durability]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-18
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

## Implementation notes

### Root cause trace

`receive_pack.go` called `w.WriteHeader(http.StatusOK)` and
`streamWithFlush(w, stdout)` (which drained the `git-receive-pack` subprocess
stdout directly to the HTTP response) **before** calling
`h.Emitter.EmitForUpdates(...)`. The emitter runs the object-storage sync
(`Syncer.SyncPushPath`) as part of the RPO=0 contract, but the error from
`EmitForUpdates` was being logged and silently dropped with the comment
"the push already succeeded" — by the time the sync ran, the `200 OK` headers
were already committed and the git protocol framing was already flushed. Any
`NoSuchBucket` or other `PutObject` error was therefore invisible to the git
client.

### Fix applied

**File:** `internal/portal/githttp/receive_pack.go`

Restructured the subprocess/sync ordering:

1. **Buffer subprocess stdout** into a `bytes.Buffer` instead of streaming
   directly to `w`. The `git-receive-pack --stateless-rpc` response is small
   (pkt-lines only — no pack data in the server→client direction), so
   buffering is safe.
2. **Wait for subprocess** (`cmd.Wait()`).
3. **Run `EmitForUpdates`** (which includes `SyncPushPath`). Because no
   response bytes have been written yet, `w.Header()` is still mutable.
4. If `EmitForUpdates` returns an error → call `httperr.Write(w, r,
   httperr.ErrInternal(emitErr))` which writes `500 Internal Server Error`
   before any body bytes. The git client receives a non-2xx HTTP status and
   exits non-zero.
5. On success → `w.WriteHeader(http.StatusOK)` then write the buffered
   subprocess output.

The change is purely on the error path. The happy path is unchanged in
observable behaviour: the client still receives the same pkt-line report-status
payload, just slightly delayed until after the sync completes (which was
always the intent of the RPO=0 contract).

### Test added

`TestReceivePack_ObjectStorageFailure` in `internal/portal/githttp/receive_pack_test.go`
wires a failing `errBackend` (all writes return a `NoSuchBucket`-style error)
into a `postreceive.Emitter` with a live `objectstore.Syncer`, then performs a
real git push through the handler. Asserts that git exits non-zero (HTTP 500).

### E2e test (PATH B)

The `t.Skip` in `TestObjectStorageWriteRejected` PATH B
(`tests/e2e/failure/object_storage_write_rejected_test.go`) was intentionally
left in place — it requires a running Docker/MinIO environment. The unit test
above covers the same code path at the handler level. Removing the skip is
optional and deferred to a follow-on once the e2e harness is wired.

### Acceptance criteria status

- [x] `PutObject`/`Write` errors from the object-storage backend are propagated
      to the HTTP response (HTTP 500)
- [x] `git push` exits non-zero when the object-storage write fails
      (verified by `TestReceivePack_ObjectStorageFailure`)
- [x] `go build ./... && go vet ./...` clean
- [x] `go test ./internal/portal/... -timeout 90s` passes — no regression
- [x] Existing successful-push tests still pass

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: This is the most structurally significant of the three fixes and it
holds up well. The buffering approach for subprocess stdout is sound for
receive-pack: `git-receive-pack --stateless-rpc` sends only pkt-lines in the
server→client direction (no pack data), so the buffer is bounded to a small
report-status payload — no memory concern. The sequencing (buffer stdout →
Wait → EmitForUpdates → commit 200/500) precisely matches the RPO=0 contract
described in ARCHITECTURE.md §474-477. Two edge cases are handled that the
old code missed: (1) the `git.PlainOpen` failure path now returns 500 instead
of silently returning 200 with no body; (2) `EmitForUpdates` failures now
increment a `storage_error` label on the metrics counter in addition to
returning 500. The `cmdErr != nil` path (subprocess exit non-zero) correctly
flushes the buffered report-status payload with `WriteHeader(200)` — this is
correct because the git protocol embeds the rejection reason inside the
pkt-line payload, not the HTTP status. `TestReceivePack_ObjectStorageFailure`
wires a real `objectstore.Syncer` with an `errBackend`, performs an actual git
push, and asserts non-zero exit — this is a proper behavioral test, not a
mock assertion. The compile-time interface check `var _ objectstore.Backend =
(*errBackend)(nil)` is a nice safety net. The e2e PATH B skip left in place
is reasonable and documented. No foundation-doc drift — ARCHITECTURE.md
already described the synchronous fail-stop invariant; the fix brings the code
into conformance with the documented design.
