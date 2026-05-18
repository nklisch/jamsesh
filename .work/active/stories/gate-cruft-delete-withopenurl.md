---
id: gate-cruft-delete-withopenurl
kind: story
stage: implementing
tags: [cleanup, plugin]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# Unused `WithOpenURL` Option constructor

## Confidence
High

## Category
dead function

## Location
`cmd/jamsesh/auth/auth.go:61-63`

## Evidence
```go
// WithOpenURL replaces the default browser-open function. Primarily used in
// tests to capture the URL without spawning a real browser.
func WithOpenURL(fn func(url string) error) Option { ... }
```

## Removal
Delete the function. Zero callers anywhere — neither production nor
tests (verified with grep). The comment claims it's "primarily used in
tests" but it isn't used at all.
