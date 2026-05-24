---
id: gate-cruft-destructionwarning-test-unused-warn-threshold-const
kind: story
stage: implementing
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# TypeScript unused-local `WARN_THRESHOLD_MS` in DestructionWarningBanner test

## Confidence
High

## Category
dead function

## Location
`frontend/src/lib/components/DestructionWarningBanner.test.ts:14`

## Evidence
```ts
const WARN_THRESHOLD_MS = 5 * 60 * 1000; // 5 min
const SAFE_MS = 10 * 60 * 1000;          // 10 min — above threshold
const WARN_MS = 4 * 60 * 1000;           // 4 min — below threshold
```
`tsc --noUnusedLocals` reports: `error TS6133: 'WARN_THRESHOLD_MS' is declared but its value is never read.`

## Removal
Delete the `WARN_THRESHOLD_MS` line. `SAFE_MS` and `WARN_MS` describe their relationship to the threshold in their inline comments, which is enough — the constant they reference is implicit in the component under test. Alternatively, if the test should assert the threshold value via the constant, add the assertion; otherwise drop it.
