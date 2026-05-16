---
id: epic-portal-ui-session-view-shell
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

# Portal UI — Session View Shell

## Brief

The main work surface — the screen humans live in while their agents are
running. Per the onboarding flow's step 4 (and UX.md), the session view is
a three-column layout:

- **Session header** at the top: goal title, meta strip (scope chips,
  default mode, member count, commit count)
- **Tree pane** (left): the git DAG colored by author with mode badges
  per ref, draft as the merged trunk, click-to-select-commit interaction
- **Artifact pane** (center): renders the file at the selected commit —
  this feature ships the layout slot only; the actual artifact rendering
  + comments lives in `epic-portal-ui-artifact-and-comments`
- **Activity feed + Presence panel** (right): chronological events
  (commits, comments, conflicts, mode changes) and online-member panel
  with current commit per member

All four right-and-center surfaces subscribe to the portal WebSocket and
re-render reactively on `commit.arrived`, `merge.succeeded`,
`conflict.detected`, `comment.added`, `presence.updated`, `mode.changed`,
`turn.ended`.

The tree pane is the highest-effort surface — SVG-based DAG layout with
author-color edges, ref-grouping labels, and live update animations.
Feature-design will decide whether to extract it as a separate feature.

Does NOT cover: the artifact viewer or inline comments
(`epic-portal-ui-artifact-and-comments`); mode/fork actions
(`epic-portal-ui-ref-actions`).

## Epic context

- Parent epic: `epic-portal-ui`
- Position in epic: container for the central jamsesh experience. The
  artifact pane and per-ref actions slot into this shell.

## Foundation references

- `docs/UX.md` — Portal UI surfaces > Session view, Flow: an agent turn
- `docs/PROTOCOL.md` — WebSocket event types, Conflict event schema
- `docs/ARCHITECTURE.md` — Multi-agent per human (tree pane renders all
  `jam/<session>/<user>/<branch>` refs grouped by user)
- `docs/SPEC.md` — Ref structure
- `.mockups/flows/onboarding/04-session-view.html` — locked design

## Decomposition risks (carried from epic pre-mortem)

- The git DAG renderer in the tree pane is the highest-risk piece in this
  epic. SVG-based, multi-branch, author-colored, live-updating. Feature-
  design may split it into its own feature if the implementation surface
  warrants — note the split if so.
- The WebSocket subscription pattern from `epic-portal-ui-foundation`
  must scale across four independent subscribing surfaces in this shell.
  Verify the API shape supports per-event-type filtering without
  duplicating connections.

<!-- Feature-design will fill in layout slots, subscription wiring, DAG
renderer interface, and decompose further if needed when
/agile-workflow:feature-design runs on this. -->
