---
id: gate-docs-protocol-unscoped-routes
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
