---
id: gate-docs-protocol-missing-endpoints
kind: story
stage: implementing
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
