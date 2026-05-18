---
id: gate-cruft-delete-stubs-go
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

# Empty placeholder file `stubs.go` with stale comment

## Confidence
High

## Category
stale comment / dead file

## Location
`cmd/jamsesh/hooks/stubs.go:1-5`

## Evidence
```go
package hooks

// stubs.go is intentionally empty. All hook handlers have been implemented.
// This file is retained as a placeholder in case future stub hooks are needed.
```

## Removal
Delete the file entirely. Empty placeholder files are pure cruft; if a
future hook needs stubbing, the file can be created at that time.

## Implementation notes
Confirmed `cmd/jamsesh/hooks/stubs.go` contained only a package declaration
and a stale comment — no exported symbols. Grep across `cmd/`, `internal/`,
and `tests/` found no references to any symbol from this file. Deleted via
`git rm`. Build (`go build ./cmd/jamsesh/hooks/...`) and tests
(`go test ./cmd/jamsesh/hooks/...`) both pass after removal.
