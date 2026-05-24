---
id: gate-tests-comments-slog-warning-emission-assertion
kind: story
stage: drafting
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
