---
id: epic-portal-ui
kind: epic
stage: drafting
tags: [ui]
parent: null
depends_on: [epic-portal-api]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal UI

## Brief

The web frontend humans glance at while their agents work. Optimized for
"glance" over "work" — most actual work happens in CC; the UI is awareness
and lightweight social acts.

Implementation tech: **Svelte 5 + Vite**. SPA only (no SvelteKit, no SSR
— this is an authenticated app with no SEO needs). Runes carry the
WS-driven reactive surface. Static build output is embedded into the
portal Go binary. Per UX.md, the primary surfaces are:

- **Session list** — grouped by status (active, finalizing, ended).
- **Session view** (the main work surface):
  - Tree pane (the git DAG colored by author, mode badges on refs)
  - Artifact pane (file viewer at a selected commit with inline comments
    and a comment composer)
  - Activity feed (chronological events: commits, comments, conflicts)
  - Presence panel (who's online, their refs, their current commits)
  - Session header (goal, scope, default mode, member count)
- **Comment composer** (overlay) with full addressing UX (`@user`,
  `@user/branch`, `@all-agents`, `@everyone`, `@auto-merger`) and `kind`
  selection.
- **Mode badges** on refs with switch action (sync ↔ isolated).
- **Fork action** UI ("fork from here" on a commit).
- **WebSocket subscription** for live updates of all of the above.

Artifact pane is read-only. There is no real-time text editing, no live
cursors — those are intentional non-features per UX.md.

This epic does NOT cover portal-side API (`epic-portal-api` provides it);
it does NOT cover the finalize curation view (`epic-finalize-flow`).

## Foundation references

- `docs/UX.md` — Interaction model, Portal UI surfaces, all flows
- `docs/ARCHITECTURE.md` — WebSocket gateway (subscription model)
- `docs/PROTOCOL.md` — WebSocket event types, Comment schema, Conflict
  event schema

## Anticipated child features

Provisional — actual decomposition lands when this epic is designed.

- Frontend skeleton (Svelte 5 + Vite project scaffolding, routing
  choice, build wiring, static-asset embedding into the Go binary)
- Design system tokens (run `/ux-ui-design:palette` first; lock typography
  + palette)
- Session list view
- Session view shell (layout, navigation between panes)
- Tree pane (git DAG rendering colored by author, mode badges)
- Artifact pane (file viewer with line-range selection for comments)
- Comment display (inline anchors, threaded ordering by anchor)
- Comment composer (overlay, addressing UX, kind selection)
- Activity feed
- Presence panel
- Mode badges + switch action
- Fork action
- WebSocket subscription + event handling

<!-- Design pass on each child feature will fill in specifics. -->
