---
id: gate-cruft-delete-realclock
kind: story
stage: implementing
tags: [cleanup, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# Dead `realClock` type with `now()` helper bypassing it

## Confidence
High

## Category
dead function / unused abstraction

## Location
`internal/portal/comments/service.go:31-33` and
`internal/portal/mcpendpoint/handler.go:33-35`

## Evidence
```go
type realClock struct{}
func (realClock) Now() time.Time { return time.Now().UTC() }

func (s *Service) now() time.Time {
    if s.Clock == nil {
        return time.Now().UTC()   // does NOT use realClock{}
    }
    return s.Clock.Now()
}
```

## Removal
Delete both `realClock` type declarations. The `now()` fallback path
calls `time.Now().UTC()` directly without going through `realClock{}`,
so the type is unreachable. Update the doc-comment on `now()` to drop
the reference to `realClock`.
