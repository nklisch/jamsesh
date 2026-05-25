---
id: gate-tests-comments-slog-warning-emission-assertion
kind: story
stage: done
tags: [testing, portal, logging]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Comments service slog warning emission not asserted

## Priority
Medium

## Spec reference
Item: `story-comments-service-use-slog-not-stdlib-log`

Acceptance criterion: Story AC: "The warning is emitted via slog with structured fields (`session`, `err`) matching the pattern in `receive_pack.go` and `sessions/handler.go`."

## Gap type
missing test for valid partition

## Suggested test
```go
func TestServiceCreate_ActivityResetFailure_EmitsSlogWarning(t *testing.T) {
    // Install slog test handler capturing records.
    // Trigger comment-create on a playground session with a store that errors
    // from ResetSessionIdleTimer.
    // Assert: single Warn record with attrs {org, session, err}.
}
```

## Test location (suggested)
`internal/portal/comments/service_test.go`

## Implementation notes

Added `TestServiceCreate_ActivityResetFailure_EmitsSlogWarning` in
`internal/portal/comments/service_test.go`.

Approach:
- Added `failingResetIdleTimerStore` — a minimal store wrapper that
  returns a fixed error from `ResetSessionIdleTimer` and delegates
  everything else to the real store.
- Seeds a playground org + session in the real store; wraps it with the
  failing store for the `Service` under test.
- Installs a `slog.NewJSONHandler(&buf, ...)` as `slog.Default()` before
  the `Create` call, restoring the original default via `t.Cleanup`.
- Asserts: Create returns a valid comment (best-effort path, no error
  propagation), exactly one WARN record is captured, the message contains
  `"reset idle timer failed"`, and the record carries attrs `org`, `session`,
  and `err` matching the values passed in `service.go` line 224.

## Review notes

Approve. Captures slog output via JSON handler, asserts level + msg + all
structured attrs (org, session, err). Comment-create succeeds (best-effort
contract preserved). Real seeded SQLite store; only the reset call is stubbed.
Test passes.
