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
updated: 2026-05-17
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

## Mockups

- Screens: `.mockups/screens/epic-portal-ui-session-view-shell/index.html`
- Selected: **option-5 (hybrid)** — 2026-05-16
- Rationale: focus-mode artifact dominance gives reading room; the tree
  rail expands on demand (collapsed → expanded → wide cycle); the bottom
  panel carries Activity and Comments as tabs (not a third column); presence
  absorbs into the tree as small online indicators on author dots. No
  separate presence panel — the tree IS the presence surface.

**Layout primitives this commits to:**

- Three layout states for the tree pane: `tree-collapsed` (56px rail, dots
  only), `tree-expanded` (~280px, full ref groups with mode badges and
  commit titles), `tree-wide` (~40% width, same content as expanded). The
  cycle button (⇔) is the primary toggle in v1; resizable drag-divider
  is a v2 follow-up.
- Bottom panel is sticky 44px collapsed (showing the latest activity item
  with a live-dot pulse) or expandable to ~280px with the active tab body.
- Bottom tabs: **Activity** (chronological event feed) and **Comments**
  (grid of comment cards across the session, each clickable to navigate the
  artifact to its anchor).
- Presence: every ref's author dot in the tree has an `.online` modifier
  that adds a small green indicator. No presence panel as a separate
  surface.

**Implementation implications (recorded for feature-design later):**

- The tree-state cycle (collapsed/expanded/wide) is local UI state — a
  per-session persisted preference (last cycle value remembered when the
  user returns to this session).
- The comments tab is essentially a flat list of all comments in the
  session, surfaceable independent of the artifact pane. Could be powered
  by `query_session_state({ include: ['unresolved_comments'] })` per
  PROTOCOL.md.
- Bottom-panel expansion-on-tab-click is the right default; collapsing
  the panel hides everything but the latest-event strip.

## Design decisions

- **Mockup**: `.mockups/screens/epic-portal-ui-session-view-shell/option-5.html` (hybrid).
- **TreeDag**: SVG-based, scoped to `TreeDag.svelte`. Per-ref columns, author-colored edges via `--author-N` tokens, mode badges, draft trunk highlighted. Click commit emits selection.
- **Bottom panel tabs**: Activity (WS event stream) + Comments (REST + WS updates).
- **Artifact slot**: empty `<div data-selected-sha={...}>` for the artifact-and-comments sibling feature to fill.
- **Tree state**: collapsed/expanded/wide cycle persisted to localStorage per session.
- **Single story** — `epic-portal-ui-session-view-shell-shell-and-tree`.

## Implementation Units

### Unit 1: SessionViewShell + zones

`frontend/src/lib/screens/SessionViewShell.svelte` — three-zone layout per the option-5 mock. Header (session meta), tree rail (TreeDag), body (artifact slot + bottom panel).

### Unit 2: TreeDag

`frontend/src/lib/components/TreeDag.svelte` — SVG layout, column-per-ref, click → selection event.

### Unit 3: ActivityFeed + CommentsTab

`frontend/src/lib/components/ActivityFeed.svelte` and `CommentsTab.svelte` — bottom panel tabs.

### Unit 4: Routing

`App.svelte` route `/orgs/<orgID>/sessions/<sessionID>` → SessionViewShell.
