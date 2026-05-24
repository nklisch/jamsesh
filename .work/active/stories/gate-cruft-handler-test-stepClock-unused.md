---
id: gate-cruft-handler-test-stepClock-unused
kind: story
stage: review
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# Defined-but-unused test helper type `stepClock`

## Confidence
High

## Category
dead function

## Location
`internal/portal/playground/handler_test.go:32-45`

## Evidence
```go
// stepClock advances by step every time Now() is called. Used only by tests
// that need a clock value to change between two consecutive reads ...
type stepClock struct {
    t    time.Time
    step time.Duration
}
func (c *stepClock) Now() time.Time {
    now := c.t
    c.t = c.t.Add(c.step)
    return now
}
```

## Removal
`deadcode -test` flags `stepClock.Now` as unreachable. `grep -n stepClock` returns only the type decl, the method decl, and the comment — no construction or usage anywhere in the repo. Delete the type, its method, and the explanatory comment block; `fixedClock` remains and covers all current tests.

## Implementation notes

Deleted unused `stepClock` type, its `Now()` method, and the comment block from `internal/portal/playground/handler_test.go:32-45`. `fixedClock` covers all current tests.

Verified: `go build ./...` clean. Affected Go tests pass (`go test ./internal/portal/playground/... ./internal/portal/storage/objectstore/...`) excluding the pre-existing `TestJoinPlaygroundSession_WithNickname_UsesIt` failure on `main` (parked as `bug-playground-join-with-nickname-returns-410-on-fresh-session`). Frontend tests pass for the two touched files (`vitest run`).
