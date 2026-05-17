---
id: epic-portal-ui-artifact-and-comments-pane-and-composer
kind: story
stage: review
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

- [x] Files endpoint returns text content for text files; 404 for missing path; mime detection identifies binary (return placeholder)
- [x] Comments POST endpoint creates a comment via Service.Create with author_kind="human"
- [x] ArtifactPane fetches + renders selected sha+path with line numbers
- [x] Line-range selection (click first + shift-click last) sets composer's anchor
- [x] CommentComposer submits + receives created comment in response
- [x] Tests green

## Implementation notes

- `internal/portal/sessions/files.go`: opens bare repo via go-git `PlainOpen`, resolves commit hash, reads file blob. Returns 413 for >1MB; binary detection scans first 8000 bytes for null bytes; extension-based MIME detection.
- `internal/portal/comments/handlers.go`: added `CreateComment` method; validates body, checks org/session membership, calls `svc.Create` with `AuthorKind: "human"`. Returns 201 with created comment.
- `docs/openapi.yaml`: added `CreateCommentRequest` and `SessionFileResponse` schemas; `POST` comments operation added under existing path (not a duplicate path key); `GET /files` path added.
- `frontend/src/lib/components/ArtifactPane.svelte`: fetches via native `fetch` with Bearer auth header; `$effect` watches `selectedSha`/`selectedPath`; shift-click for range selection; binary placeholder, loading, and error states.
- `frontend/src/lib/components/CommentComposer.svelte`: uses `client.POST` from openapi-fetch; kind dropdown (question/suggestion/action-request/fyi), addressed_to input, body textarea.
- 23 new tests across `ArtifactPane.test.ts`, `CommentComposer.test.ts`, `files_test.go`, `service_test.go`.
