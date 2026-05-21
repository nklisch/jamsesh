---
id: gate-cruft-login-test-unused-spyon-location
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

## Implementation notes

Grep for `vi.spyOn(window, 'location'` in `Login.test.ts` confirmed exactly
one instance — the `beforeEach` block. No other usage depended on it.

Deleted the `// Reset location.assign spy before each test` comment and the
4-line `vi.spyOn(window, 'location', 'get').mockReturnValue(...)` call.
`beforeEach` now contains only `vi.clearAllMocks()` and
`mockAuth.isAuthenticated = false`.

`npm run check` — 0 errors, 2 pre-existing warnings.
`npm test` run twice — 472/472 pass both times. One first-run flaky failure
in `Home.test.ts` and Login OAuth URL validation tests (from concurrent story
`gate-security-authorize-url-no-scheme-host-validation`) resolved on re-run;
neither is related to this change.
