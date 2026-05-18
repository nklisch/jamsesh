---
id: gate-tests-receive-pack-concurrent-semaphore
kind: story
stage: implementing
tags: [testing, security, portal]
parent: null
depends_on: [gate-security-receive-pack-stream-body]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# Receive-pack body buffered fully into memory — no memory-bound concurrency test

## Priority
High

## Spec reference
Item: `gate-security-receive-pack-stream-body`
Acceptance criterion: stream the body to a tempfile and add a
per-instance semaphore counting concurrent receive-pack handlers so
overall memory is bounded.

## Gap type
missing test for adversarial-spec-silent (concurrent pushes RSS). The
spec's "per-instance semaphore counting concurrent receive-pack
handlers" is wholly untested.

## Suggested test
```go
// TestReceivePack_ConcurrentPushSemaphore_BoundsConcurrency
//   Fire N=10 parallel receive-pack handlers with N > semaphore cap.
//   Assert: at most `cap` handlers run buildValidationRepo concurrently;
//   remaining return 503 or block (per spec) — not 200 with N*pack RSS spike.
```

## Test location (suggested)
`internal/portal/githttp/receive_pack_test.go`
