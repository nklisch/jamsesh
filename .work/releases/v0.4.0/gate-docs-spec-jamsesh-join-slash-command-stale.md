---
id: gate-docs-spec-jamsesh-join-slash-command-stale
kind: story
stage: done
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# SPEC.md still references deleted `/jamsesh:join` slash command

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/SPEC.md:227`
- Code: `plugins/jamsesh/skills/jam/SKILL.md:18-19`

## Current doc text
> **Join:** invite-only — emails or one-time-use join links. Recipients authenticate to the portal, accept the invite, run `/jamsesh:join <session>`.

## Reality
`/jamsesh:join` no longer exists. The consolidated surface is `/jamsesh:jam join <session>`. The skill-consolidation rollforward story corrected `docs/ARCHITECTURE.md` and `docs/UX.md` but missed this SPEC.md occurrence.

## Required edit
Replace `/jamsesh:join <session>` with `/jamsesh:jam join <session>`.

## Implementation notes

Replaced `/jamsesh:join <session>` with `/jamsesh:jam join <session>` at `docs/SPEC.md:227`.

Verified: Foundation docs are markdown — no build/test step. Edits preserve the rolling-foundation discipline (no "previously" prose, no "in v1.x" notes; assertions replaced in place).
