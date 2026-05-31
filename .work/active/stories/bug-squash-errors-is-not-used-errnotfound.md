---
id: bug-squash-errors-is-not-used-errnotfound
kind: story
stage: done
tags: [bug, portal, error-handling]
parent: epic-bug-squash-automerger-correctness
depends_on: [bug-squash-automerger-strands-commit-event, bug-squash-automerger-swallows-merge-emit]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: low
bug_domain: error-handling
bug_location: internal/portal/automerger/worker.go:338
---

# Sentinel comparison via ==/!= instead of errors.Is for store.ErrNotFound

**Location**: `internal/portal/automerger/worker.go:338` (also `internal/portal/automerger/outcomes.go:234`) · **Severity**: low · **Pattern**: comparing errors without unwrapping

These checks work today only because the dialect adapters return the bare `ErrNotFound` sentinel unwrapped. The moment any layer wraps the store error with `%w` (as the rest of the codebase routinely does), `err == store.ErrNotFound` becomes false: `worker.go` would treat a normal missing ref-mode as a hard error and abort the merge; `outcomes.go` would treat a missing conflict event as a real failure instead of a silent no-op. Inconsistent with every other not-found check in the portal. Fix: use `errors.Is(err, store.ErrNotFound)` in both places.

```go
if err != store.ErrNotFound { return "", fmt.Errorf("get ref mode: %w", err) }  // worker.go
if err == store.ErrNotFound { return nil }                                       // outcomes.go
```

## Implementation notes

Two one-line fixes:
1. `worker.go` `refModeForSession` — `if err != store.ErrNotFound` → `if !errors.Is(err, store.ErrNotFound)`.
2. `outcomes.go` `tryResolveConflict` — `if err == store.ErrNotFound` → `if errors.Is(err, store.ErrNotFound)`.

Both files already imported `errors`; no new imports needed. The `errors` import
was added to `worker.go` as part of Unit 1 (needed for `isEmitAfterSideEffect`).

Tests added in `errnotfound_test.go` (package `automerger_test`):
- `TestWorker_RefMode_WrappedErrNotFound_FallsBackToDefault` — wraps ErrNotFound
  in GetRefMode; worker falls back to DefaultMode="sync" and produces merge.succeeded
  (not a hard error abort).
- `TestApply_TryResolveConflict_WrappedErrNotFound_IsNoOp` — wraps ErrNotFound
  in GetConflictEventByID; Apply returns nil (silent no-op, not an error).
