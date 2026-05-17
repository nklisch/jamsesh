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
- `.mockups/flows/onboarding/04-session-view.html` — locked journey
  showing where this surface lives inside the session view shell

## Mockups

- Screens: `.mockups/screens/epic-portal-ui-artifact-and-comments/index.html`
- Selected: **option-4 (GitHub-PR style)** — 2026-05-16
- Rationale: sharpens the epic-design Phase 4.7 lock ("inline anchored to
  the line") with the specific expand/collapse mechanic. Collapsed-by-
  default strip below the line shows kind + author + preview + recency;
  click to expand inline into the full thread. Keeps the file view scannable
  while making the comment count visible at every anchor. Composer is also
  inline. Familiar to anyone who's reviewed a GitHub PR.

**Layout primitives this commits to:**

- **Comment-strip (collapsed default)** — a thin row below the commented
  line: `[kind badge] @author preview-text recency [expand ↓]`. Strip
  inherits the kind's color (question = accent, suggestion = warning,
  etc.). Click toggles to expanded form.
- **Comment-expanded (inline)** — full card directly below the line with
  the same data as the collapsed strip, plus reply count, "reply" link,
  "mark resolved" action. Click the head to collapse back.
- **Line affordance** — commented lines get an `inset 3px 0 0 <kind-color>`
  left border to mark their state without color-flooding the line.
- **Composer (inline, mid-state shown in mock)** — opens below the
  selected line with: anchor indicator ("@ line 20 · 1 line selected"),
  kind control (defaults to fyi; click to swap), addressing input with
  autocomplete suggesting human/agent identities + broadcast targets,
  body field, post/cancel actions, keyboard hint (`⌘↵` to post, `esc`
  to cancel).
- **Composer entry mechanisms** (all universal):
  - Hover any line → "+ comment" button appears with `c` keyboard shortcut
  - Select a line range → "Comment on selection" pill anchored above
  - "Comment on line" button in artifact-head (selects current scroll
    position by default)
  - Keyboard: `c` on the focused line
- **Expand-all** — head action lets the human flip all collapsed strips
  to expanded in one click ("expand all comments (N)"); useful when
  reviewing for sign-off.

**Implementation implications (recorded for feature-design later):**

- Collapsed strip + expanded form share data; toggling is local UI state.
- Reading a long file with many comments doesn't force the human to
  scroll past full comment cards — they see strips and decide what to
  expand. The opt-in expansion is the difference from option-1 (always
  expanded).
- Comment threading: replies are inline under the parent comment when
  expanded. The "2 replies" link in the expanded form is the affordance
  to see the rest.
- Addressing autocomplete must include the literal `@` recipients from
  PROTOCOL.md: `@<user>`, `@<user>/<branch>`, `@all-humans`,
  `@all-agents`, `@everyone`, `@auto-merger`. The data source is
  `query_session_state({ include: ['members', 'refs'] })`.

<!-- Feature-design will fill in the file-viewer rendering strategy,
line-range selection mechanic, composer overlay interaction, and addressing
autocomplete data shape when /agile-workflow:feature-design runs on this.
Feature stays at stage: drafting per --mocks-only pass. -->
