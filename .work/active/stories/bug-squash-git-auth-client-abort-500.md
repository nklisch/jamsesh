---
id: bug-squash-git-auth-client-abort-500
kind: story
stage: done
tags: [bug, portal, error-handling]
parent: epic-bug-squash-handler-error-classification
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: low
bug_domain: error-handling
bug_location: internal/portal/githttp/auth.go:47
---

# Git smart-HTTP auth middleware classifies client-abort (context cancellation) as 500

**Location**: `internal/portal/githttp/auth.go:47` (also `:86`, `:110`) · **Severity**: low · **Pattern**: context.Canceled/DeadlineExceeded misclassified as server error

The default branch maps any non-auth store error — including `context.Canceled`/`DeadlineExceeded` from a git client that hung up mid-handshake — to HTTP 500. Git clients abort frequently, so this inflates 5xx rates/alerts and emits misleading error logs for what are actually client disconnects. Same default-500 applies in `requireSessionMember` and `checkArchived`. Fix: before the default 500, detect `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` (and request-context cancellation) and respond with a client-abort status without logging an error.

```go
default:
    http.Error(w, "internal server error", http.StatusInternalServerError)  // also catches ctx.Canceled

## Implementation notes

Added `const statusClientClosedRequest = 499` and `writeClientAbort(w)` helper
in `auth.go`. In all three middleware default branches (`basicAuth`,
`requireSessionMember`, `checkArchived`), gate on `r.Context().Err() != nil`:
if true → `writeClientAbort(w)` (499, no ERROR log); else → 500 unchanged.

Discriminates on the REQUEST context (not on `errors.Is(err, context.DeadlineExceeded)`)
so store/dep timeouts on a live request context remain 5xx. Writing 499 matters
for access-log accuracy (without it, logging middleware records 200).

Tests added in `auth_client_abort_test.go`:
- `abortStore` stub implements the `githttpStore` interface, delegating all
  methods except `GetSessionMember` to the real store
- `TestRequireSessionMember_ContextCancelled_Returns499` — pre-cancelled request
  ctx + stub returning context.Canceled → not 500 (expects 499)
- `TestRequireSessionMember_StoreError_Returns500` — genuine store error on live
  ctx → 500 (unchanged behavior)
- `TestBasicAuth_CancelledContext_Returns499NotServerError` — pre-cancelled ctx
  in basicAuth default branch → not 500
```
