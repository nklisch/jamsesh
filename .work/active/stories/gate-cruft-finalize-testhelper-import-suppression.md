---
id: gate-cruft-finalize-testhelper-import-suppression
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

# Finalize test helper has obsolete finalize import suppression

## Confidence
Medium

## Category
unused import

## Location
`internal/portal/finalize/testhelpers_test.go:150`

## Evidence
```go
// ensure the finalize package import is materially used so go test doesn't
// complain about unused imports when only behaviour through env.handler is
// exercised.
var _ = finalize.FinalizeLockTTL
```

The file already uses `finalize.Handler` and `finalize.New`.

## Removal
Delete the comment and dummy `var _ = finalize.FinalizeLockTTL` line.

