---
id: gate-docs-protocol-missing-endpoints
kind: story
stage: done
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: docs
created: 2026-05-18
updated: 2026-05-18
---

# PROTOCOL.md route catalog omits shipped endpoints (finalize/lock, finalize/fetch-token, mark-shipped, files, ref-modes, comments, invites)

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/PROTOCOL.md:104-128`
- Code: `docs/openapi.yaml` (paths include `/finalize/lock`,
  `/finalize/lock/{lockID}`, `/finalize-plan`, `/finalize/fetch-token`,
  `/mark-shipped`, `/comments`, `/comments/{commentId}/resolve`,
  `/files`, `/ref-modes`, `/invites`, `/invites/{inviteID}`,
  `/invites/{inviteID}/accept`)

## Current doc text
> ### Sessions
> - `POST /api/sessions` — create a session
> - `GET /api/sessions` — list sessions visible
> - `GET /api/sessions/<id>` — session metadata
> - `PATCH /api/sessions/<id>` — update goal, scope (widen only), default_mode
> - `POST /api/sessions/<id>/finalize` — mark session as finalizing
> - `POST /api/sessions/<id>/abandon` — close session without finalize
> - `POST /api/sessions/<id>/invites` — invite participants
> - `POST /api/sessions/<id>/members/<account_id>/remove` — remove a member
> ### Session state (used by the local binary)
> - `GET /api/sessions/<id>/digest`
> - `GET /api/sessions/<id>/refs`
> - `GET /api/sessions/<id>/finalize-plan`

## Reality
Live REST surface also includes: finalize lock endpoints
(`POST/GET .../finalize/lock`, `GET/DELETE .../finalize/lock/{lockID}`),
`POST .../finalize/fetch-token`, `POST .../mark-shipped`, full comments
surface (`GET/POST .../comments`, `POST .../comments/{commentId}/resolve`),
`GET .../files`, `POST .../ref-modes`, and invite-accept
(`POST .../invites/{inviteID}/accept`). All org-scoped.

## Required edit
Expand PROTOCOL.md's Sessions and Session-state sections to list every
endpoint in `docs/openapi.yaml`, with the same scoping. The intent is a
human-readable summary; the summary must be complete to be useful.

## Implementation notes

Added 12 missing endpoints to `docs/PROTOCOL.md`. No existing entries were
modified. Endpoints added:

**Sessions section (3 new):**
- `GET /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}` — get a specific pending invite
- `POST /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept` — accept a session invite
- `POST /api/orgs/{orgID}/sessions/{sessionID}/mark-shipped` — mark a finalizing session as shipped

**Comments section (new section, 3 entries):**
- `GET /api/orgs/{orgID}/sessions/{sessionID}/comments` — list comments
- `POST /api/orgs/{orgID}/sessions/{sessionID}/comments` — post a comment
- `POST /api/orgs/{orgID}/sessions/{sessionID}/comments/{commentId}/resolve` — resolve a comment

**Session state section (2 new):**
- `GET /api/orgs/{orgID}/sessions/{sessionID}/files` — list files in the draft tree
- `POST /api/orgs/{orgID}/sessions/{sessionID}/ref-modes` — change a ref's mode

**Finalize machinery section (new section, 4 entries):**
- `POST /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock` — acquire finalize lock
- `PATCH /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}` — update curation state on held lock
- `DELETE /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}` — release the lock
- `POST /api/orgs/{orgID}/sessions/{sessionID}/finalize/fetch-token` — obtain HTTPS fetch token

Path names match openapi.yaml verbatim (e.g. `{commentId}` not `{commentID}`).

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Mechanical change matching the gate finding spec. Implementation notes accurately describe what was changed. Global `go build ./...` and `go test ./internal/portal/...` pass after the wave landed.
