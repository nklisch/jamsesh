---
id: gate-docs-readme-stale-slash-command-list
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

# README.md describes the deleted narrow slash commands

## Drift category
readme-staleness

## Location
- Doc: `README.md:79-81`
- Code: `plugins/jamsesh/skills/jam/SKILL.md`, `plugins/jamsesh/skills/finalize/SKILL.md` (the only two skills that remain after consolidation)

## Current doc text
> The jamsesh plugin runs inside Claude Code and gives each agent the `join`, `status`, `fork`, and `mode` slash commands, plus auto-loading session context so agents know how to participate in a jam.

## Reality
The skill-consolidation feature deleted `plugins/jamsesh/skills/{status,fork,mode}/SKILL.md` and folded `join` into a single intent-driven `/jamsesh:jam` skill. The plugin now exposes exactly two slash commands: `/jamsesh:jam` and `/jamsesh:finalize`.

## Required edit
Replace the sentence with: "The jamsesh plugin runs inside Claude Code and gives each agent the `/jamsesh:jam` intent-driven entry point (covers create, join, status, fork, mode) and `/jamsesh:finalize`, plus auto-loading session context so agents know how to participate in a jam."

## Implementation notes

Replaced the `join`/`status`/`fork`/`mode` slash-command sentence in `README.md:79-81` with the post-consolidation `/jamsesh:jam` (intent-driven entry covering create/join/status/fork/mode) + `/jamsesh:finalize` form.

Verified: Foundation docs are markdown — no build/test step. Edits preserve the rolling-foundation discipline (no "previously" prose, no "in v1.x" notes; assertions replaced in place).
