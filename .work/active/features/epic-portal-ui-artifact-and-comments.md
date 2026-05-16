---
id: epic-portal-ui-artifact-and-comments
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

# Portal UI — Artifact Pane & Inline Comments

## Brief

The deep-work surface inside the session view shell. Three coupled pieces
that need to ship together:

- **Artifact pane** — read-only file viewer at the selected commit
  (selected via the tree pane). Renders file content with line numbers
  and line-range selection. Handles text files with basic syntax
  highlighting (markdown, code formats common in doc work). No editing —
  changes are commits, not in-UI edits.
- **Inline comment display** — comments are anchored to a (commit, file,
  line-range) per `PROTOCOL.md`. Display style is **inline** (locked in
  epic-design Phase 4.7): comments appear directly below the line they're
  anchored to, expanding the file view. Shows author, addressing (e.g.,
  `@alice/main`), kind badge (question / suggestion / action-request / fyi),
  and a reply count. Click to expand the thread.
- **Comment composer overlay** — opens when the user clicks "Comment on
  line" or selects a line range and triggers the comment shortcut. Full
  addressing UX with autocomplete (`@user`, `@user/branch`,
  `@all-agents`, `@everyone`, `@auto-merger`), kind selector. Posts via
  MCP `post_comment` (the portal proxies MCP → REST internally).

WebSocket subscription: `comment.added`, `comment.resolved` — so newly
posted comments from peers appear live.

Does NOT cover: writing files (read-only by design); the tree pane
(in `session-view-shell`); the conflict UI (no dedicated UI — conflicts
surface in the activity feed and via comments).

## Epic context

- Parent epic: `epic-portal-ui`
- Position in epic: lives inside the artifact-pane slot provided by
  `session-view-shell`. The deepest, most-used surface for humans
  reading peer work and dropping comments.

## Foundation references

- `docs/UX.md` — Flow: posting a comment, Portal UI surfaces > Artifact
  pane and comment composer
- `docs/PROTOCOL.md` — MCP `post_comment` / `resolve_comment`, Comment
  schema (with addressing metadata), Anchor (commit, file, line range)
- `docs/ARCHITECTURE.md` — Comments data layer
- `.mockups/flows/onboarding/04-session-view.html` — locked design
  showing inline comment treatment

<!-- Feature-design will fill in the file-viewer rendering strategy,
line-range selection mechanic, composer overlay interaction, and addressing
autocomplete data shape when /agile-workflow:feature-design runs on this. -->
