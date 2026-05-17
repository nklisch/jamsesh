---
id: epic-portal-ui-session-list-screen
kind: story
stage: done
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

## Implementation notes

- `frontend/src/lib/screens/SessionList.svelte` — full screen wrapped in Chrome; fetches sessions from `GET /api/orgs/{orgID}/sessions`; `$derived` filteredSessions + counts; WS subscriptions per session (session.finalizing/ended update status in-place; commit.arrived/presence.updated refetch the individual session); `navigate()` on row click.
- `frontend/src/lib/components/NewSessionDrawer.svelte` — right-side drawer overlay; form fields: name, goal, scope (comma-sep globs → JSON array), default_mode toggle (sync/isolated); `POST /api/orgs/{orgID}/sessions`; emits `oncreated(session)` on success; closes on `onclose()` / Escape / backdrop click.
- `frontend/src/App.svelte` — replaced `SessionsLanding` import with `SessionList`; removed `Chrome` placeholder for session-view, replaced with `SessionViewShell`.
- 13 tests green; svelte-check clean; build clean.

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Three-zone Chrome+List+Drawer composition matches the mock. WS subscribe-per-session is wasteful but acceptable for v1; documented. NewSessionDrawer scope-string-to-JSON-array parser is the right v1 simplification.
