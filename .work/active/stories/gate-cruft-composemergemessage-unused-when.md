---
id: gate-cruft-composemergemessage-unused-when
kind: story
stage: implementing
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


## Autopilot scope/design (2026-05-31)

Promoted by the gate-drain autopilot run. Implement the remediation direction above as a focused single-stride story, keep edits limited to the named surface, and verify with the targeted test or check that covers the changed file. For older backlog gate items, this run binds the work to `v0.5.0` because the user explicitly requested all gate-related work be scoped, designed, and implemented before release.
