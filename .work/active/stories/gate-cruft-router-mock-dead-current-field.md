---
id: gate-cruft-router-mock-dead-current-field
kind: story
stage: review
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: cruft
created: 2026-05-20
updated: 2026-05-20
---

# Mock exposes `current` key that the SUT never imports

## Confidence
Low

## Category
over-abstraction (defensive over-mocking)

## Location
`frontend/src/lib/screens/OAuthCallback.test.ts:31` and `frontend/src/lib/screens/Home.test.ts:21`

## Evidence
```ts
// OAuthCallback.test.ts
vi.mock('$lib/router.svelte', () => ({
  current: { name: 'oauth-callback', params: {} },
  navigate: (...args: unknown[]) => mockNavigate(...args),
}));
```

Neither `OAuthCallback.svelte` nor `Home.svelte` imports `current` from
`$lib/router.svelte` — they only import `navigate`. The `current` field
is dead in the mock.

## Removal
Drop the `current: ...` line from both mock factories. Judgment call —
consistent mock shape across tests is arguably a feature, which is why
this is filed as Low.

## Implementation notes
Removed the `current: { ... }` key from the `vi.mock('$lib/router.svelte', ...)` factory in both `Home.test.ts` (line 21) and `OAuthCallback.test.ts` (line 31). Neither SUT imports `current` — only `navigate` is used. All 47 tests across both files remain green after the change.
