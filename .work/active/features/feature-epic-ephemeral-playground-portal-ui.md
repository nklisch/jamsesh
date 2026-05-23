---
id: feature-epic-ephemeral-playground-portal-ui
kind: feature
stage: drafting
tags: [ui, portal, playground]
parent: epic-ephemeral-playground
depends_on: [feature-epic-ephemeral-playground-session-lifecycle]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

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
