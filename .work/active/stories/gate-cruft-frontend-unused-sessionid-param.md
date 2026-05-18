---
id: gate-cruft-frontend-unused-sessionid-param
kind: story
stage: review
tags: [cleanup, ui]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# Unused `sessionId` parameter destructured in 3 component tests

## Confidence
High

## Category
unused parameter (TS6133)

## Location
- `frontend/src/lib/components/ActivityFeed.test.ts:13`
- `frontend/src/lib/components/CommentsTab.test.ts:20`
- `frontend/src/lib/components/TreeDag.test.ts:20`

## Evidence
```ts
const mockSubscribe = vi.fn((sessionId: string, type: string, handler: ...) => {
  if (!handlersByType.has(type)) handlersByType.set(type, []);
```

## Removal
Rename to `_sessionId` or drop the parameter — the mock body never
inspects the session id. Triggered when running
`tsc --noUnusedParameters --noUnusedLocals`.

## Implementation notes

Renamed `sessionId` → `_sessionId` in the `mockSubscribe` `vi.fn` declaration
in all three files:
- `frontend/src/lib/components/ActivityFeed.test.ts:13`
- `frontend/src/lib/components/CommentsTab.test.ts:20`
- `frontend/src/lib/components/TreeDag.test.ts:20`

Chose rename over drop to keep the mock signature aligned with the real
`subscribe` function interface for type-checking purposes.

Verification:
- `tsc --noUnusedLocals --noUnusedParameters --noEmit` produced no output for
  these three files (sessionId warnings fully cleared).
- All 28 tests across the three test files passed (3 test files, 28 tests).
