---
id: gate-cruft-router-test-unused-beforeEach-import
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
