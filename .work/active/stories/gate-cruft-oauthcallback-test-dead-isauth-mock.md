---
id: gate-cruft-oauthcallback-test-dead-isauth-mock
kind: story
stage: done
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: cruft
created: 2026-05-20
updated: 2026-05-20
---

# Dead `isAuthenticated: false` field in OAuthCallback's auth mock

## Confidence
Medium

## Category
dead function (dead mock field)

## Location
`frontend/src/lib/screens/OAuthCallback.test.ts:25`

## Evidence
```ts
vi.mock('$lib/auth.svelte', () => ({
  auth: {
    setTokens: (...args: unknown[]) => mockSetTokens(...args),
    loadCurrentUser: () => mockLoadCurrentUser(),
    isAuthenticated: false,
  },
}));
```

`OAuthCallback.svelte` never reads `auth.isAuthenticated` (grep
confirmed). The field is defensive scaffolding.

## Removal
Delete line 25. The mock surface should be the minimum the SUT
exercises.

## Implementation notes

- Confirmed `OAuthCallback.svelte` has zero references to `auth.isAuthenticated` (grep returned empty).
- Removed the `isAuthenticated: false` line from the `vi.mock('$lib/auth.svelte', ...)` block in `OAuthCallback.test.ts`.
- `npm run check`: 0 errors, 2 pre-existing warnings (unrelated files).
- `npm test OAuthCallback`: 15/15 tests pass. No test relied on the dead field.

## Review (2026-05-20)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: One-line removal of dead mock field. SUT (`OAuthCallback.svelte`) confirmed not to read `auth.isAuthenticated`. All 15 OAuthCallback tests still pass.
