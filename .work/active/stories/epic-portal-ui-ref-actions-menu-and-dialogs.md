---
id: epic-portal-ui-ref-actions-menu-and-dialogs
kind: story
stage: review
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

- [x] POST .../ref-modes upserts ref mode and emits mode.changed event
- [x] RefActionsMenu opens on right-click; shows Fork… and Switch Mode items
- [x] ForkDialog submits → MCP fork tool called → new ref appears in TreeDag
- [x] ModeSwitchDialog confirms + calls ref-modes endpoint; TreeDag refetches and shows new mode badge
- [x] Tests green

## Implementation notes

- `internal/portal/sessions/refmodes.go`: auth check, org/session membership, `store.UpsertRefMode`, `events.Log.Emit("mode.changed")` best-effort. Returns 204.
- `docs/openapi.yaml`: added `RefMode` as a named schema (referenced by `Ref.mode` via `$ref`); added `UpsertRefModeRequest` schema; new `POST /ref-modes` path.
- `frontend/src/lib/components/RefActionsMenu.svelte`: fixed-position backdrop + menu; "Fork…" and "Switch mode…" items; calls `onaction` then `onclose`.
- `frontend/src/lib/components/ForkDialog.svelte`: calls MCP JSON-RPC via `fetch('/mcp', ...)` with `tools/call fork`; fetches tip SHA from refs list first.
- `frontend/src/lib/components/ModeSwitchDialog.svelte`: radio buttons sync/isolated; submit disabled when `selectedMode === currentMode`; uses `client.POST` from openapi-fetch.
- `frontend/src/lib/components/TreeDag.svelte`: added `onrefaction` prop; `oncontextmenu` on both rail-ref and ref-group elements.
- 32 new tests across `RefActionsMenu.test.ts`, `ModeSwitchDialog.test.ts`, `refmodes_test.go`.
