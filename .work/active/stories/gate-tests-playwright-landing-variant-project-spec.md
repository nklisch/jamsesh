---
id: gate-tests-playwright-landing-variant-project-spec
kind: story
stage: implementing
tags: [testing, e2e-test, ui, portal]
parent: null
depends_on: []
release_binding: v0.4.1
gate_origin: tests
created: 2026-05-25
updated: 2026-05-25
---

# Missing Playwright spec — anonymous `/` with `landing_variant=project`

## Priority
Critical

## Spec reference
Item: `feature-portal-visitor-entry-pages`
Testing → Integration / smoke (feature body lines 360-364):
> Add one new spec covering anonymous `/` with
> `JAMSESH_LANDING_VARIANT=project` rendering the ProjectLanding component.

## Gap type
Missing e2e — scoped in design, not implemented.

## Location
`tests/e2e/playwright/` contains only `error-states.spec.ts`,
`finalize.spec.ts`, `login.spec.ts`. No spec touches `landing_variant`,
`project`, or `ProjectLanding`. The wsgateway/portal e2e suite
(`tests/e2e/golden/*.go`) likewise has zero hits for `portal/info`,
`landing_variant`, or `ProjectLanding`.

## Suggested test
```ts
// tests/e2e/playwright/landing-variant.spec.ts
test('anonymous root with landing_variant=project shows ProjectLanding', async ({ page }) => {
  // portal already booted with JAMSESH_LANDING_VARIANT=project in fixture
  await page.goto('/');
  await expect(page.getByRole('heading', { name: /jamsesh/i })).toBeVisible();
  await expect(page.getByRole('link', { name: /sign in/i })).toBeVisible();
});
```

## Test location (suggested)
`tests/e2e/playwright/landing-variant.spec.ts` + fixture extension to
boot portal with the env var set.

## Impact
The whole-stack contract — operator sets env → portal serves config →
SPA fetches it → router dispatches by variant → DOM renders
`<ProjectLanding/>` — has unit-level coverage but no integrated proof.
The feature design explicitly called this out as the smoke test for the
cross-cutting capability.
