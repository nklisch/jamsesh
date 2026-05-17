---
id: epic-portal-ui-session-view-shell-shell-and-tree
kind: story
stage: implementing
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
