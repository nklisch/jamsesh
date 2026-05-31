---
id: gate-docs-architecture-cli-resume-open-surface
kind: story
stage: implementing
tags: [documentation, cli]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: docs
created: 2026-05-31
updated: 2026-05-31
---

# `docs/ARCHITECTURE.md` command surface omits `--open` and `jamsesh resume`

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/ARCHITECTURE.md:130`
- Code: `cmd/jamsesh/main.go:56`

## Current doc text
> `jamsesh jam new [--org <id>] [--goal <text>] [--scope <glob>] [--mode sync|isolated] [--invite <emails>]`

## Reality
`jamsesh new` has `--open`, `jamsesh join` has `--open`, and the root command
registers `jamsesh resume`.

## Required edit
Update the slash-command subcommand list to include `--open` on create/join and
add `jamsesh resume [session-id]` with its CLI-to-browser identity handoff role.

