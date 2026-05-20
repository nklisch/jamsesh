---
id: gate-cruft-login-test-unused-spyon-location
kind: story
stage: drafting
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: cruft
created: 2026-05-20
updated: 2026-05-20
---

# Unobserved `vi.spyOn(window, 'location', 'get')` setup in `beforeEach`

## Confidence
Medium

## Category
defensive try/catch (defensive test scaffolding)

## Location
`frontend/src/lib/screens/Login.test.ts:29-32`

## Evidence
```ts
beforeEach(() => {
  vi.clearAllMocks();
  mockAuth.isAuthenticated = false;
  // Reset location.assign spy before each test
  vi.spyOn(window, 'location', 'get').mockReturnValue({
    ...window.location,
    assign: vi.fn(),
  } as any);
});
```

Every test that actually needs to observe `window.location.assign`
re-stubs via `Object.defineProperty(window, 'location', { value: ...,
writable: true, configurable: true })` (lines 53-57, 121-125, 249-253,
262-266). The `vi.spyOn` getter's `assign: vi.fn()` is never referenced
by any assertion.

## Removal
Delete the `vi.spyOn(window, 'location', 'get').mockReturnValue(...)`
block from `beforeEach`. The `afterEach` `vi.restoreAllMocks()` plus
per-test `Object.defineProperty` handles cleanup and stubbing already.
