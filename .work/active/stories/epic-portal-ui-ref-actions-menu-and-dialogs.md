---
id: epic-portal-ui-ref-actions-menu-and-dialogs
kind: story
stage: implementing
tags: [ui]
parent: epic-portal-ui-ref-actions
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Ref Actions — Context Menu + Dialogs + ref-modes endpoint

## Scope

Add backend ref-modes endpoint + RefActionsMenu + ForkDialog + ModeSwitchDialog; wire into TreeDag.

## Units delivered

- Backend:
  - `internal/portal/sessions/refmodes.go` — `POST .../ref-modes` with `{ref, mode}`; UpsertRefMode + emit mode.changed event
  - openapi.yaml + regen
- Frontend:
  - `frontend/src/lib/components/RefActionsMenu.svelte`
  - `frontend/src/lib/components/ForkDialog.svelte` — calls MCP fork tool via the existing `/mcp` endpoint
  - `frontend/src/lib/components/ModeSwitchDialog.svelte` — calls the new ref-modes endpoint
  - `frontend/src/lib/components/TreeDag.svelte` (edit) — emit ref-action events
  - `frontend/src/lib/screens/SessionViewShell.svelte` (edit) — catch ref-action events, open the matching dialog
- Tests

## Acceptance Criteria

- [ ] POST .../ref-modes upserts ref mode and emits mode.changed event
- [ ] RefActionsMenu opens on right-click; shows Fork… and Switch Mode items
- [ ] ForkDialog submits → MCP fork tool called → new ref appears in TreeDag
- [ ] ModeSwitchDialog confirms + calls ref-modes endpoint; TreeDag refetches and shows new mode badge
- [ ] Tests green
