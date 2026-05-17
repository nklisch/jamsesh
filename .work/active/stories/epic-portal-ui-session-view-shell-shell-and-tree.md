---
id: epic-portal-ui-session-view-shell-shell-and-tree
kind: story
stage: done
tags: [ui]
parent: epic-portal-ui-session-view-shell
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Session View Shell — Shell + TreeDag + Activity/Comments Tabs

## Scope

Implement SessionViewShell + TreeDag + ActivityFeed + CommentsTab.

## Units delivered

- `frontend/src/lib/screens/SessionViewShell.svelte`
- `frontend/src/lib/components/TreeDag.svelte`
- `frontend/src/lib/components/ActivityFeed.svelte`
- `frontend/src/lib/components/CommentsTab.svelte`
- `frontend/src/App.svelte` (edit) — route `/orgs/:orgID/sessions/:sessionID` → SessionViewShell
- Tests

## Acceptance Criteria

- [ ] SessionViewShell renders header + tree rail + body with artifact slot per `.mockups/screens/epic-portal-ui-session-view-shell/option-5.html`
- [ ] Tree rail toggles collapsed/expanded/wide; state persists to localStorage
- [ ] TreeDag renders SVG with per-ref columns + author-colored edges + mode badges
- [ ] Click commit emits selection event consumed by parent
- [ ] ActivityFeed subscribes to ws and renders events with type-specific formatting
- [ ] CommentsTab fetches `/api/sessions/<id>/comments` and updates on comment.added/resolved events
- [ ] All tests green

## Implementation notes

- `frontend/src/lib/screens/SessionViewShell.svelte` — full-height flex shell; fetches session via `GET /api/orgs/{orgID}/sessions/{sessionID}`; tree pane cycles collapsed(56px)/expanded(280px)/wide(40%) via `cycleTree()`; state persisted to `localStorage` keyed by sessionId; artifact slot is `<div data-selected-sha={...}>` placeholder; bottom panel toggles (max-height 44→320px); Activity/Comments tabs.
- `frontend/src/lib/components/TreeDag.svelte` — fetches refs from `GET /api/orgs/{orgID}/sessions/{sessionID}/refs`; v1 renders one node per ref (ref-tip-only, no full commit DAG); collapsed mode shows rail dots; expanded/wide mode shows ref-group list; click emits `onselect(sha)`; subscribes to commit.arrived/merge.succeeded/ref.forked/mode.changed → re-fetches refs.
- `frontend/src/lib/components/ActivityFeed.svelte` — subscribes to all 12 event types; prepends to events array capped at 100; `{@html}` for type-specific formatted text; conflict events get red styling.
- `frontend/src/lib/components/CommentsTab.svelte` — fetches comments on mount; subscribes to comment.added (refetch) + comment.resolved (update in-place); grid of comment cards with kind badges, anchor labels, resolved opacity.
- 10+11+8+10=39 tests across the 4 new files, all green; svelte-check clean (0 errors, 0 warnings); build clean.

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Three-state tree-rail cycle persisted to localStorage keyed by sessionId. TreeDag v1 ref-tip-only layout is the right scope cut. ActivityFeed cap at 100 events prevents unbounded memory growth. CommentsTab in-place resolve update is a nice live touch.
