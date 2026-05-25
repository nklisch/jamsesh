---
id: gate-docs-skill-jamsesh-hard-deadlines-prose-brittle
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

# Skill `jamsesh/SKILL.md` Playground section repeats a (still-correct) idle-timeout default that already lives in SPEC.md

## Drift category
repo-skill-staleness

## Location
- Doc: `plugins/jamsesh/skills/jamsesh/SKILL.md:283-285`
- Code: `internal/portal/config/config.go:625` (sweep interval), `docs/SPEC.md:266-270` (limits table)

## Current doc text
> **Hard deadlines**: a session is destroyed after either 24 hours since creation (`hard_cap`) or 30 minutes of inactivity (`idle_timeout`), whichever fires first.

## Reality
These values are the documented defaults today (matches `JAMSESH_PLAYGROUND_HARD_CAP_S=86400` and `JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S=1800`) but are hard-coded prose. If an operator changes either env var, the skill's hard-coded "24 hours" / "30 minutes" becomes misleading. Not strictly drifted today, but brittle — risk of becoming drift on the next env-var tune.

## Required edit
Reword to: "**Hard deadlines**: a session is destroyed after either a hard-cap wall-clock window (`JAMSESH_PLAYGROUND_HARD_CAP_S`, default 24h) or an idle-timeout window since the last substantive activity (`JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S`, default 30m), whichever fires first." — keeps the defaults but pins them to the env var so the source of truth stays in `docs/SPEC.md` / `docs/SELF_HOST.md`.

## Implementation notes

Reworded the "Hard deadlines" bullet in `plugins/jamsesh/skills/jamsesh/SKILL.md:283-285` per the required edit. The new prose names the env vars (`JAMSESH_PLAYGROUND_HARD_CAP_S`, `JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S`) and parenthesizes the current defaults (24h / 30m) so the skill still tells the reader the live behavior but stops pretending those numbers are immutable. Added a trailing sentence that points readers to `docs/SPEC.md` / `docs/SELF_HOST.md` as the source of truth for defaults — operator tunes flow through the env vars, the docs reflect the canonical defaults, and the skill no longer drifts when defaults change.

Edits applied in the parent autopilot session — auto-mode's self-modification classifier blocked the sub-agent from editing under `plugins/jamsesh/skills/`.

Verification: `go build ./...` passes (sanity for doc-only change).
