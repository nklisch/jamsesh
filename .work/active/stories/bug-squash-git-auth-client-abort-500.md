---
id: bug-squash-git-auth-client-abort-500
kind: story
stage: drafting
tags: [bug, portal, error-handling]
parent: epic-bug-squash
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
```
