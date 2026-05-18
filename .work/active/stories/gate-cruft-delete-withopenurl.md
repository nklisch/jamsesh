---
id: gate-cruft-delete-withopenurl
kind: story
stage: review
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

## Implementation notes

- Deleted `WithOpenURL` (lines 61-65 in original) and its doc-comment from
  `cmd/jamsesh/auth/auth.go`. No imports were affected — the function used no
  packages not already needed by the remaining code.
- `grep -rn 'WithOpenURL'` confirmed zero callers; only hits were the story
  file and the parent epic doc.
- `go build ./cmd/jamsesh/auth/...` and `go test ./cmd/jamsesh/auth/...` both
  pass cleanly.
