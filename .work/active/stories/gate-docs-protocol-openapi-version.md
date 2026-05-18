---
id: gate-docs-protocol-openapi-version
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

# PROTOCOL.md "OpenAPI 3.1" claim contradicts the pinned `openapi: 3.0.3` spec

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/PROTOCOL.md:79`
- Code: `docs/openapi.yaml:1` (`openapi: 3.0.3`); SPEC.md:36-42 documents
  why 3.0.3 is pinned until oapi-codegen v2.8.0

## Current doc text
> **Authoritative spec**: `docs/openapi.yaml` is the canonical OpenAPI
> 3.1 description of every route below.

## Reality
`docs/openapi.yaml` declares `openapi: 3.0.3` and is intentionally kept
on the 3.0-compatible subset until oapi-codegen v2.8.0 tags.

## Required edit
Change "canonical OpenAPI 3.1" to "canonical OpenAPI 3.0.3" in
PROTOCOL.md.
