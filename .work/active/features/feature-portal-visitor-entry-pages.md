---
id: feature-portal-visitor-entry-pages
kind: feature
stage: review
tags: [ui, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Portal visitor-facing entry pages

## Brief

The portal today has no curated entry experience for anonymous visitors.
The root path `/` requires auth, so unauthed traffic bounces straight to
`/login` — there's no project landing, no path to discovering the
playground unless the visitor already knows the `/playground` URL. This
feature introduces a deploy-time flag that selects what unauthenticated
visitors see at `/`, with playground-discoverability solved as a
side-effect of the default mode.

## Scope changes since scope-time

- The `story-playground-share-view` child has been **pulled out of this
  feature** — investigation showed the "404" is a 5-line CLI bug in
  `cmd/jamsesh/sessioncmd/new.go:502` (the CLI prints
  `/playground/<id>` instead of `/playground/s/<id>/join`, which is the
  actual SPA route per `frontend/src/lib/router.svelte.ts:26`). That's
  `[bug, cli, plugin]` work, not portal UI. Tracked separately as
  `story-fix-cli-playground-share-url`.
- The `story-landing-flagged-dual-mode` child has been **replaced** by
  two more specific child stories that match the natural backend ⇄
  frontend seam (see Implementation Units below).

## Design decisions

- **Variant strategy**: project landing is a NEW component at `/`;
  "generic" mode reuses the existing `/playground`
  (`PlaygroundLanding.svelte`) via routing — no duplicate component. The
  flag determines which path unauthenticated `/` traffic takes.
- **Flag mechanism**: env var + YAML, following the
  `docs/SELF_HOST.md` pattern. `JAMSESH_LANDING_VARIANT` /
  `landing.variant`.
- **Flag values** (3 modes):
  - `auto` (default) — if `JAMSESH_PLAYGROUND_ENABLED=true` in this
    deploy, redirect anonymous `/` to `/playground`. If playground is
    disabled, bounce to `/login`. This is the **playground-discoverability
    default** — playground deploys get a real public entry point with
    zero extra config.
  - `project` — render the new `ProjectLanding.svelte` component at `/`
    for anonymous visitors. Used by jamsesh.dev.
  - `login` — anonymous `/` bounces to `/login`, today's behaviour.
    Available for self-hosters who want auth-only entry even when
    playground is enabled.
- **Flag scope**: anonymous visitors only. Authenticated visitors at `/`
  continue to see `Home.svelte` (the org picker / org creation flow) —
  the flag has no effect on the authed path.
- **Discovery surface for playground**: the project landing's primary
  hero CTA is the "Try the playground →" button (matches the
  `01-prospect-landing.html` design language). Sign-in stays as a
  quieter top-bar link, not a competing CTA.
- **SPA bootstrap**: introduces a new public `GET /api/portal/info`
  endpoint that returns `{ playground_enabled: bool, landing_variant:
  string }`. The SPA fetches it on bootstrap before the router decides
  what to render at `/` for anonymous visitors. No auth required.
- **Fail-safe**: if the `/api/portal/info` fetch fails on bootstrap,
  the SPA treats `landing_variant` as `login` and `playground_enabled`
  as `false` — falls back to today's bounce-to-/login behaviour rather
  than rendering an indeterminate landing.

## Mockups

- **Project landing — chosen direction**:
  [`.mockups/screens/feature-portal-visitor-entry-pages/option-1.html`](../../../.mockups/screens/feature-portal-visitor-entry-pages/option-1.html)
  — **Option 1, Swiss / ITS pole**. Strict 12-col grid, numbered
  sections, asymmetric type-led composition, header nav (Home / Docs
  / Self-host / Playground + GitHub icon + Sign in). Signed off as
  the chosen direction on 2026-05-24 after a 4-option exploration
  (Swiss / Neubrutalism / Provocation / Swiss+Neubrutalist hybrid)
  produced via `/ux-ui-design:screens`. ProjectLanding.svelte
  implementation mirrors this mockup's structure.
- **Comparison index**:
  [`.mockups/screens/feature-portal-visitor-entry-pages/index.html`](../../../.mockups/screens/feature-portal-visitor-entry-pages/index.html)
  — side-by-side view of the chosen direction plus all explored
  alternatives, with the friendly-minimalist baseline kept for
  reference (`project-landing.html`).
- **Alternative options retained for reference**:
  `option-2.html` (Neubrutalism), `option-3.html` (Provocation),
  `option-4.html` (Swiss + Neubrutalist hybrid), `project-landing.html`
  (friendly-minimalist baseline). All in
  `.mockups/screens/feature-portal-visitor-entry-pages/`.
- **Generic landing (auto mode)**: reuses
  [`.mockups/flows/playground-onboarding/01-prospect-landing.html`](../../../.mockups/flows/playground-onboarding/01-prospect-landing.html)
  — the existing `/playground` rendered at `/` via redirect. No new
  mockup needed.
- **Login mode**: no mockup — today's `/login` bounce, unchanged.
- See [`.mockups/screens/feature-portal-visitor-entry-pages/README.md`](../../../.mockups/screens/feature-portal-visitor-entry-pages/README.md)
  for the full mockup index.

## Architectural choice

**Chosen**: backend exposes a public `/api/portal/info` endpoint
returning deploy-time flags as JSON; SPA fetches on bootstrap, caches in
a rune store, and the auth gate in `App.svelte` consults it before
deciding what `/` renders for anonymous visitors.

**Alternative considered: build-time injection.** The variant could be
baked into the SPA at Vite build time (Vite env var). Rejected because
it would force two SPA builds (project / generic), couple the SPA
release cadence to deploy variant decisions, and break the "one binary
for self-hosters" promise — operators could no longer flip the flag in
deploy config without rebuilding the SPA.

**Alternative considered: server-rendered index.html.** Inject the
config into a `<script>` tag in `index.html` at request time. Rejected
because the portal currently serves the SPA as embedded static assets
without per-request rendering — introducing template rendering for one
config blob is over-architected. The fetch-on-bootstrap pattern is
trivial (one round-trip) and matches how the SPA already fetches
`/api/me` after auth.

## Implementation Units

### Unit 1: Portal config + `GET /api/portal/info` endpoint
**Story**: `story-portal-visitor-entry-pages-info-endpoint`
**Files**:
- `internal/portal/config/config.go` — add `LandingVariant string` field
  to the config struct; validate enum `auto|project|login`; default
  `auto`; parse from `JAMSESH_LANDING_VARIANT` and YAML key
  `landing.variant`. Follow the existing pattern for enum config
  knobs in this file.
- `internal/portal/portalinfo/handler.go` *(new)* — package containing
  `Handler` with one method `GetPortalInfo(ctx, req) (resp, error)`
  conforming to the strict-server interface. Returns the locked
  config snapshot.
- `internal/portal/portalinfo/handler_test.go` *(new)* — table-driven
  test covering `(playground_enabled, landing_variant)` combinations:
  `(true, auto)`, `(false, auto)`, `(true, project)`, `(true, login)`.
- `cmd/portal/main.go` — wire the handler into the strict-server
  registration. The route is **public** (no auth middleware), under
  `/api/portal/info`.
- `docs/openapi.yaml` — add `PortalInfo` schema (object with
  `playground_enabled: boolean`, `landing_variant: string` enum), add
  `GET /api/portal/info` path returning `PortalInfo` (200, no auth
  required, no error codes beyond 5xx). Trigger
  `go generate ./internal/api/openapi` to refresh the generated server.

```go
// internal/portal/config/config.go (additions, abbreviated)
type Config struct {
    // ... existing fields ...
    LandingVariant string // "auto" | "project" | "login"; default "auto"
}

func (c *Config) validateLandingVariant() error {
    switch c.LandingVariant {
    case "auto", "project", "login":
        return nil
    default:
        return fmt.Errorf("invalid JAMSESH_LANDING_VARIANT %q (want auto|project|login)", c.LandingVariant)
    }
}
```

```go
// internal/portal/portalinfo/handler.go (new, abbreviated)
package portalinfo

type Handler struct {
    PlaygroundEnabled bool
    LandingVariant    string
}

func (h *Handler) GetPortalInfo(ctx context.Context, _ openapi.GetPortalInfoRequestObject) (openapi.GetPortalInfoResponseObject, error) {
    return openapi.GetPortalInfo200JSONResponse{
        PlaygroundEnabled: h.PlaygroundEnabled,
        LandingVariant:    openapi.PortalInfoLandingVariant(h.LandingVariant),
    }, nil
}
```

**Implementation Notes**:
- The endpoint must be reachable BEFORE auth — register it in the
  unauthenticated route group alongside `/api/auth/oauth/*` and
  `/api/playground/sessions` (which are already public-by-design per
  `cmd/portal/main.go:966`).
- The handler holds a config snapshot at construction (no live re-read
  from the config struct) — config is immutable post-startup in
  jamsesh's design.

**Acceptance Criteria**:
- `GET /api/portal/info` returns 200 with JSON `{playground_enabled,
  landing_variant}` matching the running deploy's config.
- `JAMSESH_LANDING_VARIANT=invalid` fails portal startup with a clear
  error (per `Fail Fast` principle).
- `JAMSESH_LANDING_VARIANT` unset → response carries `"auto"`.
- The endpoint is reachable without an Authorization header.
- Table-driven handler test covers all 6 combinations
  (2 playground states × 3 landing variants).

---

### Unit 2: SPA portalInfo store + landing route handling
**Story**: `story-portal-visitor-entry-pages-spa-landing`
**Depends on**: Unit 1
**Files**:
- `frontend/src/lib/portalInfo.svelte.ts` *(new)* — module-level rune
  store; fetches `/api/portal/info` once on app startup, caches the
  response, exposes `playgroundEnabled` and `landingVariant` via the
  established wrapper-object-rune-store pattern (see
  `.claude/skills/patterns/wrapper-object-rune-store.md`). On fetch
  failure, falls back to `{playgroundEnabled: false, landingVariant:
  'login'}` and logs a warning.
- `frontend/src/lib/portalInfo.test.ts` *(new)* — tests the store:
  successful fetch sets values, failed fetch falls back to login-safe
  defaults, doesn't refetch on second `init()` call.
- `frontend/src/App.svelte` — call `portalInfo.init()` in the
  bootstrap `$effect` alongside the existing `auth.init()`. Update the
  auth-gate logic: when `!auth.isAuthenticated && current.name ===
  'home'`, branch on `portalInfo.landingVariant`:
  - `'project'` → render `ProjectLanding.svelte` (no navigate)
  - `'auto'` + `portalInfo.playgroundEnabled` → `navigate('/playground')`
  - `'auto'` + !playgroundEnabled, or `'login'`, or fallback →
    `navigate('/login')` (existing path)
- `frontend/src/lib/screens/ProjectLanding.svelte` *(new)* — Svelte 5
  component mirroring the `project-landing.html` mockup. Uses
  `navigate()` from the router for internal links (`/playground`,
  `/login`); external links (GitHub, Docs) use plain `<a href>`.
  Imports `tokens.css` via the standard Vite asset pipeline.
- `frontend/src/lib/screens/ProjectLanding.test.ts` *(new)* — tests:
  renders the mockup structure (eyebrow, h1, primary CTA, what-is
  grid, install disclosure, footer); "Try the playground →" click
  calls `navigate('/playground')`; sign-in click calls
  `navigate('/login')`.
- `frontend/src/App.test.ts` *(amend)* — extend the unauthed routing
  test to cover all three landing-variant paths. Mock `portalInfo` per
  the `spa-test-module-mock-barrel` pattern.

```svelte
<!-- frontend/src/App.svelte (auth-gate addition, abbreviated) -->
<script lang="ts">
  import { portalInfo } from '$lib/portalInfo.svelte';
  // ...
  $effect(() => {
    if (!auth.isAuthenticated && current.name === 'home') {
      const v = portalInfo.landingVariant;
      if (v === 'project') return; // render ProjectLanding (below)
      if (v === 'auto' && portalInfo.playgroundEnabled) {
        navigate('/playground'); return;
      }
      navigate('/login'); return;
    }
    // ... existing logic
  });
</script>

{#if current.name === 'home' && !auth.isAuthenticated && portalInfo.landingVariant === 'project'}
  <ProjectLanding />
{:else if current.name === 'home'}
  <Home />
{:else if ...}
  ...
{/if}
```

**Implementation Notes**:
- The `/` route's `requiresAuth: true` declaration in
  `router.svelte.ts:12` stays unchanged. The auth gate special-cases
  the `project` variant *inside* App.svelte's existing gate effect
  rather than declaring `/` as anonymous-public — keeps the auth model
  consistent and avoids a footgun for future routes.
- `portalInfo.init()` MUST resolve before the gate effect makes a
  routing decision; gate the effect on `portalInfo.loaded` (boolean)
  to avoid flashing the wrong UI during the initial fetch.
- Project mode renders `<ProjectLanding />` in-place. We do NOT
  navigate to a separate `/welcome` URL — keeping the URL at `/` means
  the existing `requiresAuth` flow takes over the moment the visitor
  signs in, with no awkward navigation back.

**Acceptance Criteria**:
- Anonymous visitor at `/` with `landing_variant: project` sees the
  ProjectLanding component (matches the mockup structure).
- Anonymous visitor at `/` with `landing_variant: auto` and
  `playground_enabled: true` lands on `/playground`.
- Anonymous visitor at `/` with `landing_variant: auto` and
  `playground_enabled: false` lands on `/login`.
- Anonymous visitor at `/` with `landing_variant: login` lands on
  `/login`.
- Authenticated visitor at `/` sees Home.svelte (org picker) for ALL
  three variants — the flag does not affect authed traffic.
- `/api/portal/info` fetch failure on bootstrap → SPA renders the
  login-bounce path (safe fallback); a warning is logged to console.
- Clicking "Try the playground →" on ProjectLanding navigates to
  `/playground` without a page reload.
- All new components have tests following the established
  `spa-test-module-mock-barrel` and `view-state-union-machine`
  patterns.

---

### Unit 3: Foundation docs roll-forward
**Folded into**: Units 1 & 2 (committed alongside the code change that
makes each assertion true).
**Files**:
- `docs/SELF_HOST.md` — add `JAMSESH_LANDING_VARIANT` row to the
  Reference table (§2). Note the default is `auto`. Add a one-paragraph
  subsection under §2 explaining the three modes and the
  playground-discoverability rationale.
- `docs/UX.md` — add an "Anonymous visitor at `/`" entry under the
  "Portal UI surfaces" section, listing the three variants and what
  each renders. Cross-reference the project-landing mockup path.

**Acceptance Criteria** (per the rolling-foundation principle):
- Docs describe the system as it is post-implementation; no
  "previously" prose.
- Reference table entry uses the same column shape as adjacent rows.
- UX.md links to the mockup paths.

---

## Implementation Order

1. **Unit 1** (backend): config knob + endpoint + OpenAPI + tests.
   Lands first because Unit 2 depends on the endpoint existing.
2. **Unit 2** (frontend): portalInfo store + App.svelte gate +
   ProjectLanding component + tests.
3. **Unit 3** (docs): folded into the commits above; SELF_HOST.md with
   Unit 1, UX.md with Unit 2.

`implement-orchestrator` will wave these sequentially given the
`depends_on` chain — no fan-out parallelism available within the
feature, but the two stories give clear gate-friendly surfaces (backend
handler test vs. frontend component test).

## Testing

### Unit 1 tests: `internal/portal/portalinfo/handler_test.go`
- Table-driven, one row per `(playground_enabled, landing_variant)`
  combination (4-6 rows). Use the established `testenv-harness-struct`
  pattern from `.claude/skills/patterns/testenv-harness-struct.md` for
  wiring.
- Config validation: `internal/portal/config/config_test.go` (amend)
  — invalid `LandingVariant` value returns error; default applied when
  field is empty.

### Unit 2 tests: frontend
- `frontend/src/lib/portalInfo.test.ts` — fetch success / fetch failure
  paths, idempotent `init()`.
- `frontend/src/lib/screens/ProjectLanding.test.ts` — structural
  rendering, link interactions.
- `frontend/src/App.test.ts` (amend) — the 6 routing combinations
  spelled out in Unit 2's acceptance. Use `spa-test-module-mock-barrel`
  for `portalInfo` and `auth` mocks.

### Integration / smoke
- Existing `tests/e2e/` Playwright suite covers `/` and `/playground`
  flows for the auto-with-playground path. Add one new spec covering
  anonymous `/` with `JAMSESH_LANDING_VARIANT=project` rendering the
  ProjectLanding component. The Login and auto-without-playground
  paths are already exercised.

## Risks

- **Risk**: SPA bootstrap order — if `portalInfo.init()` fires AFTER
  the first gate effect runs, anonymous `/` could briefly flash the
  wrong UI (Home.svelte → ProjectLanding) before stabilizing.
  *Mitigation*: explicit `portalInfo.loaded` gating, render a tiny
  loading shell (transparent) until both `auth.init()` and
  `portalInfo.init()` resolve. Pattern mirrors `auth.svelte.ts`'s
  existing `auth.ready` gate.
- **Risk**: openapi regeneration churn — adding a new path may regenerate
  unrelated boilerplate in `*.gen.go` files.
  *Mitigation*: review the diff carefully before commit; the
  generation is deterministic so churn outside `PortalInfo` is a sign
  something else changed. If the diff is excessive, run regen on
  unchanged spec first to baseline.
- **Risk**: project-landing mockup design diverges from the
  established `01-prospect-landing.html` voice during implementation.
  *Mitigation*: the Svelte component MUST link the same
  `tokens.css` and inherit identical font/colour/spacing tokens;
  visual regression is caught by the structural component test +
  the Playwright spec rendering the actual page.

## Out of scope

- The CLI share-URL bug (separate story
  `story-fix-cli-playground-share-url`).
- A redesign of `/playground` (`PlaygroundLanding.svelte`) — the
  existing component serves auto-mode unchanged.
- An admin UI surface to flip `JAMSESH_LANDING_VARIANT` at runtime —
  it's a deploy-time env var, restart required.
- Email-capture, analytics, or marketing pixels on the project
  landing.

## Children complete (2026-05-24)

Both child stories implemented and advanced to terminal-or-review:

- `story-portal-visitor-entry-pages-info-endpoint` → `stage: done`
  (reviewed; one inline nit re: release-notes call-out for the
  intentional behavior change on upgrade).
- `story-portal-visitor-entry-pages-spa-landing` → `stage: review`
  (just landed; 723/723 frontend tests + Go build clean).

**Cross-cutting verification**: `go build ./...` clean,
`pnpm test` in `frontend/` shows 723/723 green across 60 files,
`go test ./...` clean (verified after wave 1).

**No cross-cutting deviations**. The chosen Option 1 mockup
(`.mockups/screens/feature-portal-visitor-entry-pages/option-1.html`)
is mirrored by `frontend/src/lib/screens/ProjectLanding.svelte`; the
new public endpoint exposed by Unit 1 is consumed by the portalInfo
store in Unit 2 exactly as designed; the auth-gate special-case for
the `project` variant lives in `App.svelte`'s existing gate effect
(no new route registration), as specified.

Advancing feature to `stage: review` — both children meet the
terminal-or-review bar.
