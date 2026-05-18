---
id: gate-tests-router-discovery-shutdown
kind: story
stage: backlog
tags: [testing, infra]
parent: null
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# `bug-router-static-discoverer-not-started` fix has no test asserting goroutine lifecycle on shutdown

## Priority
Low

## Spec reference
Item: `bug-router-static-discoverer-not-started` (archived).
Acceptance criterion: the signal context (`ctx`) was moved earlier in
`run()` so it is defined when the goroutine is launched.

## Gap type
missing test for adversarial (shutdown ordering). No test asserts the
discovery goroutine exits cleanly on SIGTERM (no leaked goroutines, no
`context.Canceled` propagated as an error log).

## Suggested test
```go
// cmd/jamsesh-router/main_test.go — TestRun_DiscoveryGoroutineExitsOnContextCancel
//   Build the static-mode config, spawn run() in goroutine, cancel ctx.
//   Assert: run() returns within shutdown deadline, no goroutine leak (use goleak),
//   no error logged for context.Canceled (only unexpected errors should log).
```

## Test location (suggested)
`cmd/jamsesh-router/main_test.go`
