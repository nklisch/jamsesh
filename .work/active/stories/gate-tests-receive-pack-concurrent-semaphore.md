---
id: gate-tests-receive-pack-concurrent-semaphore
kind: story
stage: done
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

## Implementation notes

### Test: `TestReceivePack_ConcurrentPushSemaphore_BoundsConcurrency`
Location: `internal/portal/githttp/receive_pack_test.go`

**Contention pattern**: `io.Pipe` bodies (not `semBlockReader`). Each of the
5 concurrent request goroutines uses an `io.Pipe`; the write end is held by
the test. The server's `io.Copy(bodyFile, r.Body)` blocks waiting for bytes
from the client, which keeps the semaphore slot held. Once the server is
blocked, any additional request hits the `default:` branch in the semaphore
select and returns 503 immediately.

**Why `io.Pipe`, not a client-side blocking reader**: an initial approach used
a custom `semBlockReader` that signalled and then blocked inside `Read()`.
This failed because Go's HTTP client transport reads the body to SEND it to
the server; the server handler runs asynchronously in a separate goroutine. By
the time the body reader blocks on the client side, the server goroutines
had already processed and returned (empty body → 400). `io.Pipe` creates a
synchronous pipe where the server's `r.Body.Read()` blocks until the client
calls `pw.Write()` or `pw.Close()`, correctly holding the semaphore.

**Why file-based SQLite**: concurrent requests from 5 goroutines hitting the
`:memory:` SQLite caused spurious 500 errors in the `requireSessionMember`
middleware. A file-based DB with WAL mode enabled handles concurrent reads
correctly. `busy_timeout(5000)` is injected automatically by `db.Open`.

**Race detector**: test passes with `-race`.

**Semaphore leak finding**: no leak detected. The `defer func() { <-h.ReceivePackSem }()`
release is inside the semaphore acquire branch; it runs even on all
subsequent error paths (400, 413, 500). The race detector did not flag any
concurrent semaphore access. No blocking issued.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Concurrency test verifies cap=2 admits 2, rejects 3 with 503+Retry-After. Solid engineering: uses io.Pipe (not a client-side blocking reader) to produce server-side contention, since Go's HTTP client transport reads the body before server processing. File-based SQLite + WAL to handle concurrent middleware reads; DisableKeepAlives:true so each request hits a separate server goroutine. -race clean. No semaphore leak observed — defer release runs on all error paths (400/413/500).
