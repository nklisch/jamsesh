---
id: gate-cruft-frontend-unused-beforeeach-import
kind: story
stage: done
tags: [cleanup, ui]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# Unused `beforeEach` import in three test files

## Confidence
High

## Category
unused import (TS6133)

## Location
- `frontend/src/lib/components/Chrome.test.ts:7`
- `frontend/src/lib/router.test.ts:4`
- `frontend/src/lib/screens/SessionsLanding.test.ts:5`

## Evidence
`tsc --noUnusedLocals --noUnusedParameters` reports `'beforeEach' is
declared but its value is never read.` Each file imports `beforeEach`
from vitest but no setup hook is actually defined.

## Removal
Drop `beforeEach` from the vitest import list in each file.

## Implementation notes

Removed `beforeEach` from the vitest import destructuring in each of the three files:

- `frontend/src/lib/components/Chrome.test.ts`: `{ describe, it, expect, vi, afterEach }` (kept `afterEach`)
- `frontend/src/lib/router.test.ts`: `{ describe, test, expect }` (only import in file)
- `frontend/src/lib/screens/SessionsLanding.test.ts`: `{ describe, it, expect, vi, afterEach }` (kept `afterEach`)

Verified: `tsc --noUnusedLocals --noUnusedParameters --noEmit` no longer reports `beforeEach` errors in any of the three files. All 24 tests across the three suites pass.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Mechanical change matching the gate finding spec. Implementation notes accurately describe what was changed. Global `go build ./...` and `go test ./internal/portal/...` pass after the wave landed.
