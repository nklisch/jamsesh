---
id: gate-tests-automerger-detached-emit-context
kind: story
stage: implementing
tags: [testing]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: tests
created: 2026-05-31
updated: 2026-05-31
---

# Auto-merger detached-context emit path is not covered

## Priority
Critical

## Spec reference
Item: `epic-bug-squash-automerger-correctness`
Acceptance criterion: "Worker-ctx cancellation after the durable write still lets the emit attempt run (detached ctx), within `emitGraceTimeout`."

## Gap type
missing test for cancellation/state transition

## Suggested test
```go
// Cancel the worker context after the durable merge side effect and before
// event emit; assert emit still runs via detached context within timeout.
```

## Test location (suggested)
`internal/portal/automerger/emit_retry_test.go`

