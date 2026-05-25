---
id: gate-docs-changelog-playground-hardcap-default-wrong
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

# CHANGELOG.md v0.4.0: HardCap default wrong (says 2h, code is 24h)

## Drift category
foundation-doc-assertion

## Location
- Doc: `CHANGELOG.md:39` (v0.4.0 "Ephemeral anonymous playground" bullet)
- Code: `internal/portal/config/config.go:478` (`PlaygroundHardCapS: 86400`)
- Cross-check docs: `docs/SPEC.md:268` (table row: 24 h / 86400 s), `docs/SELF_HOST.md:1544` (`86400 (24h)`)

## Current doc text
> Lifecycle is bounded by configurable `IdleTimeout`
> (default 30m) and `HardCap` (default 2h); abuse is bounded by a
> per-IP/hour create cap and a per-session content cap enforced at
> pre-receive time.

## Reality
The production default for `JAMSESH_PLAYGROUND_HARD_CAP_S` is `86400` seconds
(**24 hours**), set in `internal/portal/config/config.go:478`. `SPEC.md`,
`SELF_HOST.md`, and `SECURITY.md` all document 24h consistently — only
`CHANGELOG.md` is stale at 2h. Tests use 24h (`handler_test.go:319`:
`HardCap: 24 * time.Hour`).

## Required edit
In `CHANGELOG.md:39`, change `HardCap` (default 2h) to `HardCap` (default 24h)
in the v0.4.0 release bullet. No "previously" prose — the changelog entry is
the historical record of what shipped in v0.4.0, and what shipped was 24h.
