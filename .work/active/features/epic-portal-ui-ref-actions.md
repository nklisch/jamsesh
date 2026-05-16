---
id: epic-portal-ui-ref-actions
kind: feature
stage: drafting
tags: [ui]
parent: epic-portal-ui
depends_on: [epic-portal-ui-session-view-shell]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
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

<!-- Feature-design will fill in the dialog flow for fork target naming,
the mode-switch confirmation pattern, and the MCP call wiring when
/agile-workflow:feature-design runs on this. -->
