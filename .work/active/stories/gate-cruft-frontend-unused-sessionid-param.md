---
id: gate-cruft-frontend-unused-sessionid-param
kind: story
stage: implementing
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
