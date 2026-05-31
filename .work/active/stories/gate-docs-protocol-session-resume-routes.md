---
id: gate-docs-protocol-session-resume-routes
kind: story
stage: implementing
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: docs
created: 2026-05-31
updated: 2026-05-31
---

# `docs/PROTOCOL.md` REST catalog omits session-resume endpoints

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/PROTOCOL.md:79`
- Code: `cmd/portal/main.go:997`

## Current doc text
> `docs/openapi.yaml` is the canonical OpenAPI 3.0.3 description of every route below.

## Reality
The portal now mounts `POST /api/session-resumes` for authenticated minting and
`POST /api/session-resumes/exchange` as an unauthenticated token exchange, but
the human-readable route catalog does not list either endpoint.

## Required edit
Add a session-resume subsection describing mint versus exchange, including bearer
auth for mint and unauthenticated resume-token credential semantics for exchange.

