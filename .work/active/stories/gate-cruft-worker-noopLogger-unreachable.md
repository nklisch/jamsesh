---
id: gate-cruft-worker-noopLogger-unreachable
kind: story
stage: done
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

## Implementation notes

Deleted unreachable `noopLogger` func + comment from `internal/portal/playground/worker.go:136-140`. The `slog` import is still used for `slog.Logger` field type and `slog.Default()` fallback, so the import stays.

Verified: `go build ./...` clean. Affected Go tests pass (`go test ./internal/portal/playground/... ./internal/portal/storage/objectstore/...`) excluding the pre-existing `TestJoinPlaygroundSession_WithNickname_UsesIt` failure on `main` (parked as `bug-playground-join-with-nickname-returns-410-on-fresh-session`). Frontend tests pass for the two touched files (`vitest run`).
