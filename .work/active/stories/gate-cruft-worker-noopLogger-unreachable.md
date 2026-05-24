---
id: gate-cruft-worker-noopLogger-unreachable
kind: story
stage: implementing
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# Unreachable `noopLogger` in production source — duplicate exists in test package and is the one actually used

## Confidence
High

## Category
dead function

## Location
`internal/portal/playground/worker.go:136-140`

## Evidence
```go
// noopLogger returns a slog.Logger that discards all output. Used by tests
// that don't want log noise.
func noopLogger() *slog.Logger {
    return slog.New(slog.DiscardHandler)
}
```

## Removal
`worker.go` is `package playground`. The three test files that call `noopLogger()` (`worker_test.go`, `destruction_test.go`, `handler_test.go`) are all `package playground_test` and therefore cannot access the unexported function in `worker.go`; they resolve to the duplicate `noopLogger` defined in `handler_test.go:334`. `deadcode -test` confirms `worker.go:138` is unreachable. Delete the function and its preceding comment block; the production package has no callers.
