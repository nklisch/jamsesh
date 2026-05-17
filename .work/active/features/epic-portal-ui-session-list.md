---
id: epic-portal-ui-session-list
kind: feature
stage: implementing
tags: [ui]
parent: epic-portal-ui
depends_on: [epic-portal-ui-foundation, epic-portal-ui-design-system]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
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

## Mockups

- Screens: `.mockups/screens/epic-portal-ui-session-list/index.html`
- Selected: **option-1 (row cards, large)** — 2026-05-16
- Rationale: matches the journey direction already locked in the onboarding
  flow's step 3. Full context per row reads well at the expected scale
  (5-15 active sessions per user). Filter chips group by status; freshly-
  invited session gets an accent border. Per-row contents include scope
  chips, member strip, mode pill, commit count, and live recency.

**Layout primitives this commits to:**

- Single-column vertical stack of row cards under a sticky page chrome
  (wordmark, org chip, theme chip, "New session" button, avatar)
- Filter chips (All / Active / Finalizing / Ended) with counts
- Per-row content blocks: left side carries name + status pill + goal text
  + meta strip (mode pill, member count, commit count, scope chips);
  right side carries author strip + live recency
- Status pills inline with the session name (e.g., `finalizing`, `ended`)
- "Just invited" sessions get an accent border + `new` badge
- Ended sessions render at reduced opacity with `bg-tertiary`

## Design decisions

- **New session button**: opens an inline drawer/modal in the portal UI calling POST /api/sessions (matching the locked sign-in card's portal-side flow).
- **Mockup source**: `.mockups/screens/epic-portal-ui-session-list/option-1.html` is the locked design.
- **WebSocket subscriptions**: v1 opens N concurrent WS connections, one per session in the list. Wasteful but simple. Future optimization: server-side multiplex.
- **Pagination**: cursor-based with infinite scroll (load more on scroll-bottom).
- **Single story**: `epic-portal-ui-session-list-screen`.

## Implementation Units

### Unit 1: SessionList screen

**File**: `frontend/src/lib/screens/SessionList.svelte`

State: sessions array, activeFilter (all|active|finalizing|ended), nextCursor, isLoading. On mount: fetch `/api/sessions`, subscribe to each session's WS feed. Row component renders per the mockup.

### Unit 2: NewSessionDrawer

**File**: `frontend/src/lib/components/NewSessionDrawer.svelte`

Form modal: name, goal, scope (textarea comma-sep globs → JSON array), default_mode (toggle), invitees (multiline emails). On submit calls `client.POST` to the org's sessions endpoint.

### Unit 3: Routing wire-up

`App.svelte` route `/orgs/<orgID>/sessions` → SessionList. Update from the SessionsLanding placeholder.

## Testing

- Renders list of sessions
- Filter chip filters work
- New session drawer submits + prepends new session
- Live WS event updates row in place
