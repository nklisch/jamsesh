---
id: gate-docs-pattern-dual-dialect-stale-createsession-columns
kind: story
stage: implementing
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# Pattern skill `dual-dialect-mirror-queries.md` example shows a stale `CreateSession` column list

## Drift category
pattern-skill-staleness

## Location
- Doc: `.claude/skills/patterns/dual-dialect-mirror-queries.md:26-39`
- Code: `db/queries/sqlite/sessions.sql:1-5`, `db/queries/postgres/sessions.sql:1-5`

## Current doc text
> ```sql
> -- name: CreateSession :one
> INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at)
> VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
> RETURNING *;
> ```

## Reality
The bundle's playground-session migrations (00018) added three columns to `sessions` (`last_substantive_activity_at`, `hard_cap_at`, `idle_timeout_at`) and the `CreateSession` query was extended to 13 columns / 13 placeholders. The pattern's example shows the pre-bundle 10-column shape.

## Required edit
Update both example SQL blocks at lines 26-39 to match the current 13-column form (`id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, last_substantive_activity_at, hard_cap_at, idle_timeout_at`), with 13 placeholders.
