---
id: story-epic-ephemeral-playground-portal-ui-anonymous-entry
kind: story
stage: done
tags: [ui, playground]
parent: feature-epic-ephemeral-playground-portal-ui
depends_on: [story-epic-ephemeral-playground-portal-ui-router-refactor]
release_binding: v0.4.0
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Anonymous portal entry surfaces

## Scope

Story 2 of the parent feature. Three new Svelte screens for the
anonymous-mode entry path:

1. **PlaygroundLanding** (`/playground`) — unauthenticated landing page
   with CLI-install pitch and CTA (mirrors mockup step 01)
2. **JoinerPicker** (`/playground/s/:sessionId/join`) — nickname picker
   that calls `POST /api/playground/sessions/:id/join` (mirrors mockup
   step 05)
3. **SessionTombstone** (`/playground/s/:sessionId/ended`) — post-
   destruction confirmation page (mirrors mockup step 07c)

Plus a "Try a playground session" CTA added to the existing Home.svelte
for signed-in users who want to spin up a playground anyway.

Full design in the parent feature body's "Story 2" section.

## Files delivered

- `frontend/src/lib/screens/PlaygroundLanding.svelte` (new) + test
- `frontend/src/lib/screens/JoinerPicker.svelte` (new) + test
- `frontend/src/lib/screens/SessionTombstone.svelte` (new) + test
- `frontend/src/lib/router.svelte.ts` (modify) — register 3 new public routes
- `frontend/src/lib/screens/Home.svelte` (modify) — add playground CTA

## Acceptance criteria

See the parent feature body's "Story 2 acceptance criteria" section.

## Notes for the implementing agent

- All three screens declare `requiresAuth: false` in the router. The
  Story-1 router refactor must be landed before this story can register
  the routes (depends_on declared).
- JoinerPicker's submit handler:
  1. POST /api/playground/sessions/:id/join with `{ nickname }` body
  2. On 200: write returned bearer to `auth.playgroundContext`,
     navigate to `/orgs/org_playground/sessions/:id`
  3. On 409 (session full): show friendly "this session is full"
     message with "try another playground" CTA
  4. On 410 (session ended): redirect to `/playground/s/:id/ended`
- SessionTombstone reads from
  `GET /api/playground/sessions/:id/tombstone`:
  - On 404 (active session, no tombstone yet): redirect to the live
    session view
  - On 200: render summary stats + CTAs
- All screens inherit the design tokens from `.mockups/design-system/tokens.css`
  via the existing global stylesheet inclusion
- Use `openapi-fetch` (generated client) for all REST calls — pattern
  matches the existing screens' API usage

## Implementation notes

All three screens implemented and registered. Key decisions made during
implementation:

**PlaygroundLanding** — Static landing with two-step CLI install card
(plugin install + `jamsesh playground new`), ephemeral-session warning
note, and three feature cards (Real git / Auto-merger / Addressed
comments). Copy-to-clipboard on both command blocks with a "Copied!"
label feedback. Top bar has "Sign in →" link; footer has "Sign up for a
durable account" link.

**JoinerPicker** — No public GET exists for session metadata before
joining (the GET `/api/playground/sessions/{id}` requires a bearer that
only exists post-join). Pre-fill uses a client-side adjective-animal
nickname generator that mirrors the server's wordlist style. User can
edit the suggested nickname or reroll it. Nickname validation:
`/^[a-z0-9][a-z0-9-]{0,22}[a-z0-9]$|^[a-z0-9]{2}$/` (2–24 chars,
lowercase). On 409: friendly full-state with "Try another playground"
CTA. On 410: redirect to `/playground/s/:id/ended`. On 200: calls
`auth.setPlaygroundContext` with `{ sessionId, bearer, nickname }` from
the join response (confirmed nickname, not the requested one), then
navigates to `/orgs/org_playground/sessions/:id`.

**SessionTombstone** — Fetches tombstone on mount with try/catch for
transport-level failures (pattern: try/catch wraps the await, not
`try { data } catch`). On 404 (active session): redirect to live view.
On 200: renders stats (members, commits, auto-merges, duration formatted
as `Xh Ym` or `Ym`). Error state has "Try again" button that re-fires
the GET.

**Router** — Three new routes appended after `org-settings`:
`playground`, `playground-join` (`:sessionId`), `playground-ended`
(`:sessionId`). All `requiresAuth: false`.

**App.svelte** — Three new `{:else if}` branches importing the new screens.

**Home.svelte** — `playgroundCta` snippet rendered below the create form
in both empty-state and picker-state branches. Separated by a border-top
rule with "Just exploring?" prefix label.

**Types** — ran `npm run generate` to regenerate types.gen.ts from the
updated openapi.yaml (playground schemas were added by the
session-lifecycle-rest-endpoints story).

**Test counts**: PlaygroundLanding 14, JoinerPicker 22, SessionTombstone 19.
All 624 suite tests pass. `npm run check` 0 errors. `npm run build` clean.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: 1
- Tombstone 404 conflates two cases (session still active vs tombstone
  TTL expired). Current code redirects all 404s to the live session
  view, which loops for 30+ day-old URLs. Filed as
  `.work/backlog/idea-tombstone-expired-redirect-distinguishes-active-vs-expired.md`.
  Low priority — only affects users hitting expired-tombstone URLs.

**Nits** (not items):
- `JoinerPicker.svelte` line 2 imports `onMount` but never uses it; line 8
  aliases `PlaygroundSessionSummary` but never references it. Minor
  cleanup — would be flagged by `noUnusedLocals` / lint if enabled.
- `PlaygroundLanding.copyCmd`: rapid double-click within 1.6s captures
  "Copied!" as `orig`, restoring "Copied!" instead of "Copy". Cosmetic.
- Clipboard write failures fail silently. Comment says "the text is still
  visible" which is true, but an aria-live "Copy failed — select
  manually" would be slightly more accessible.

**Notes**:
- 3 screens, 3 new public routes, 1 Home CTA. 55 new tests, all 624
  pass. `npm run check` clean.
- Design deviation (client-side nickname suggestion vs design's
  server-suggested) is documented inline with rationale (no public GET
  endpoint exists pre-join). Acceptable.
- Pattern compliance: `view-state-union-machine`,
  `openapi-fetch-result-branch`, `wrapper-object-rune-store`,
  `spa-test-module-mock-barrel`, `snippet-children-component`
  (Home.svelte `playgroundCta`). All clean.
