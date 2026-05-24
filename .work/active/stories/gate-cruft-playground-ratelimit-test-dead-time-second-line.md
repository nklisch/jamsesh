---
id: gate-cruft-playground-ratelimit-test-dead-time-second-line
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

# Dead leftover `_ = time.Second` in playground ratelimit test

## Confidence
High

## Category
stale comment

## Location
`internal/portal/playground/ratelimit_test.go:273`

## Evidence
```go
if !allowed1 || !allowed2 {
    t.Error("first 2 requests should be within the per-minute burst of 2")
}
_ = time.Second // not actually sleeping; just confirming logic via the above
}
```

## Removal
The comment confesses the line is dead — "not actually sleeping". Delete the `_ = time.Second` line entirely. If the `time` import becomes unused, drop it too (verify with `go build`).
