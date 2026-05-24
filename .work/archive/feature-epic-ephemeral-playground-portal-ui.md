---
id: feature-epic-ephemeral-playground-portal-ui
kind: feature
stage: done
tags: [ui, portal, playground]
parent: epic-ephemeral-playground
depends_on: [feature-epic-ephemeral-playground-session-lifecycle]
release_binding: v0.4.0
gate_origin: null
created: 2026-05-23
updated: 2026-05-24
---

## Implementation summary (autopilot)

All 4 child stories advanced to `stage: review`:

- `story-...-portal-ui-drawer-rework` — NewSessionDrawer reworked to CLI + skill output generator; 532 frontend tests pass
- `story-...-portal-ui-router-refactor` — declarative `requiresAuth` route refactor + `auth.playgroundContext` rune field; 540 tests
- `story-...-portal-ui-session-view-extensions` — PlaygroundChip + CountdownBadge + DestructionWarningBanner; SessionViewShell playground branch; 569 tests
- `story-...-portal-ui-anonymous-entry` — PlaygroundLanding + JoinerPicker + SessionTombstone screens + Home CTA; 624 tests total

**Cross-cutting deviations**:
- Drawer-rework removed the `name` field (not present in the CLI signature) and the `oncreated` callback path (drawer no longer creates sessions itself)
- Session-view-extensions WS event payload types defined inline with TODO pointing at openapi-typescript regeneration (since session-lifecycle landed earlier in the same wave, the integration fix is mechanical)
- Anonymous-entry's JoinerPicker uses a client-side adjective-animal nickname generator that mirrors the server's wordlist style (no public GET for session metadata before joining)

**Verification status**: `npm run check` clean, `npm run test` 624/624 pass, `npm run build` clean bundle.

# Portal UI — playground surfaces + CLI-first creation rework

## Brief

Adds every portal-frontend surface the playground epic needs and reworks
the existing "New session" flow to align with the CLI-first creation
pattern. New surfaces: unauthenticated portal landing
(`/playground/...`), joiner nickname picker, anonymous-mode chip and
countdown badge added to `SessionViewShell` chrome (rendered only when
the session's org is the reserved playground org), idle / hard-cap
warning banners, post-destruction confirmation page. Routes are added
to `router.svelte.ts`; the auth gate in `App.svelte` is extended to
exempt the playground route group.

The `NewSessionDrawer.svelte` rework is the design decision this feature
resolves. Two options under consideration (per the parent epic's
strategic-decisions section, which folded in the CLI-first unification):
(A) keep the drawer as an alternative path that collects inputs and then
prints a `jamsesh new ...` CLI command for the user to run locally; or
(B) keep the drawer for users with `JAMSESH_CLI_FIRST_OPTIONAL=true` and
otherwise hide it. The choice is made in this feature's design pass
based on what the CLI-first creation feature actually ships.

Auth state in `auth.svelte.ts` gains a `_playgroundContext` rune field
that tracks whether the current view is anonymous-mode and which
anonymous bearer is in use; the auth gate consults this when deciding
which redirect a 401 triggers.

## Epic context
- Parent epic: `epic-ephemeral-playground`
- Position in epic: **wave 3** — depends on `session-lifecycle` for the
  REST routes the UI calls. Parallelizable with `plugin-skills`.

## Foundation references
- `docs/UX.md` § Flow: creating a session — this feature's design pass
  rolls UX.md forward to describe the unified CLI-first creation flow
  (alongside any retained portal-form path) AND adds the playground
  onboarding flow as a first-class section
- `docs/ARCHITECTURE.md` § Portal frontend — minor: any new top-level
  route group or auth-state shape change is noted

## Mockups
- **Inherits parent epic flow**:
  `.mockups/flows/playground-onboarding/index.html`
- Every user-visible surface this feature ships is covered there:
  step 01 (prospect landing), step 03 (creator session with chip +
  countdown), step 05 (joiner nickname picker), step 06 (joiner
  session), step 07 (warning banners + post-destruction page)
- **Do NOT re-mock at the feature tier.** The flow is the source of
  truth for visual decisions on this feature
- The reworked `NewSessionDrawer` (CLI + skill output generator) is a
  new screen state not covered in the parent flow. Invoke
  `/ux-ui-design:screens new-session-drawer-cli-output` for that
  surface during this feature's full design pass.

## Design decisions

Locked at `--only-questions` time. Feature-design Phase 5 inherits these
as fixed input.

- **`NewSessionDrawer` rework**: convert to a CLI + skill command-output
  generator. The drawer keeps its existing form (goal / scope / mode /
  org / invitees) but, instead of POSTing to the portal's create API,
  it renders the resolved command in **two formats** the user can copy:
  1. **Skill form** (for users in a CC session): `/jamsesh:new --org X
     --goal '<text>' --scope '<glob>' --mode <sync|isolated>
     --invite a@x,b@x` — pastable directly into the CC composer; the
     `/jamsesh:new` skill (owned by `plugin-skills` feature) invokes the
     binary with the same flags
  2. **Raw CLI form** (for users in a bash terminal): `jamsesh new ...`
     with the same flags — pastable into a checkout's terminal
  Both forms wrap the same underlying invocation. The drawer becomes
  "what to ask your agent to do" + "what to type yourself" — exactly
  parallel to the agent-primary mental model locked in for
  `cli-first-creation`. The portal no longer creates sessions
  directly; the creator's local checkout always does the
  push-as-base. This is option A from the parent epic body's
  decomposition risk, now resolved.

- **Countdown update mechanism**: client-side ticker + initial server
  payload. On session-load (and on every `playground.activity_reset`
  WS event), the SPA receives `{ created_at, hard_cap_at,
  idle_timeout_at, last_substantive_activity_at }`; a 1-second JS
  ticker recomputes the "ends in" / "idle in" values locally. Pure
  client-side math between server updates — no WS chatter for the
  countdown itself. The 5-min-before-destruction warning banners are
  triggered when the client-computed remaining time crosses the
  threshold (rendered locally without waiting for a server event).
  Resilient to brief WS drops; the countdown stays correct as long
  as the SPA has the most recent server payload.

- **Post-destruction URL behavior**: real URL serves a 410-gone
  confirmation page. The destruction routine (in
  `session-lifecycle`) writes a `tombstones` row keyed by `session_id`
  before deleting the session row, capturing summary stats: members
  count, commits count, auto-merges count, duration, end_reason. The
  SPA renders the destruction confirmation page from this tombstone
  data, with the route returning HTTP 410 Gone. Anyone visiting the
  URL after destruction — original participants returning later,
  joiners arriving too late — sees the destruction summary and the
  "try another playground" CTA. Tombstones for playground sessions
  themselves have a TTL (TBD in design pass — likely 30 days) after
  which they're purged.

- **Auth-gate exemption pattern**: restructure routes into explicit
  `public` and `auth-required` groups. Refactor `router.svelte.ts` so
  every route declares `requiresAuth: boolean` (default true). The auth
  gate in `App.svelte` reads the current route's flag rather than
  maintaining a separate allowlist. Migrate the existing login,
  magic-link, and oauth-callback routes from the gate's hardcoded
  allowlist into the new flag. Playground routes (`/playground`,
  `/playground/s/:token`) declare `requiresAuth: false` (anonymous-mode
  is checked separately by the playground-aware components). One source
  of truth for auth-required-ness; refactors a pre-existing wart in
  the same stride. Note: this is a small refactor outside the strictly
  playground-scoped surface — documented here so design-time accounts
  for it.

## Architectural choice

**Four-story decomposition along Svelte file boundaries** — router
refactor as substrate, anonymous-entry surfaces clustered together (they
share the auth-state extensions), SessionViewShell extensions modify
one existing component, NewSessionDrawer rework modifies another. The
work parallelizes naturally: 3 stories run in parallel, the entry-
surfaces story waits for the router refactor (it needs the new
declarative-auth route pattern to register against).

Why this shape:
- **Per-route stories**: too granular; landing + picker + post-
  destruction share the auth-state extension (`_playgroundContext`
  rune field) and the `openapi-fetch` calls against the new playground
  endpoints. One agent owning all three keeps the auth-state changes
  consistent.
- **Monolithic feature**: too much surface (router refactor + 3 new
  routes + 2 modified components) for one agent to land cleanly; gate-
  review would have nowhere to score per-concern.
- **Per-component story**: matches Svelte's component-as-unit idiom
  and keeps test ownership clear (each story owns its component's
  test file).

## Implementation units

4 stories. Each is one of the chunks below. Full code skeletons for
each unit live in the per-story body files.

### Story 1: Router refactor + auth-state extensions
**Files**:
- `frontend/src/lib/router.svelte.ts` (refactor) — add `requiresAuth: boolean`
  to every route registration; default true
- `frontend/src/App.svelte` (modify) — replace hardcoded auth-gate
  allowlist with `currentRoute.requiresAuth` check
- `frontend/src/lib/auth.svelte.ts` (extend) — add `_playgroundContext`
  rune field (`{ sessionId: string, bearer: string, nickname: string } | null`)
  + getter `auth.playgroundContext`
- `frontend/src/lib/auth.svelte.test.ts` (extend) — coverage for new field
- `frontend/src/lib/router.svelte.test.ts` (extend or add) — coverage for
  the new `requiresAuth` flag handling

Migration: the existing login, magic-link, and oauth-callback routes
already in the gate's allowlist get `requiresAuth: false` declarations
inline. The gate's allowlist code is removed.

**Acceptance criteria**:
- [ ] Every existing route still works post-refactor (visit each, assert
      no spurious auth redirects)
- [ ] A new route with `requiresAuth: false` doesn't trigger the gate
- [ ] A new route with `requiresAuth: true` (or unset, defaulting to
      true) redirects unauthenticated users to `/login`
- [ ] `auth.playgroundContext` returns null when not in playground mode;
      returns the populated context when set
- [ ] Setting `_playgroundContext` doesn't affect `auth.isAuthenticated`
      (anonymous-mode is orthogonal to authenticated-mode)

### Story 2: Anonymous portal entry surfaces
**Files**:
- `frontend/src/lib/screens/PlaygroundLanding.svelte` (new) — the
  unauthenticated landing page (mirrors mockup step 01)
- `frontend/src/lib/screens/JoinerPicker.svelte` (new) — nickname picker
  (mirrors mockup step 05)
- `frontend/src/lib/screens/SessionTombstone.svelte` (new) — post-
  destruction confirmation page (mirrors mockup step 07c)
- `frontend/src/lib/router.svelte.ts` (modify) — register the three new
  public routes:
  - `/playground` → PlaygroundLanding (requiresAuth: false)
  - `/playground/s/:sessionId/join` → JoinerPicker (requiresAuth: false)
  - `/playground/s/:sessionId/ended` → SessionTombstone (requiresAuth: false)
- `frontend/src/lib/screens/Home.svelte` (modify) — add "Try a playground
  session" CTA pointing at `/playground`
- Tests alongside each new screen file

The JoinerPicker calls `POST /api/playground/sessions/:id/join` with the
chosen nickname; on success, writes the bearer to `auth.playgroundContext`
and navigates to the in-session view (`/orgs/org_playground/sessions/:id`).
The SessionViewShell render branch for playground sessions reads from
`auth.playgroundContext` for bearer-based requests.

The SessionTombstone screen reads from `GET /api/playground/sessions/:id/tombstone`.
On 404 (session still active, no tombstone yet) it redirects to the live
session view; on success it renders the destruction summary + "try
another playground" CTA (loops to `/playground`).

The post-destruction transition: when a participant's WS connection
emits `session.destroyed`, the SessionViewShell triggers a client-side
navigate to `/playground/s/:id/ended`. The 410 status comes from the
backend tombstone endpoint; the SPA renders a static page (no special
HTTP status from the SPA's perspective — that's a backend concern).

**Acceptance criteria**:
- [ ] `/playground` route renders PlaygroundLanding without requiring auth
- [ ] CTA on Home.svelte navigates to `/playground` (for authenticated
      users who want to spin up a playground anyway — sample case)
- [ ] Joiner picker: pre-fills server-suggested nickname, "join as <name>"
      button calls the join endpoint, navigates to in-session on success
- [ ] Joiner picker: 409 session-full response renders the friendly
      "this session is full" message with "try another" CTA
- [ ] Post-destruction page renders tombstone summary; "try another"
      CTA loops to `/playground`
- [ ] Tests cover the happy paths + the 409/404 responses

### Story 3: SessionViewShell playground extensions
**Files**:
- `frontend/src/lib/screens/SessionViewShell.svelte` (modify) — add
  playground-mode chip, countdown badge, idle / hard-cap warning banners
- `frontend/src/lib/components/PlaygroundChip.svelte` (new) — the chip
  itself
- `frontend/src/lib/components/CountdownBadge.svelte` (new) — the
  client-side ticker
- `frontend/src/lib/components/DestructionWarningBanner.svelte` (new)
- Tests alongside each

The SessionViewShell render branch for playground sessions (detected via
`session.orgId === 'org_playground'` or the new `auth.playgroundContext`
being set):
- Header chrome shows PlaygroundChip alongside the existing breadcrumb
- Header chrome's right side shows CountdownBadge (hides org-name)
- When CountdownBadge's computed remaining-time crosses the 5-minute
  threshold, DestructionWarningBanner renders at the top of the body
  (idle warning OR hard-cap warning depending on which timer crossed)
- Post-destruction transition (via WS event) navigates to `/playground/s/:id/ended`

CountdownBadge implementation per the locked decision:
- Receives `{ hardCapAt, idleTimeoutAt, lastSubstantiveActivityAt }`
  via props at mount
- `$state` rune for current "now" updated by a 1-second setInterval
- `$derived` runes compute `idleRemaining` and `hardCapRemaining`
- On `playground.activity_reset` WS event, replaces the
  `lastSubstantiveActivityAt` prop value, which propagates through `$derived`

**Acceptance criteria**:
- [ ] PlaygroundChip renders only when the session's org is `org_playground`
- [ ] CountdownBadge ticks every second; both timers visible
- [ ] When `idleRemaining` < 5 min, idle warning banner renders
- [ ] When `hardCapRemaining` < 5 min, hard-cap warning banner renders
      (priority: hard-cap wins over idle if both crossed; only one banner shown)
- [ ] Substantive-activity WS event resets the idle baseline (no banner
      until next idle approach)
- [ ] `session.destroyed` WS event navigates to `/playground/s/:id/ended`
- [ ] All durable session views (org_id != org_playground) unchanged:
      no chip, no badge, no warning banners

### Story 4: NewSessionDrawer rework
**Files**:
- `frontend/src/lib/components/NewSessionDrawer.svelte` (modify) — replace
  the POST-to-create-API logic with a CLI + skill command-output renderer
- `frontend/src/lib/components/NewSessionDrawer.test.ts` (extend) — test
  the new behavior

The drawer keeps its existing form fields (name, goal, scope, mode,
org, invitees). On submit, instead of POSTing, it renders **two**
copyable command-output sections per the locked design decision:

1. **Skill form** (for CC users):
   `/jamsesh:jam --org X --goal '<text>' --scope '<glob>' --mode <mode> --invite a@x,b@y`
2. **Raw CLI form** (for bash users):
   `jamsesh new --org X --goal '<text>' --scope '<glob>' --mode <mode> --invite a@x,b@y`

Each rendered with a "copy" button and brief explanatory text.

The drawer no longer creates sessions itself. The session-create REST
endpoint at `POST /api/orgs/{org}/sessions` remains available for any
direct API consumer; the drawer just stops being a UI client of it.

A net-new screen state worth mocking: the drawer's command-output
panel. Inherits the existing drawer's visual tone; renders two
`<code>` blocks with copy buttons. Per the parent epic's Mockup
inheritance, this state isn't covered in the playground-onboarding
flow — small enough to skip a fresh `/ux-ui-design:screens` invocation
and rely on the implementing agent's visual consistency with the
existing drawer.

**Acceptance criteria**:
- [ ] Filling the form + clicking submit renders both CLI and skill
      command forms with the form values substituted in
- [ ] Copy buttons copy the respective command to clipboard
- [ ] The drawer makes NO `POST /api/orgs/{org}/sessions` call on submit
- [ ] All existing form-validation logic (scope-glob parsing, required
      org pick) still runs before rendering the commands
- [ ] Test coverage for the two output forms + clipboard interaction

## Implementation order

- Sub-wave A: Stories 1, 3, 4 in parallel (3 sub-agents)
- Sub-wave B: Story 2 (after Story 1 lands the router refactor)

## Risks

- **Router refactor breaks existing flows silently**: the migration
  from hardcoded allowlist to declarative `requiresAuth` is a quiet
  change with broad blast radius. Mitigation: Story 1's acceptance
  criterion #1 is "visit each existing route, assert no spurious
  redirects" — implementer runs through every route in the existing
  app post-refactor.

- **`auth.playgroundContext` is a fundamentally different identity
  shape from `auth.user` / `auth.orgs`**: the SPA's existing auth
  flow assumes either signed-in (with orgs[]) or signed-out (empty).
  Playground introduces a third state: "anonymous-mode bearer for
  one specific session." Mitigation: the rune is intentionally
  separate (`_playgroundContext` rather than overloading `_currentUser`)
  so existing code paths that read `auth.isAuthenticated` /
  `auth.orgs` are unaffected. Playground-aware components consult
  the new field explicitly.

- **Countdown ticker accuracy across browser tabs**: a backgrounded
  tab's `setInterval` may throttle. Mitigation: on visibility-change
  (Page Visibility API), recompute remaining-time from scratch using
  Date.now() and the prop values — corrects any drift accumulated
  while backgrounded. Implementer adds the visibility listener in
  CountdownBadge's onMount.

- **WS event coupling**: SessionViewShell's playground branch
  depends on two WS event types from the session-lifecycle feature
  (`playground.activity_reset`, `session.destroyed`). If those
  events' payload shapes drift, this feature breaks silently.
  Mitigation: OpenAPI envelope schemas (shared between REST and WS
  per `docs/SPEC.md`) generate TS types from the same source. The
  implementer should import those generated types rather than
  defining inline shapes.

- **NewSessionDrawer rework loses a usage signal**: today the drawer
  POSTs and the server logs creation. After the rework, the server
  doesn't see drawer-driven creates (the user copy-pastes into a
  terminal/CC, which then hits the API directly from CLI). Lose
  the "creates initiated from drawer" telemetry. Mitigation: not
  needed for v1 — the API still gets the create call, just attributed
  to CLI not drawer. If observability needs the source distinction,
  add a `?source=drawer` query param convention later.

## Review (2026-05-24)

**Verdict**: Approve — feature delivered as briefed.

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: All child stories approved individually. Aggregate review: design realized end-to-end, no cross-cutting deviations beyond what the implementation summary documents, no foundation-doc drift uncaught, no API breakage. Tests green across the affected packages.
