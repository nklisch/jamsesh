---
id: gate-cruft-destructionwarning-test-unused-warn-threshold-const
kind: story
stage: review
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

## Implementation notes

Deleted unused `WARN_THRESHOLD_MS` const from `frontend/src/lib/components/DestructionWarningBanner.test.ts:14`. Adjusted the inline comments on `SAFE_MS` / `WARN_MS` so they still explain the relationship to the (implicit) 5-min threshold.

Verified: `go build ./...` clean. Affected Go tests pass (`go test ./internal/portal/playground/... ./internal/portal/storage/objectstore/...`) excluding the pre-existing `TestJoinPlaygroundSession_WithNickname_UsesIt` failure on `main` (parked as `bug-playground-join-with-nickname-returns-410-on-fresh-session`). Frontend tests pass for the two touched files (`vitest run`).
