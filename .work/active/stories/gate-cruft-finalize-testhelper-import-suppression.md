---
id: gate-cruft-finalize-testhelper-import-suppression
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


## Autopilot scope/design (2026-05-31)

Promoted by the gate-drain autopilot run. Implement the remediation direction above as a focused single-stride story, keep edits limited to the named surface, and verify with the targeted test or check that covers the changed file. For older backlog gate items, this run binds the work to `v0.5.0` because the user explicitly requested all gate-related work be scoped, designed, and implemented before release.
