---
id: gate-tests-addorg-reactivity
kind: story
stage: review
tags: [testing]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: tests
created: 2026-05-20
updated: 2026-05-20
---

# `auth.addOrg` reactivity (subscriber notified after append) not asserted

## Priority
Medium

## Spec reference
Item: `spa-logged-in-landing-auth-store-orgs-cache`
Acceptance criterion: "`auth.addOrg(org)` appends `org` to `_orgs`, creating the array when `_orgs` was `null`, **via reassignment (not in-place push)**." Risks section: "Reactivity edge case in `auth.addOrg`. ... Tested explicitly."

## Gap type
missing test for boundary

## Suggested test
```ts
it('addOrg triggers $effect re-run for orgs subscribers', async () => {
  const { auth } = await import('$lib/auth.svelte');
  let observed = 0;
  const cleanup = $effect.root(() => {
    $effect(() => {
      auth.orgs; // dependency
      observed++;
    });
  });
  auth.addOrg({ id: 'a', name: 'a', slug: 'a', role: 'member' });
  await tick();
  expect(observed).toBeGreaterThanOrEqual(2);
  cleanup();
});
```

## Test location (suggested)
`frontend/src/lib/auth.test.ts`

## Context
Existing test verifies the array reference changes
(`expect(auth.orgs).not.toBe(originalArray)`), which is a proxy for
reactivity. But the spec's Risks section explicitly names "consumer
effects fire after addOrg" as the property being defended. A
reactivity-observation test pins the consumer-visible behavior; the
reference-inequality test does not (a reassignment that mutates outside
Svelte's tracking would also produce a new reference).

## Implementation notes

### Approach taken: Approach B — mounted Svelte component (separate test file)

**Files changed:**
- `frontend/src/lib/auth.reactivity.test.ts` — new test file containing the reactivity test
- `frontend/src/lib/AuthOrgsReactivityHarness.test.svelte` — tiny Svelte test component that reads `auth.orgs` in a `$effect` and renders the run count to DOM

### Why a separate test file (not added to `auth.test.ts`)

The existing `auth.test.ts` calls `vi.resetModules()` in `beforeEach`. `vi.resetModules()` evicts the `svelte` package from the module cache along with all other modules. When the test then re-imports `$lib/auth.svelte` via `await import(...)`, the fresh auth module binds to a **different Svelte runtime instance** than the `AuthOrgsReactivityHarness` component (which was compiled and evaluated before the reset). This means the auth module's `$state` signals live in a different scheduler context — cross-module reactive tracking breaks silently (the effect never re-runs). 

The solution: a dedicated file `auth.reactivity.test.ts` that does NOT reset modules. Both `auth` and the harness component are statically imported, so they share the same Svelte runtime and the same signal registry. Reactivity works correctly.

### Why the harness component imports `auth` directly (not via prop)

Injecting `auth` as a prop would require the test to pass the module instance after `await import(...)`, which re-introduces the cross-runtime problem. With a direct static import, the component and test get the same module instance automatically.

### `untrack()` to prevent infinite update cycle

Inside the harness's `$effect`, the counter increment is `effectRunCount = untrack(() => effectRunCount) + 1`. Without `untrack`, the `effectRunCount++` expression reads `effectRunCount` (a `$state`) inside the effect, subscribing the effect to `effectRunCount`. Each increment would then re-trigger the effect — an infinite update loop (`effect_update_depth_exceeded`). `untrack()` reads the value without registering it as a dependency.

### Negative-case verification

**Tested with a no-op `addOrg`** (function body left empty — `_orgs` never mutated): the test correctly fails with `expected 1 to be greater than or equal to 2`. The effect stays at count 1 because `_orgs` never changes. This confirms the test is not trivially green.

**Also tested with push-in-place** (`_orgs!.push(org)`): the test **passes** — because Svelte 5's `$state` wraps arrays in a reactive proxy, and `.push()` is tracked by that proxy. This is a significant finding: the story's proposed negative case (push-in-place) does NOT represent a reactivity failure in Svelte 5. The existing `'addOrg appends to existing orgs array via reassignment'` test in `auth.test.ts` asserts reference inequality (`expect(auth.orgs).not.toBe(originalArray)`) which IS a meaningful discriminator for push vs. reassign — but the reactivity test cannot distinguish them because both are reactive. The reactivity test defends the property that matters to consumers: "the `$effect` re-runs after any `addOrg` call", not specifically that the internal implementation uses reassignment over push.

### Run results

Both runs consistent. 42 test files / 465 tests pass. `npm run check` reports 0 errors (2 pre-existing warnings unrelated to this change).
