---
id: bug-scan-automerger-swallows-merge-emit
created: 2026-05-30
tags: [bug, error-handling, high]
bug_origin: scan
bug_severity: high
bug_domain: error-handling
bug_location: internal/portal/automerger/outcomes.go:155
---

# Auto-merger advances the draft ref durably, then swallows a failed merge.succeeded emit

**Location**: `internal/portal/automerger/outcomes.go:155` · **Severity**: high · **Pattern**: confirm-after-commit with no rollback / partial mutation then error dropped

The draft ref is moved on disk (`SetReference`, a durable git write) before the `merge.succeeded` event is logged. If `Emit` fails (transient DB error), `Apply` returns an error that the worker (`worker.go:308`) only `slog.WarnContext`-logs and drops — there is no retry (`replayScan` is a documented no-op). Result: the draft branch has silently advanced but no `merge.succeeded` exists in the log or on the WebSocket, so every client's view of the draft tip stays stale until an unrelated future push re-triggers the worker; the git system-of-record and event log diverge with no compensation. Fix: emit before advancing the ref, or make the post-advance emit retryable/idempotent (outbox), or re-derive missing events on worker startup.

```go
if err := in.Repo.Storer.SetReference(ref); err != nil { ... }   // durable git write
if _, err := a.Log.Emit(ctx, ..., "merge.succeeded", data); err != nil {
    return ApplyOutput{}, fmt.Errorf("automerger apply: emit merge.succeeded: %w", err)
}
```
