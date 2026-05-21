---
id: gate-tests-addorg-reactivity
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
