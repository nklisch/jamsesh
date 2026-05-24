---
id: gate-tests-frontend-set-playground-context-rune-store
kind: story
stage: drafting
tags: [testing, ui, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# `setPlaygroundContext` rune-store persistence not isolated-tested

## Priority
Medium

## Spec reference
Item: `story-epic-ephemeral-playground-portal-ui-anonymous-entry`

Acceptance criterion: Auth must persist + restore the playground bearer + nickname for the in-session JS context. JoinerPicker tests assert the call is made; nothing tests that the rune-store actually persists.

## Gap type
missing test for e2e-seam

## Suggested test
```ts
it('setPlaygroundContext stores bearer, sessionId, and nickname accessible via auth.playground{...}', () => { ... });
it('clearPlaygroundContext purges the rune-store state', () => { ... });
```

## Test location (suggested)
`frontend/src/lib/auth.test.ts`
