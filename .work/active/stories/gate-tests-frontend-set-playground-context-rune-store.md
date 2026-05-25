---
id: gate-tests-frontend-set-playground-context-rune-store
kind: story
stage: done
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

## Implementation notes

**Land mode** — all specified tests already exist in `frontend/src/lib/auth.test.ts` (lines 396–464). The test suite includes:
- `playgroundContext starts null` (line 396)
- `setPlaygroundContext populates playgroundContext` with all three fields (line 401)
- `setPlaygroundContext(null) clears the context` (line 417)
- `setting playgroundContext does not affect isAuthenticated` (line 431)
- `isAuthenticated true and playgroundContext non-null can coexist` (line 448)

The seam contract is fully documented. No new code written.

## Review notes

Approve. Land mode verified: all 5 claimed tests exist at the claimed line
numbers in `frontend/src/lib/auth.test.ts` (396, 401, 417, 431, 448). Each
asserts the real `auth.playgroundContext` rune-store after calling
`setPlaygroundContext` directly. The 4th and 5th tests pin the important
orthogonality with `isAuthenticated`. All pass.
