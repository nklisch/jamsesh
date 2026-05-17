---
id: epic-portal-ui-artifact-and-comments-pane-and-composer
kind: story
stage: implementing
tags: [ui]
parent: epic-portal-ui-artifact-and-comments
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Artifact + Comments — Pane + Composer + 2 Backend Endpoints

## Scope

Add 2 new REST endpoints + the ArtifactPane + CommentComposer Svelte components; wire into SessionViewShell's artifact slot.

## Units delivered

- Backend:
  - `internal/portal/sessions/files.go` — `GET .../files?commit=<sha>&path=<filepath>` returning `{content: string, mime: string}`
  - `internal/portal/comments/handlers.go` (edit) — `POST .../sessions/<sid>/comments` calling `comments.Service.Create`
  - openapi.yaml additions + regen
- Frontend:
  - `frontend/src/lib/components/ArtifactPane.svelte` — fetches file content, renders with line numbers, line-range selection
  - `frontend/src/lib/components/CommentComposer.svelte` — overlay form, POSTs comment
  - `frontend/src/lib/screens/SessionViewShell.svelte` (edit) — render ArtifactPane in artifact slot; manage composer open/close + selection state
- Tests

## Acceptance Criteria

- [ ] Files endpoint returns text content for text files; 404 for missing path; mime detection identifies binary (return placeholder)
- [ ] Comments POST endpoint creates a comment via Service.Create with author_kind="human"
- [ ] ArtifactPane fetches + renders selected sha+path with line numbers
- [ ] Line-range selection (click first + shift-click last) sets composer's anchor
- [ ] CommentComposer submits + receives created comment in response
- [ ] Tests green
