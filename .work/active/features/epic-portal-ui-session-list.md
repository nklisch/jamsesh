---
id: epic-portal-ui-session-list
kind: feature
stage: drafting
tags: [ui]
parent: epic-portal-ui
depends_on: [epic-portal-ui-foundation, epic-portal-ui-design-system]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal UI — Session List

## Brief

The session-list view from the onboarding flow's step 3. Lists sessions the
authenticated user is a member of, grouped by status (Active / Finalizing /
Ended). Each row shows session name, goal preview, member dots, commit
count, scope chips, and last-activity recency. Filter chips at the top
toggle between status groups; a "New session" button in the chrome opens
the new-session flow (see boundary note below). The freshly-invited
session is highlighted with an accent border per the mockup.

Subscribes to portal WebSocket events: `commit.arrived`,
`session.finalizing`, `session.ended`, `presence.updated` — to keep the
list live without a page reload.

Does NOT cover: the inside of a session (`session-view-shell`); the
new-session-creation wizard itself (see Decomposition risks).

## Epic context

- Parent epic: `epic-portal-ui`
- Position in epic: first user-facing surface after login. Independent of
  the session view (parallel with `session-view-shell`).

## Foundation references

- `docs/UX.md` — Portal UI surfaces > Session list
- `docs/PROTOCOL.md` — REST API > Sessions > `GET /api/sessions`,
  WebSocket event types
- `docs/ARCHITECTURE.md` — Multi-tenancy (sessions scoped to user's org)
- `.mockups/flows/onboarding/03-session-list.html` — locked design

## Decomposition risks (carried from epic pre-mortem)

- The "New session" button's behavior is unresolved: open a wizard inside
  the portal UI, or redirect the user to the `/jamsesh:create` slash
  command in CC? Feature-design must pick one and document the boundary
  with `epic-cc-plugin` and `epic-portal-api` (which owns the create
  endpoint).

<!-- Feature-design will fill in component structure, data fetching, and
the new-session-flow resolution when /agile-workflow:feature-design runs
on this. -->
