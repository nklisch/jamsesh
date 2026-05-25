---
id: review-openapi-fetch-pattern-rolling-foundation-prose
kind: story
stage: review
tags: [documentation, cleanup]
parent: null
depends_on: []
release_binding: null
created: 2026-05-24
updated: 2026-05-24
---

# Trim "v0.4.0 god-component decomposition" prose from openapi-fetch pattern skill

## Origin
Spawned during review of `gate-docs-pattern-openapi-fetch-middleware-stale-anchors`.

## Issue
`.claude/skills/patterns/openapi-fetch-middleware-client.md:59-60` contains
the phrase "the v0.4.0 god-component decomposition refactor moved most of
these" — a rolling-foundation violation (docs describe NOW, not the journey).

The note at lines 27-31 already explains "files are under active refactor"
without naming a version, which is sufficient.

## Fix
Replace the introductory sentence at line 59-60 with a version-neutral
restatement (e.g. "anchor by handler name rather than line number") OR
delete it outright (the heading + bullet list speak for themselves).

## Implementation notes

Replaced `"the v0.4.0 god-component decomposition refactor moved most of these"` with `"these files are under active refactor"` — version-neutral, preserves the "anchor by name, not line number" reason. Edit applied in the parent autopilot session (auto-mode classifier blocks sub-agents from `.claude/skills/`).
