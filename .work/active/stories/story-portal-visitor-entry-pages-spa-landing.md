---
id: story-portal-visitor-entry-pages-spa-landing
kind: story
stage: review
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

## Implementation notes

### New files

- **`frontend/src/lib/portalInfo.svelte.ts`** — module-level wrapper-object
  rune store. Private `$state` vars for `_playgroundEnabled`, `_landingVariant`,
  `_loaded`. Exposed via `export const portalInfo = { get ..., init() }`.
  `init()` is idempotent (guards with `_loaded` and a `_fetching` promise).
  Calls `client.GET('/api/portal/info')` using the shared openapi-fetch client.
  Falls back to `{ playgroundEnabled: false, landingVariant: 'login' }` on any
  error, always marks `_loaded = true` in `finally`. Exports `LandingVariant`
  string-literal-union type.

- **`frontend/src/lib/screens/ProjectLanding.svelte`** — Svelte 5 component
  mirroring `option-1.html` (Swiss / ITS). Top bar with wordmark + nav (Home /
  Docs / Self-host / Playground) + GitHub icon-link + Sign in. Three numbered
  sections: 01 HERO (h1 + lede + "Try the playground →" CTA), 02 SCHEMATIC
  (body text + diagram listing), 03 METHOD (three steps A/B/C). Colophon footer.
  Internal links use `onclick` → `navigate()`; external links use plain `<a
  href target="_blank">`. GitHub SVG inline. Scoped `<style>` with 12-col Swiss
  grid using CSS tokens.

### Modified files

- **`frontend/src/App.svelte`** — added `portalInfo` import + `ProjectLanding`
  import. Added `void portalInfo.init()` call at module bootstrap (parallel
  with auth). Auth-gate `$effect` updated: the `home` branch now checks
  `portalInfo.loaded` before deciding variant; `project` variant returns early
  (no navigation); `auto`+enabled → `/playground`; everything else → `/login`.
  Template: added `{:else if current.name === 'home' && !auth.isAuthenticated &&
  portalInfo.loaded && portalInfo.landingVariant === 'project'}` branch before
  the existing `home` branch to render `<ProjectLanding />` in-place.

- **`frontend/src/lib/api/types.gen.ts`** — regenerated via `pnpm run generate`
  to pick up the `PortalInfo` schema and `GET /api/portal/info` operation added
  in the dep story.

### New tests

- **`frontend/src/lib/portalInfo.test.ts`** — 6 tests: successful fetch with
  project variant; successful fetch with auto+disabled; no-data response falls
  back + warns; network throw falls back + warns; second `init()` is no-op;
  concurrent `init()` calls share one promise.

- **`frontend/src/lib/screens/ProjectLanding.test.ts`** — 18 tests: wordmark,
  eyebrow, h1, lede, section labels (01/SCHEMATIC/METHOD), schematic diagram
  lines, method steps A/B/C, colophon; "Try the playground →" click calls
  `navigate('/playground')`; Sign-in click calls `navigate('/login')`; wordmark
  click calls `navigate('/')`; GitHub icon-link has correct href + target;
  SVG present; Self-host URL correct.

### Amended tests

- **`frontend/src/App.test.ts`** — added `ProjectLanding.svelte` stub mock;
  added `portalInfo` wrapper-object mock with mutable `mockPortalInfo` state;
  added `mockPortalInfo` reset in both suite `beforeEach` blocks; added 6 new
  landing-variant routing tests covering all acceptance-criteria combinations
  (project/auto+enabled/auto+disabled/login for unauthed; authed+project stays
  on Home; gate holds while not loaded).

### Docs roll-forward

- **`docs/UX.md`** — added "Anonymous visitor at `/`" entry at the top of
  "Portal UI surfaces", documenting the three variants (`project`, `auto`,
  `login`), their render behaviour, the fallback policy, and the mockup path.
