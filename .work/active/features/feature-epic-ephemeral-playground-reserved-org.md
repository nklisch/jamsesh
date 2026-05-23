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

## Design decisions

Locked at `--only-questions` time. Feature-design Phase 5 inherits these
as fixed input.

- **Reserved org slug**: hardcoded `playground`. Single canonical value
  across every deployment. Docs, support material, observability
  dashboards, and `pre-receive` checks can hard-reference `org:playground`
  without env-var lookup. If an operator has a pre-existing real org
  named `playground` and tries to enable the feature, the startup
  provisioning hook detects the slug collision (the existing row has
  no `org_protected: true`), logs a clear conflict error, and refuses
  to enable playground until the operator renames their org. No silent
  upgrade-then-merge surprise.

- **Disable-flip behavior** (`JAMSESH_PLAYGROUND_ENABLED` true → false):
  reject new creates immediately; let active sessions age out
  naturally. `POST /api/playground/sessions` returns
  `503 playground.disabled`; the join endpoint also returns 503 for new
  joiners. Existing in-flight sessions keep running through their
  normal idle / hard-cap lifecycles — the destruction sweep continues
  to fire even when the create endpoint is off. Within 24h (hard cap),
  the deployment is naturally playground-free. Lowest surprise to
  in-flight participants. Operators who need an immediate shutdown
  can still trigger a manual destruction sweep via ops tooling
  (out-of-scope for this feature).

- **`org_protected` scope**: block delete + rename only. The flag
  rejects `DELETE /api/orgs/{id}` and `PATCH /api/orgs/{id}` mutations
  to name/slug. Member-add operations against the playground org are
  still allowed (preserves flexibility for a future ops/observability
  use case where a human is added to inspect the playground org's
  sessions). Other writes (session create, member-leave, etc.) go
  through their own auth + tenancy checks; `org_protected` is
  specifically about the org row's identity stability.

- **Provisioning on upgrade**: only when the operator opts in. Existing
  deployments stay byte-identical after upgrading to the version that
  ships playground. No `playground` org row appears until the operator
  explicitly sets `JAMSESH_PLAYGROUND_ENABLED=true` and restarts the
  portal. First-opt-in startup seeds the org row idempotently. Loss:
  `org_protected: true` can't be enforced on a pre-existing user-owned
  `playground` org row that survives an upgrade — addressed by the
  startup-conflict check from the first decision above (the conflict
  detection runs every boot, not just at first provisioning).
