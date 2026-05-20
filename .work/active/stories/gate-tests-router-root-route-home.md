---
id: gate-tests-router-root-route-home
kind: story
stage: implementing
tags: [testing]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: tests
created: 2026-05-20
updated: 2026-05-20
---

# Router test missing: `/` matches `home` (first-match wins)

## Priority
High

## Spec reference
Item: `spa-logged-in-landing-home-screen`
Acceptance criterion: "Navigating to `/` triggers `current.name === 'home'` and renders `Home.svelte`."

## Gap type
missing test for valid partition / e2e-seam

## Suggested test
```ts
test('matches / as home (first-match wins)', async () => {
  const { navigate, current } = await import('./router.svelte');
  navigate('/');
  expect(current.name).toBe('home');
  expect(current.params).toEqual({});
});
```

## Test location (suggested)
`frontend/src/lib/router.test.ts`

## Context
The router pattern for `/` is a net-new entry that must be FIRST per
design ("first-match semantics catch `/` before any other pattern"). The
seam between the router and Home screen at `current.name === 'home'` is
currently asserted only inside Home.test.ts via a mocked router. Without
a direct router test, a regression that reorders `routes[]` or breaks
the `/^\/$/` pattern would not be caught at the router layer.
