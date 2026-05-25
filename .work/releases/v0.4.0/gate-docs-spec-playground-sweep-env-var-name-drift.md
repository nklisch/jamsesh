---
id: gate-docs-spec-playground-sweep-env-var-name-drift
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

# SPEC.md asserts wrong env var name for the playground destruction sweep interval

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/SPEC.md:261`
- Code: `internal/portal/config/config.go:625`

## Current doc text
> The destruction sweep runs every 60 seconds (configurable via `JAMSESH_PLAYGROUND_SWEEP_INTERVAL_S`).

## Reality
The env var is `JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S` (matches `docs/ARCHITECTURE.md:82` and `docs/SELF_HOST.md:1548`, and the live `readEnvInt(...)` call). The SPEC.md table at line 266-270 even lacks a row for this knob entirely; the prose at line 261 invents an env var that does not exist.

## Required edit
Replace `JAMSESH_PLAYGROUND_SWEEP_INTERVAL_S` with `JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S`. Add a row to the limits table at lines 264-270 for the sweep interval (`60s` default, env var `JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S`).

## Implementation notes

Replaced `JAMSESH_PLAYGROUND_SWEEP_INTERVAL_S` with `JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S` in the prose at `docs/SPEC.md:260-262`. Added a `Destruction sweep interval` row to the limits table (`60s` default, env var name matches the live config).

Verified: Foundation docs are markdown — no build/test step. Edits preserve the rolling-foundation discipline (no "previously" prose, no "in v1.x" notes; assertions replaced in place).
