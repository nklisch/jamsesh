---
id: gate-docs-changelog-v0-4-0-entry-missing
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

# CHANGELOG.md has no v0.4.0 entry yet

## Drift category
changelog-gap

## Location
- Doc: `CHANGELOG.md:1` (latest entry is v0.3.1)
- Code: 80 bound items in `/tmp/bound-ids-v0.4.0.txt`

## Current doc text
> Top entry is `## v0.3.1` (2026-05-21); no `## v0.4.0` heading present.

## Reality
80 substrate items bound to v0.4.0 are shipping (epic-ephemeral-playground end-to-end, multiple refactor features, spec-discipline feature, foundation-doc rollups). The Changelog still ends at v0.3.1.

## Required edit
Composed in release-deploy Phase 5.5 — group by category (Features / Fixes / Refactor / Performance / Security / Documentation / Internal), one bullet per logical change. This item is resolved as part of the release-deploy changelog composition step.
