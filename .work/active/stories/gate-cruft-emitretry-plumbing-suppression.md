---
id: gate-cruft-emitretry-plumbing-suppression
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

# Emit-retry test has obsolete plumbing import suppression

## Confidence
Medium

## Category
unused import

## Location
`internal/portal/automerger/emit_retry_test.go:555`

## Evidence
```go
// Suppress plumbing import - used via buildConflictRepo / buildApplyRepo.
var _ plumbing.Hash
```

`plumbing` is directly used at lines 117 and 191, so this suppression is no
longer needed.

## Removal
Delete the comment and dummy `var _ plumbing.Hash` line.

