---
id: story-epic-ephemeral-playground-portal-ui-anonymous-entry
kind: story
stage: implementing
tags: [ui, playground]
parent: feature-epic-ephemeral-playground-portal-ui
depends_on: [story-epic-ephemeral-playground-portal-ui-router-refactor]
release_binding: null
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
