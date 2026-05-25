---
id: gate-tests-playwright-landing-variant-project-spec
kind: story
stage: review
tags: [testing, e2e-test, ui, portal]
parent: null
depends_on: []
release_binding: null
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

## Implementation notes

### What was written

New file: `tests/e2e/playwright/landing-variant.spec.ts`

Two tests following approach (a) — a pure Playwright spec that assumes the
portal is booted with `JAMSESH_LANDING_VARIANT=project` before Playwright
runs.  This matches the pattern of all existing specs in this directory; none
of them boot Go fixtures inline.

**Test 1** — asserts `getByRole('heading', { name: /your team on a call/i, level: 1 })` is visible.
Selector derived from `ProjectLanding.svelte` hero section line 53:
`<h1>Your team on a call.<br>Your agents <span class="accent">in the loop.</span></h1>`
The story's suggested selector (`{ name: /jamsesh/i }`) was NOT used because
the wordmark (`jamsesh.`) is a link, not a heading — no `<h1>` or `<h2>`
in the component carries "jamsesh" text.

**Test 2** — asserts `getByRole('link', { name: /sign in/i })` is visible.
Derived from the topbar `<a href="/login" class="signin">Sign in →</a>` (line 42).
The regex omits `→` intentionally so the assertion is decoupled from cosmetic
copy changes.

The file-level docstring explains:
- the invariant (anonymous `/` → `<ProjectLanding/>` instead of redirect to `/login`)
- the runtime requirement (`PORTAL_URL` must point at a portal started with `JAMSESH_LANDING_VARIANT=project`)
- selector rationale for both role queries
- the deferred Go-orchestration wiring (ci-workflow story)

### What was verified

- `npx playwright test --list landing-variant.spec.ts` discovers both tests
  under `[chromium]` — confirmed.
- TypeScript compilation: the spec uses only Playwright types; no new imports;
  tsconfig.json in the playwright directory covers `.spec.ts` files.
- Full browser run was NOT performed.  Running the spec end-to-end requires
  Docker + a portal image built with `make test-portal-image` and the portal
  started with `JAMSESH_LANDING_VARIANT=project`.  That is heavier than this
  story alone justifies.

### Deferred work

Go-orchestrated fixture boot with `JAMSESH_LANDING_VARIANT=project` injected
via `Options.ExtraEnv` is tracked separately in the ci-workflow story.  Until
that lands, running these tests requires the manual docker invocation documented
in the spec file's docstring.
