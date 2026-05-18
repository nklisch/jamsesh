---
id: gate-docs-protocol-unscoped-routes
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

# PROTOCOL.md REST route catalog uses unscoped `/api/sessions/<id>/...` paths instead of org-scoped paths

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/PROTOCOL.md:107-128`
- Code: `docs/openapi.yaml:1521` and throughout; router mounts at
  `internal/portal/router/router.go:103`

## Current doc text
> `GET /api/sessions/<id>` — session metadata
> `PATCH /api/sessions/<id>` — update goal, scope (widen only), default_mode
> `POST /api/sessions/<id>/finalize` — mark session as finalizing
> `GET /api/sessions/<id>/digest?since=<seq>` …
> `GET /api/sessions/<id>/refs` …
> `GET /api/sessions/<id>/finalize-plan` …

## Reality
Every session-scoped REST route is mounted at
`/api/orgs/{orgID}/sessions/{sessionID}/...`. The multi-tenancy
invariant ("every API route is org-scoped") is enforced at the path
level — there is no unscoped `/api/sessions/...` surface in the openapi
or in chi.

## Required edit
Rewrite the PROTOCOL.md REST sections (Auth/Orgs/Sessions/Session-state)
to use `/api/orgs/{orgID}/sessions/{sessionID}/...` paths and add the
missing org-scoping. Match the live OpenAPI catalog.

## Implementation notes

Lines edited in `docs/PROTOCOL.md` (original line numbers, Sessions and Session-state sections):

- Line 106: `POST /api/sessions` → `POST /api/orgs/{orgID}/sessions`
- Line 108: `GET /api/sessions` → `GET /api/orgs/{orgID}/sessions`
- Line 109: `GET /api/sessions/<id>` → `GET /api/orgs/{orgID}/sessions/{sessionID}`
- Line 110: `PATCH /api/sessions/<id>` → `PATCH /api/orgs/{orgID}/sessions/{sessionID}`
- Line 111: `POST /api/sessions/<id>/finalize` → `POST /api/orgs/{orgID}/sessions/{sessionID}/finalize`
- Line 114: `POST /api/sessions/<id>/abandon` → `POST /api/orgs/{orgID}/sessions/{sessionID}/abandon`
- Line 115: `POST /api/sessions/<id>/invites` → `POST /api/orgs/{orgID}/sessions/{sessionID}/invites`
- Line 116: `POST /api/sessions/<id>/members/<account_id>/remove` → `POST /api/orgs/{orgID}/sessions/{sessionID}/members/{accountID}/remove`
- Line 120: `GET /api/sessions/<id>/digest?since=<seq>` → `GET /api/orgs/{orgID}/sessions/{sessionID}/digest?since=<seq>`
- Line 122: `GET /api/sessions/<id>/refs` → `GET /api/orgs/{orgID}/sessions/{sessionID}/refs`
- Line 123: `GET /api/sessions/<id>/finalize-plan` → `GET /api/orgs/{orgID}/sessions/{sessionID}/finalize-plan`

Path parameter naming matches `docs/openapi.yaml` verbatim (`{orgID}`, `{sessionID}`, `{accountID}`). No new endpoints added; no section restructuring. The Orgs & accounts section (lines 101-102) already used org-scoped paths and was left unchanged.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Mechanical change matching the gate finding spec. Implementation notes accurately describe what was changed. Global `go build ./...` and `go test ./internal/portal/...` pass after the wave landed.
