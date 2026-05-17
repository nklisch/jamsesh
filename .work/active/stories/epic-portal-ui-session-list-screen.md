---
id: epic-portal-ui-session-list-screen
kind: story
stage: implementing
tags: [ui]
parent: epic-portal-ui-session-list
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Session List — Screen + New Session Drawer

## Scope

Implement the SessionList screen + NewSessionDrawer; wire into routing.

## Units delivered

- `frontend/src/lib/screens/SessionList.svelte`
- `frontend/src/lib/components/NewSessionDrawer.svelte`
- `frontend/src/App.svelte` (edit) — route to SessionList
- Tests

## Acceptance Criteria

- [ ] Renders sessions from `client.GET('/api/sessions')` per `.mockups/screens/epic-portal-ui-session-list/option-1.html`
- [ ] Filter chips toggle (all/active/finalizing/ended) with correct counts
- [ ] "New session" button opens drawer; submit POSTs to portal; new session appears in list
- [ ] WS subscription updates row in place when commit.arrived / session.finalizing / session.ended / presence.updated arrives
- [ ] Tests use mocked client + mocked ws.subscribe via injection

## Notes

- Design-system components in use: Card, Badge, ModePill, AuthorDot, Button.
- Use Svelte 5 runes + snippets per the project conventions.
- Mockup HTML lives at `.mockups/screens/epic-portal-ui-session-list/option-1.html` — implement faithfully.
