---
id: gate-cruft-sessions-handler-test-fakeclock-advance-unused
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

# handler_test.go: sessionsFakeClock.advance method has no callers

## Confidence
High

## Category
dead function

## Location
`internal/portal/sessions/handler_test.go:790`

## Evidence
```go
type sessionsFakeClock struct{ t time.Time }

func (c *sessionsFakeClock) Now() time.Time          { return c.t }
func (c *sessionsFakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }
```

`deadcode -test ./internal/portal/sessions/...` reports `(*sessionsFakeClock).advance` unreachable. `grep -n '\.advance(' internal/portal/sessions/handler_test.go` returns nothing — the integration tests never advance the clock; they construct a fresh `sessionsFakeClock` per test instead. The `advance` method was added speculatively but never wired into a test case.

## Removal
Delete the `advance` method definition (line 790). The struct and its `Now()` method remain — they're used to inject a fixed time into handler tests. After removing, `go vet ./internal/portal/sessions/...` should pass; no other edits required.

## Implementation notes
Deleted `(*sessionsFakeClock).advance` from `internal/portal/sessions/handler_test.go`. The struct and `Now()` method remain. `go test ./internal/portal/sessions/...` passes.
