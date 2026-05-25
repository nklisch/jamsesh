---
id: story-portal-visitor-entry-pages-spa-landing
kind: story
stage: implementing
tags: [ui, portal]
parent: feature-portal-visitor-entry-pages
depends_on: [story-portal-visitor-entry-pages-info-endpoint]
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# SPA portalInfo store + landing-variant routing + ProjectLanding component

## Brief

Wire the SPA to consume `/api/portal/info` (added in the dependency
story), branch the anonymous `/` route on `landing_variant`, and ship
the new `ProjectLanding.svelte` component for `project` mode. See
`feature-portal-visitor-entry-pages.md` Implementation Unit 2 for the
full design.

## Scope

- New `frontend/src/lib/portalInfo.svelte.ts` rune store (per the
  `wrapper-object-rune-store` pattern). Fetches `/api/portal/info`
  once on `init()`; caches; exposes `playgroundEnabled`,
  `landingVariant`, `loaded`. Safe fallback on fetch failure:
  `{playgroundEnabled: false, landingVariant: 'login'}`.
- New `frontend/src/lib/screens/ProjectLanding.svelte` mirroring
  `.mockups/screens/feature-portal-visitor-entry-pages/project-landing.html`.
  Uses `navigate()` for internal links.
- Update `frontend/src/App.svelte`: bootstrap `portalInfo.init()`
  alongside `auth.init()`; gate the existing auth-effect on
  `portalInfo.loaded`; branch anonymous `/` rendering by variant.
  - `project` → render `<ProjectLanding />` in-place at `/`
  - `auto` + `playgroundEnabled` → `navigate('/playground')`
  - `auto` + !`playgroundEnabled` OR `login` OR fallback →
    `navigate('/login')`
- Authenticated visitors at `/` are unchanged (Home.svelte).
- Tests for portalInfo store, ProjectLanding component, and the
  expanded App routing matrix (use the `spa-test-module-mock-barrel`
  pattern).
- Roll forward `docs/UX.md` — add an "Anonymous visitor at `/`"
  entry under "Portal UI surfaces" listing the three variants.

## Acceptance

Per the parent feature's Unit 2 acceptance criteria, in full:
- Anonymous `/` with `landing_variant: project` renders ProjectLanding
  matching the mockup structure.
- Anonymous `/` with `landing_variant: auto` and `playground_enabled:
  true` → `/playground`.
- Anonymous `/` with `landing_variant: auto` and `playground_enabled:
  false` → `/login`.
- Anonymous `/` with `landing_variant: login` → `/login`.
- Authenticated `/` shows Home.svelte for ALL variants.
- `/api/portal/info` fetch failure → safe fallback to login bounce;
  console warning logged.
- "Try the playground →" click on ProjectLanding navigates without
  page reload.
- All new components have tests following the established
  `spa-test-module-mock-barrel` and `view-state-union-machine`
  patterns.

## References

- Parent feature design: `.work/active/features/feature-portal-visitor-entry-pages.md`
- Mockup: `.mockups/screens/feature-portal-visitor-entry-pages/project-landing.html`
- Backend endpoint contract (dependency): see
  `story-portal-visitor-entry-pages-info-endpoint`
- Rune store pattern: `.claude/skills/patterns/wrapper-object-rune-store.md`
- Test mock pattern: `.claude/skills/patterns/spa-test-module-mock-barrel.md`
- View-state pattern: `.claude/skills/patterns/view-state-union-machine.md`
- Existing auth-gate code: `frontend/src/App.svelte`
- Existing router: `frontend/src/lib/router.svelte.ts`
