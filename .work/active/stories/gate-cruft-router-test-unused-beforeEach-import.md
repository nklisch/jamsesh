---
id: gate-cruft-router-test-unused-beforeEach-import
kind: story
stage: done
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# TypeScript unused-import `beforeEach` in router test

## Confidence
High

## Category
unused import

## Location
`frontend/src/lib/router.test.ts:4`

## Evidence
```ts
import { describe, test, expect, beforeEach } from 'vitest';
```
`tsc --noUnusedLocals --noUnusedParameters` reports: `error TS6133: 'beforeEach' is declared but its value is never read.`

## Removal
Drop `beforeEach` from the import list. No other change needed — no `beforeEach(...)` call appears anywhere in the file.

## Implementation notes

Dropped `beforeEach` from the vitest import in `frontend/src/lib/router.test.ts:4`. No call sites in the file.

Verified: `go build ./...` clean. Affected Go tests pass (`go test ./internal/portal/playground/... ./internal/portal/storage/objectstore/...`) excluding the pre-existing `TestJoinPlaygroundSession_WithNickname_UsesIt` failure on `main` (parked as `bug-playground-join-with-nickname-returns-410-on-fresh-session`). Frontend tests pass for the two touched files (`vitest run`).
