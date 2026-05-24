---
id: gate-docs-protocol-rest-route-catalog-missing-playground
kind: story
stage: review
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# PROTOCOL.md REST API route catalog does not list the four playground endpoints shipped in this bundle

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/PROTOCOL.md:104-152`
- Code: `internal/portal/playground/handler.go`, `docs/openapi.yaml:3190-3349`

## Current doc text
> Sections "Sessions", "Comments", "Session state (used by the local binary)", "Finalize machinery", "Git smart-HTTP (separate path tree)" — none contain a Playground section.

## Reality
Four playground REST routes exist and are documented in `docs/openapi.yaml`: `POST /api/playground/sessions`, `POST /api/playground/sessions/{id}/join`, `GET /api/playground/sessions/{id}`, `GET /api/playground/sessions/{id}/tombstone`. PROTOCOL.md's "REST API" section catalogs every other route family but is silent on the playground family.

## Required edit
Add a new "Playground" subsection in `docs/PROTOCOL.md` between "Finalize machinery" and "Git smart-HTTP" listing the four playground routes with one-line descriptions (auth requirement, purpose). Cross-reference the openapi.yaml fragment for full schemas.

## Implementation notes

Added a `### Playground` subsection to `docs/PROTOCOL.md` between Finalize machinery and Git smart-HTTP, listing the four playground REST routes (create, join, get, tombstone) with auth + status-code semantics.

Verified: Foundation docs are markdown — no build/test step. Edits preserve the rolling-foundation discipline (no "previously" prose, no "in v1.x" notes; assertions replaced in place).
