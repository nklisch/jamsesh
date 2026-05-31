---
id: gate-cruft-composemergemessage-unused-when
kind: story
stage: drafting
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: cruft
created: 2026-05-31
updated: 2026-05-31
---

# `composeMergeMessage` carries an unused future-use parameter

## Confidence
Medium

## Category
unused parameter

## Location
`internal/portal/automerger/outcomes.go:267`

## Evidence
```go
func composeMergeMessage(sourceCommit *object.Commit, in ApplyInput, when time.Time) string {
	_ = when // for future use
```

## Removal
Remove the `when time.Time` parameter, remove `_ = when`, and update the single
call site at `outcomes.go:192`.

