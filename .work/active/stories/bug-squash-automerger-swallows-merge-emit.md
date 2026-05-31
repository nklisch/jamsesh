---
id: bug-squash-automerger-swallows-merge-emit
kind: story
stage: done
tags: [bug, portal, error-handling, high]
parent: epic-bug-squash-automerger-correctness
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
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

## Implementation notes

Added `ErrEmitAfterSideEffect` sentinel and `emitWithRetry` to `outcomes.go`.
`emitWithRetry` runs on a detached `context.WithoutCancel(ctx)+emitGraceTimeout` ctx
so shutdown cancellation cannot re-drop the event. Classifies transience via
`deperr.WrapDBIfTransient` (because `events.Log.Emit` returns raw store errors,
not pre-wrapped deperr errors) before deciding to retry. Retries up to
`emitMaxRetries=3` times; on exhaustion returns `ErrEmitAfterSideEffect` wrapping
the last error.

Applied to all three emit sites: `merge.succeeded` (`applySuccess`),
`conflict.detected` (`applyConflict`), and `conflict.resolved` (`tryResolveConflict`).

`worker.go processEvent`: when `Apply` returns `ErrEmitAfterSideEffect`,
`slog.ErrorContext` (not Warn) with session_id + sha + recovery hint, and
increments `AutoMergerOutcomes{outcome="emit_failed"}`.

Duplicate emit on ambiguous commit-phase failure is tolerated (consumers keyed
on `merge_commit_sha` / event `id` are idempotent) — no dedup store added.

Tests added in `emit_retry_test.go`:
- `TestApply_EmitRetry_TransientSucceedsOnRetry` — failN store, 2 fails then
  success; Apply returns nil, draft ref advanced, merge.succeeded in log.
- `TestApply_EmitRetry_AlwaysFailEscalates` — alwaysFail store; Apply returns
  ErrEmitAfterSideEffect, draft ref still advanced (side effect committed).
- `TestApply_EmitRetry_ConflictDetected_AlwaysFailEscalates` — same for conflict path.
- `TestApply_DuplicateEmit_ConsumerIdempotent` — two merge.succeeded events for same
  merge_commit_sha both carry identical sha; consumer is idempotent.
- `TestApply_EmitRetry_ConflictResolved_AlwaysFailEscalates` — conflict.resolved path.
