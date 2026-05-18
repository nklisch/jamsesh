---
id: gate-docs-arch-unscoped-routes
kind: story
stage: review
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: docs
created: 2026-05-18
updated: 2026-05-18
---

# ARCHITECTURE.md uses unscoped `/api/sessions/<id>/digest` while the rest of the system is org-scoped

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/ARCHITECTURE.md:97,181,338`
- Code: `docs/openapi.yaml` —
  `/api/orgs/{orgID}/sessions/{sessionID}/digest`; finalize plan is
  `/api/orgs/{orgID}/sessions/{sessionID}/finalize-plan`

## Current doc text
> Calls `GET /api/sessions/<id>/digest?since=<seq>` on the portal …
> the plan body via `GET /finalize-plan` and runs it locally

## Reality
Real paths are
`GET /api/orgs/{orgID}/sessions/{sessionID}/digest?since=<seq>` and
`GET /api/orgs/{orgID}/sessions/{sessionID}/finalize-plan`.

## Required edit
Replace `/api/sessions/<id>/...` with
`/api/orgs/{orgID}/sessions/{sessionID}/...` in the data-flow section
and the finalize section. Drop the bare `/finalize-plan` shortcut; show
the full path.

## Implementation notes

Three lines edited in `docs/ARCHITECTURE.md`:

- **Line 97** (hook subcommands section): `GET /api/sessions/<id>/digest?since=<seq>` → `GET /api/orgs/{orgID}/sessions/{sessionID}/digest?since=<seq>`
- **Line 181** (turn data-flow section): `GET /api/sessions/<id>/digest?since=<seq>` → `GET /api/orgs/{orgID}/sessions/{sessionID}/digest?since=<seq>`
- **Line 338** (finalize section): `GET /finalize-plan` → `GET /api/orgs/{orgID}/sessions/{sessionID}/finalize-plan`

Canonical path shapes confirmed from `docs/openapi.yaml` (lines 2067 and 2714). No prose added, no semantic claims changed, no lines touched by sibling stories `gate-docs-arch-k8s-discovery` or `gate-docs-spec-arch-no-git-http-backend`.
