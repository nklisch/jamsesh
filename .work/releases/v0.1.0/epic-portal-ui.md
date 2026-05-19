---
id: epic-portal-ui
kind: epic
stage: done
tags: [ui]
parent: null
depends_on: [epic-portal-api]
release_binding: v0.1.0
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

## Mockups

**Design system** (the source of truth all UI features inherit):

- `.mockups/design-system/tokens.css` — locked 2026-05-16
- Palette: Quiet Slate (cool monochromatic + muted teal accent, Linear-adjacent)
- Typography: Geist + Geist Mono (modern, neutral, geometric)
- Toggle: `prefers-color-scheme` by default, `[data-theme="dark"|"light"]`
  on `<html>` for explicit override
- Preview pages (trimmed to chosen direction):
  - `.mockups/design-system/palette.html`
  - `.mockups/design-system/typography.html`
- Author colors: 8 distinct earth-tone hues defined as `--author-1`..`--author-8`,
  each with light and dark variants for the git DAG renderer

**Onboarding flow** (the multi-step journey new users take):

- `.mockups/flows/onboarding/index.html` (4 steps: invite-landing → sign-in
  → session-list → session-view) — polished, desktop-first, signed off
  2026-05-16

**Per-feature screen mocks** (one chosen direction per UI feature with
a screen exploration):

- Session view shell — option-5 hybrid (cyclable tree, tabbed bottom,
  presence-in-tree):
  `.mockups/screens/epic-portal-ui-session-view-shell/option-5.html`
- Session list — option-1 row cards (large):
  `.mockups/screens/epic-portal-ui-session-list/option-1.html`
- Artifact & comments — option-4 GitHub-PR style (collapsed strip,
  click-to-expand):
  `.mockups/screens/epic-portal-ui-artifact-and-comments/option-4.html`

**Features without their own screen pass** (UI lives inside other
surfaces or has no novel screen): `epic-portal-ui-foundation` (login is
in the onboarding flow), `epic-portal-ui-design-system` (the design
system IS its source), `epic-portal-ui-ref-actions` (affordances live in
the session-view tree pane). Each feature body's `## Mockups` section
points at where its visual reference lives.

## Design decisions

Locked during the epic-design pass:

- **Theme:** both light and dark, with toggle. Default behavior follows
  `prefers-color-scheme`; explicit `[data-theme="dark"|"light"]` on
  `<html>` overrides the system preference and is persisted client-side.
- **Auth UX shape:** OAuth and magic-link are equally prominent on the
  sign-in card. Neither is the fallback; both are first-class entry
  points.
- **Comment display style:** inline anchored to the line (GitHub PR style).
  Comments expand the file view directly below their commented line.

## Decomposition

The decomposition splits by capability, not by layer. `foundation` and
`design-system` are independent infrastructure features that run in
parallel from day one. `session-list` and `session-view-shell` are the
two user-facing surfaces, both depending on foundation + design-system.
`artifact-and-comments` and `ref-actions` slot into the session-view-shell
container as the two interaction surfaces inside it.

The mockup phase delivered the locked design system at
`.mockups/design-system/` (palette + typography + tokens.css with both
color modes) and the locked onboarding journey at
`.mockups/flows/onboarding/` (login → session list → session view, polished
desktop-first). Per-feature screen alternatives via `/ux-ui-design:screens`
fire at feature-design time for surfaces that warrant exploration.

### Child features

- `epic-portal-ui-foundation` — Svelte 5 + Vite scaffold, routing,
  embedded-static-assets, OAuth + magic-link login UI, WebSocket client
  wrapper, reactive state primitives — depends on: `[]`
- `epic-portal-ui-design-system` — tokens.css import pipeline, theme
  toggle, base components (Button, Input, Card, Badge, Pill, AuthorDot,
  InlineCode) — depends on: `[]`
- `epic-portal-ui-session-list` — session list view with filter chips and
  status grouping; WebSocket subscription for live session updates —
  depends on: `[epic-portal-ui-foundation, epic-portal-ui-design-system]`
- `epic-portal-ui-session-view-shell` — three-column session view layout,
  session header, tree pane (git DAG renderer), activity feed, presence
  panel; central artifact-pane slot for downstream feature —
  depends on: `[epic-portal-ui-foundation, epic-portal-ui-design-system]`
- `epic-portal-ui-artifact-and-comments` — read-only artifact viewer,
  line-range selection, inline-anchored comment display, comment composer
  overlay with addressing autocomplete and kind selector —
  depends on: `[epic-portal-ui-session-view-shell]`
- `epic-portal-ui-ref-actions` — per-ref mode switch (sync ↔ isolated)
  and fork action (replace ref or create sibling under user's namespace) —
  depends on: `[epic-portal-ui-session-view-shell]`

### Decomposition risks

- **Git DAG renderer is the highest-risk piece in the epic.** SVG-based,
  multi-branch, author-colored, live-updating via WebSocket. Lives inside
  `epic-portal-ui-session-view-shell` initially; feature-design may split
  it into a sibling feature if the implementation surface warrants.
- **WebSocket subscription pattern is cross-cutting.** The shared
  abstraction lives in `epic-portal-ui-foundation`. Every other feature
  that displays live data uses it. Feature-design must enforce a single
  consistent subscription API to prevent per-feature drift.
- **Routing-library choice** (svelte-spa-router / hand-rolled / hash-based)
  is locked at `foundation`-design time and affects every consuming feature.
- **"New session" button's boundary is unresolved** between epic-portal-ui
  (button + dialog?), epic-portal-api (create endpoint), and epic-cc-plugin
  (the `/jamsesh:create` slash command). Feature-design on `session-list`
  picks an answer and documents the boundary with the other two epics.

<!-- Design pass on each child feature will fill in interfaces,
component APIs, signatures, and test approach. -->


## Children complete (2026-05-17)

All 6 child features are at `stage: done`:
- epic-portal-ui-foundation (Vite + Svelte 5 skeleton, openapi-fetch client, WS client + auth)
- epic-portal-ui-design-system (palette, typography, 7 base components)
- epic-portal-ui-session-list (SessionList screen + NewSessionDrawer + Chrome)
- epic-portal-ui-session-view-shell (3-pane shell + TreeDag + ActivityFeed + breadcrumb)
- epic-portal-ui-artifact-and-comments (ArtifactPane + CommentComposer + 2 backend endpoints)
- epic-portal-ui-ref-actions (RefActionsMenu + ForkDialog + ModeSwitchDialog + ref-modes endpoint)

Final state: 226 frontend tests pass; all Go packages green; svelte-check clean; build clean.

## Review (2026-05-17)

**Verdict**: Approve

Epic delivered as briefed. Portal UI surface complete: sessions list with live WS updates, three-pane session view (tree + artifact + activity), inline comment composer, ref actions (fork + mode switch). All capability demos end-to-end against the portal API stack. Capability completeness: a user can land in the portal, see their sessions, click into one, browse refs in the tree, view files at any commit, post comments, fork from a peer's tip, and switch a ref's mode — every interaction round-trips through the API and emits the corresponding WS events.

No cross-cutting concerns surfaced across the 6 children. Advancing to done.
