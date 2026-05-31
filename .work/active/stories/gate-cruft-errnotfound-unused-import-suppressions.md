---
id: gate-cruft-errnotfound-unused-import-suppressions
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

# Automerger ErrNotFound test keeps unused imports alive with dummy vars

## Confidence
Medium

## Category
unused import

## Location
`internal/portal/automerger/errnotfound_test.go:245`

## Evidence
```go
// Suppress unused imports.
var _ *gogit.Repository
var _ *object.Commit
var _ plumbing.Hash
```

## Removal
Remove the suppression block; remove the unused `gogit` and `object` imports.
Keep `plumbing`, which is used earlier in the file.


## Autopilot scope/design (2026-05-31)

Promoted by the gate-drain autopilot run. Implement the remediation direction above as a focused single-stride story, keep edits limited to the named surface, and verify with the targeted test or check that covers the changed file. For older backlog gate items, this run binds the work to `v0.5.0` because the user explicitly requested all gate-related work be scoped, designed, and implemented before release.
