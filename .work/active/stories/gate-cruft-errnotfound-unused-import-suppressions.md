---
id: gate-cruft-errnotfound-unused-import-suppressions
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

