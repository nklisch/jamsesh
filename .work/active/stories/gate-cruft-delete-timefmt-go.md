---
id: gate-cruft-delete-timefmt-go
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

# Dead helper module `internal/db/store/timefmt.go`

## Confidence
High

## Category
dead function

## Location
`internal/db/store/timefmt.go:11-27`

## Evidence
```go
const tsLayout = "2006-01-02T15:04:05Z"
func formatTS(t time.Time) string { return t.UTC().Format(tsLayout) }
func parseTS(s string) (time.Time, error) { /* ... */ }
```

## Removal
Delete the file. `tsLayout`, `formatTS`, and `parseTS` are never
referenced anywhere outside this file (verified with grep across `cmd/`
and `internal/`). sqlc-generated adapters handle timestamp marshalling
directly.
