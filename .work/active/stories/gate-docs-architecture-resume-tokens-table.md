---
id: gate-docs-architecture-resume-tokens-table
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

# `docs/ARCHITECTURE.md` table list omits `resume_tokens`

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/ARCHITECTURE.md:561`
- Code: `db/schema/sqlite.sql:83`

## Current doc text
> **Tables (high-level):**

## Reality
The bundle adds a `resume_tokens` table in both SQLite and Postgres schemas,
plus a `ResumeTokenStore` interface for single-use CLI resume exchange tokens.

## Required edit
Add `resume_tokens` to the high-level table list as the hashed, single-use,
expiring token store for CLI-to-browser session resume.

