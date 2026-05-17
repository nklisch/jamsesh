---
id: epic-portal-ui-ref-actions
kind: feature
stage: done
tags: [ui]
parent: epic-portal-ui
depends_on: [epic-portal-ui-session-view-shell]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
---

# Portal UI — Ref Actions (Mode Switch & Fork)

## Brief

The interactive surface for per-ref actions in the tree pane. Two related
capabilities:

- **Mode switch** — click a ref's mode badge (sync / isolated) in the
  tree pane to flip it. Calls the portal API to persist the mode change.
  Visual transition (badge color + ref's tree position emphasis)
  reflects the new mode. Subscribes to `mode.changed` WebSocket events
  so peer-initiated mode changes appear live.
- **Fork action** — click a commit in the tree pane → "Fork from here"
  affordance opens a small dialog asking: replace your current ref, or
  create a new sibling ref under your namespace (`jam/<session>/<user>/<branch>`)?
  If sibling, prompts for branch name + mode (defaults to session default).
  Calls the portal MCP `fork` tool to do the server-side ref manipulation.
  Subscribes to `ref.forked` WS events so peer forks appear live in the tree.

These actions are coherent as one feature because they share the same
substrate (per-ref interaction in the tree pane), the same target
(modifying refs in the user's own namespace), and the same UI affordances
(action menu attached to ref groups + commits in the tree).

Does NOT cover: the tree pane rendering itself (in `session-view-shell`);
the actual server-side ref manipulation (in `epic-portal-api`).

## Epic context

- Parent epic: `epic-portal-ui`
- Position in epic: enriches the tree pane (provided by `session-view-shell`)
  with the two write-back interactions humans can take from the UI side
  of jamsesh. Most ref changes still happen via CC slash commands; these
  are the portal-side equivalents for "I'm already looking at the tree."

## Foundation references

- `docs/UX.md` — Flow: forking from a peer, Flow: switching mode
- `docs/PROTOCOL.md` — MCP `fork` tool (with `target_ref` and `mode`
  params), WebSocket events `mode.changed` and `ref.forked`
- `docs/ARCHITECTURE.md` — Dual mode (sync vs isolated semantics),
  Multi-agent per human (ref namespace)
- `.mockups/flows/onboarding/04-session-view.html` — locked tree-pane
  treatment showing mode badges in place

## Mockups

This feature's UI surfaces are interactive affordances embedded in the
tree pane — they don't warrant their own `/screens` pass. The treatments
they extend are visible in the locked session-view-shell option-5 mock:

- Mode badges in the tree pane:
  `.mockups/screens/epic-portal-ui-session-view-shell/option-5.html`
  - Collapsed (rail) mode: vertical-text mode label per ref
    (`a/main` / `b/main` / `c/expl` / `draft`) colored by mode
  - Expanded mode: horizontal `.mode-mini` chip in each ref's header
    (`sync` = accent-muted, `isolated` = warning-muted, `draft` = neutral)
- Author dot online indicator in the tree (used as presence signal —
  see `.online::after` rule in option-5)

**Action patterns this feature commits to:**

- **Mode switch** — clicking a mode badge in the tree (collapsed or
  expanded) opens a confirmation popover: "Switch alice/main to isolated?
  Future commits won't be auto-merged into draft. Already-merged commits
  stay in draft." Confirm → portal API call → optimistic UI update
  (badge color flips, ref drifts visually in tree to indicate detachment)
  + WebSocket `mode.changed` event confirms persistence.
- **Fork action** — clicking a commit dot in the tree opens a small
  contextual menu: "Fork from here". Selecting opens a dialog:
  - Choice: "Replace my current ref" or "Create a new sibling ref"
  - If sibling: branch name input (slug-validated), mode picker
    (defaults to session default mode)
  - Confirm → portal MCP `fork` tool call → optimistic tree update
    showing new/moved ref
- Both actions emit WS events that other clients receive — the originator
  sees optimistic confirmation immediately; peers see the change in
  near-real-time.

No standalone screens to mock — the affordances live entirely inside
already-locked surfaces (the tree pane, the action menu popover patterns
will reuse `Card` + `Button` from `epic-portal-ui-design-system`).

## Design decisions

- **Action surface**: right-click context menu on TreeDag ref nodes + an inline "actions" button when a ref is selected. Actions: Fork (from this commit), Switch Mode (sync ↔ isolated), View Ref Details.
- **Fork dialog**: Card-based modal with target_ref input + mode toggle. On submit: calls MCP `fork` tool via portal's `/mcp` endpoint (already shipped). On success: TreeDag refetches refs and the new ref appears.
- **Mode switch**: confirmation modal. v1 calls the new portal endpoint `POST /api/orgs/<org>/sessions/<sid>/ref-modes` with body `{ref, mode}` — ship the endpoint as part of this story (small addition to sessions-rest). Emits `mode.changed` event from the handler.
- **Single story** — `epic-portal-ui-ref-actions-menu-and-dialogs`.

## Implementation Units

### Unit 1: Backend ref-modes endpoint

`POST /api/orgs/<orgID>/sessions/<sessionID>/ref-modes` with body `{ref: string, mode: "sync"|"isolated"}`. Validates membership + ref exists; UpsertRefMode; emits mode.changed event with ModeChangedPayload. Add to `internal/portal/sessions/`. openapi + regen.

### Unit 2: RefActionsMenu.svelte

`frontend/src/lib/components/RefActionsMenu.svelte` — right-click + button-triggered menu. Items: Fork…, Switch to sync/isolated, Copy ref name.

### Unit 3: ForkDialog.svelte

`frontend/src/lib/components/ForkDialog.svelte` — modal with target_ref input + mode toggle. POSTs MCP fork tool.

### Unit 4: ModeSwitchDialog.svelte

`frontend/src/lib/components/ModeSwitchDialog.svelte` — confirmation modal. POSTs ref-modes endpoint.

### Unit 5: Wire into TreeDag

TreeDag fires `onref-action(ref, action)` event. SessionViewShell catches and opens the matching dialog.

## Single story

`epic-portal-ui-ref-actions-menu-and-dialogs`

## Implementation summary

Single child story done.

## Review

**Verdict**: Approve. Capability complete.
