---
id: feature-epic-ephemeral-playground-reserved-org
kind: feature
stage: drafting
tags: [portal]
parent: epic-ephemeral-playground
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Reserved playground org provisioning + config

## Brief

Adds the config knobs that gate playground availability and the
idempotent startup hook that provisions the reserved system-owned
`playground` org. When `JAMSESH_PLAYGROUND_ENABLED=true`, `cmd/portal/main.go`
seeds the org row on every boot (idempotent via `slug = 'playground'`
uniqueness); when false, no provisioning runs and any playground REST
route returns `503` to signal the feature is disabled for this
deployment.

The reserved org gets an `org_protected: true` boolean column on `orgs`
set at provisioning time. Any handler that mutates or deletes orgs must
check this flag and reject with `409 org.protected` — this is the data-
layer guard against the playground org being accidentally destroyed by
an unrelated future feature (defense in depth, not handler-level only).

Config knobs introduced (per the strategic-decisions section of the
parent epic):
- `JAMSESH_PLAYGROUND_ENABLED` (bool, default `false`)
- `JAMSESH_PLAYGROUND_IDLE_TIMEOUT` (duration, default `30m`)
- `JAMSESH_PLAYGROUND_HARD_CAP` (duration, default `24h`)
- Abuse-cap env vars are listed and reserved here but consumed in
  `session-lifecycle`: `JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR` (default
  `3`), `JAMSESH_PLAYGROUND_MAX_PARTICIPANTS` (default `5`),
  `JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES` (default `50 << 20`, 50 MiB).

This feature is config + startup substrate only. It does NOT add the
playground REST routes, the destruction worker, or anything user-
visible. Those live in `session-lifecycle`.

## Epic context
- Parent epic: `epic-ephemeral-playground`
- Position in epic: **wave 1 foundation** — no dependencies; required by
  `session-lifecycle` (wave 2) for the playground org row to exist
  before sessions can be created inside it.

## Foundation references
- `docs/SPEC.md` § Hard constraints + § Deployment shape — the
  multi-tenant invariant the reserved-org pattern preserves; the env-var
  list this feature extends
- `docs/ARCHITECTURE.md` § Data layer (multi-tenancy) — the
  "Reserved orgs" paragraph added at scope time describes this
  feature's runtime contract
- `docs/SELF_HOST.md` — env-var reference table roll-forward is owned
  by this feature's design pass

## Mockups
No UI surface — config + startup substrate. The parent epic's flow
mocks cover everything user-visible.
