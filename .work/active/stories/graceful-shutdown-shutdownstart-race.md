---
id: graceful-shutdown-shutdownstart-race
kind: story
stage: implementing
tags: [bug, portal, infra]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Graceful shutdown: shutdownStart write/read race

## Brief

The graceful-shutdown drain in `cmd/portal/main.go` uses an unsynchronized
`shutdownStart time.Time` variable shared between two goroutines: a writer
goroutine that sets it on `<-ctx.Done()`, and the main goroutine that reads
it after `server.Run` returns. The in-code comment claims "no mutex needed
because server.Run blocks until ctx is cancelled, creating the necessary
happens-before relationship" — this reasoning is incorrect under the Go
memory model.

`ctx.Done()` close creates an HB edge from cancellation to each independent
observer, NOT between observers. There is no synchronization edge between
the writer's `shutdownStart = time.Now()` and the reader's
`if !shutdownStart.IsZero()` check.

## Why it's benign in practice (but should still be fixed)

- The writer fires within microseconds of ctx cancellation (just `time.Now()`
  + struct assign + `sync.Once.Do` machinery).
- The reader runs after HTTP draining, which takes milliseconds to seconds.
- On x86/arm64 with current Go compiler, `time.Time` reads are typically
  not torn at typical alignment.
- Worst case (stale-zero observed by reader): drain block skipped entirely,
  auto-merger and WS gateway don't get their Stop() calls — no data corruption
  but the cleanup leak is unprincipled.

## Why it should still be fixed

- `go test -race` would flag this if a test exercised the main.go path.
- Heavily-loaded or non-x86 systems could see torn `time.Time` reads with
  surprising behavior.
- The in-code comment is incorrect and misleading future maintainers.

## Fix sketch

Replace the bare variable + `sync.Once` with a `chan time.Time`:

```go
shutdownStartCh := make(chan time.Time, 1)
go func() {
    <-ctx.Done()
    shutdownStartCh <- time.Now()
}()

// ... server.Run blocks ...

// After server.Run returns:
select {
case shutdownStart := <-shutdownStartCh:
    // graceful path — use shutdownStart
    httpElapsed := time.Since(shutdownStart)
    // ... rest of drain block
default:
    // listen-error path — no drain needed
}
```

The channel receive provides the necessary HB edge. The `default` branch
correctly handles the listen-error path (where ctx was never cancelled).

Alternative: `atomic.Pointer[time.Time]`.

## Acceptance criteria

- [ ] `cmd/portal/main.go` no longer has the race documented above.
- [ ] In-code comment updated to reflect the corrected synchronization.
- [ ] `go test -race ./cmd/portal/...` passes if a test exercises this path
  (add one if none exists).
- [ ] Existing graceful-shutdown integration tests continue to pass.

## Notes

Surfaced in review of
`epic-cloud-native-deploy-operational-polish-graceful-shutdown` (commit
336b38e). Not blocking — the implementation works correctly in practice
under typical loads — but the formal-correctness gap deserves a clean fix.
