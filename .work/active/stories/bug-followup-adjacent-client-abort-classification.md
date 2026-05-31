---
id: bug-followup-adjacent-client-abort-classification
kind: story
stage: done
tags: [bug, portal, error-handling]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
bug_origin: gate
bug_severity: low
bug_domain: error-handling
bug_location: internal/portal/auth
---

# Adjacent client-abort / body-read error misclassifications

Surfaced by the codex final-gate review of `epic-bug-squash` (out of scope —
adjacent to the `git-auth-client-abort-500` fix). Two more handler
error-classification gaps of the same family that `epic-bug-squash-handler-error-classification`
did NOT cover: (1) bearer-auth token validation that is cancelled by client
disconnect can be reported as a dependency 503 rather than a client-abort; and
(2) git receive-pack request-body read errors can surface as a 413 when they are
really client aborts. Apply the same `r.Context().Err()`-based client-abort
discrimination (499 / no-5xx) used in `bug-squash-git-auth-client-abort-500`.
Low severity (alert-hygiene), but completes the client-abort classification
sweep.

## Implementation notes

Mirrored `bug-squash-git-auth-client-abort-500` (commit 05f0d88) for two
adjacent spots.

**(a) BearerMiddleware — `internal/portal/tokens/middleware.go`**

Added `const statusClientClosedRequest = 499` (local copy; import-cycle prevents
reusing the githttp one). In the `default:` branch of the `switch` on
`svc.Validate` error, gate on `r.Context().Err() != nil`: cancelled ctx →
`w.WriteHeader(499)` + return; live ctx → existing `httperr.WriteFromError` with
`deperr.WrapDBIfTransient` (503 unchanged).

Tests: `internal/portal/tokens/bearer_client_abort_test.go`
- `TestBearerMiddleware_CancelledContext_Returns499`: cancelled ctx → 499 (was 503)
- `TestBearerMiddleware_LiveCtx_StoreError_Still503`: live ctx + store error → 503 unchanged

**(b) receive_pack body-read — `internal/portal/githttp/receive_pack.go`**

After `io.Copy(bodyFile, limitedBody)` returns an error, gate on
`r.Context().Err() != nil`: cancelled ctx → `writeClientAbort(w)` (499); live
ctx → `http.Error(w, "pack exceeds size limit", 413)` unchanged.

Tests: `internal/portal/githttp/receive_pack_body_abort_test.go`
- `TestReceivePack_BodyReadError_CancelledContext_Returns499`: body reader cancels
  ctx on first Read → handler returns 499 (was 413)
- `TestReceivePack_BodyReadError_LiveContext_Returns413`: oversized body on live
  ctx → 413 unchanged
