---
id: epic-finalize-flow-plan-generation
kind: feature
stage: drafting
tags: [portal]
parent: epic-finalize-flow
depends_on: [epic-portal-api-events-log, epic-portal-api-sessions-rest, epic-portal-foundation-http-skeleton, epic-portal-git-storage]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Finalize Flow — Plan Generation

## Brief

The portal-side surface that backs finalize. Owns the `finalize_locks`
table (concurrent-finalize coordination), the plan-generation
endpoint that computes the cherry-pick script body from a curated
commit list, and the "Mark as shipped" status transition.

**Endpoints delivered**:

- `POST /api/sessions/<id>/finalize/lock` — acquire the finalize lock
  for this session. Body: optional `override: true` to take a lock
  held by another member (creates a fresh lock superseding theirs).
  Returns `{lock_id, expires_at}`. Idempotent if the caller already
  holds the lock.
- `PATCH /api/sessions/<id>/finalize/lock/<lock_id>` — update the
  curation state (selected commits, target branch name). Body:
  `{selected_commit_shas: [], target_branch: string, base_sha:
  string}`. Resets the 30-min idle timer.
- `DELETE /api/sessions/<id>/finalize/lock/<lock_id>` — release the
  lock manually.
- `GET /api/sessions/<id>/finalize-plan?lock_id=<id>` — returns the
  cherry-pick plan computed from the lock's curation state + live
  bare-repo state (per-commit author/message via go-git from
  `epic-portal-git-storage`). Response shape includes:
  - `plan_id` (just `<session_id>:<lock_id>` — opaque to clients)
  - `summary` — the plain-English summary the portal UI displays:
    target branch name, base sha + short message, N commits in order
    with author + summary + sha
  - `script` — the bash script body (cherry-pick sequence + verbose
    logging + clean-failure messages)
  - `lock_status` — `held_by: account_id`, `expires_at`, `is_caller:
    bool`
- `POST /api/sessions/<id>/mark-shipped` — manual status transition
  from `finalizing` to `ended` with `end_reason: "shipped"`. Recorded
  in archived-session stubs as the distinguishing reason vs.
  `abandoned`. Emits `session.ended` event.

**Lock semantics** (locked at epic-design):

- 30-minute idle auto-release based on last `PATCH` timestamp.
- Other members see lock status in their next API/WebSocket update;
  the portal-UI curation-view feature surfaces "Alice is finalizing —
  wait or override."
- Override creates a new lock; old lock becomes a ghost
  (auto-releases without effect).

**Schema additions** (sqlc migration owned by this feature):

- `finalize_locks` — `id`, `org_id`, `session_id`, `acquired_by_account_id`,
  `acquired_at`, `last_activity_at`, `selected_commit_shas` (JSON),
  `target_branch`, `base_sha`, `superseded_by_lock_id` (nullable).
- `archived_sessions` adds `end_reason` enum
  (`shipped | abandoned | timeout`) — owned by `epic-portal-git-storage`
  schema but this feature ensures the `shipped` value is set on the
  mark-shipped transition.

**Plan determinism**: the cherry-pick script pins to the curated SHAs
at the moment the plan was generated. Even if draft advances between
plan generation and `jamsesh finalize-run` execution, the script
references the exact SHAs the human reviewed. Plan-generation reads
SHAs from the lock state and resolves commit metadata from the bare
repo via go-git.

**Generated-contracts scope**: the endpoints above are added to
`docs/openapi.yaml` per the SPEC.md generated-contracts decision.
Component schemas added: `FinalizeLock`, `LockStatus`, `PlanResponse`,
`MarkShippedRequest`. The Go server handler implements the
oapi-codegen-generated `ServerInterface` methods.

Does NOT cover the curation UI (`portal-ui-curation-view` feature).
Does NOT cover the local execution command
(`plugin-finalize-command`). Does NOT cover finalize-flow-wide e2e
testing (each component feature owns its own integration tests).

## Epic context

- Parent epic: `epic-finalize-flow`
- Position in epic: backend foundation for the cross-component slice.
  Both UI curation view and plugin finalize command consume this
  feature's endpoints.

## Foundation references

- `docs/ARCHITECTURE.md` — Reconciliation (local) section (the
  cherry-pick plan model)
- `docs/UX.md` — Flow: finalizing (the user-facing flow this backs)
- `docs/PROTOCOL.md` — REST API > Sessions (`POST /finalize`,
  `GET /finalize-plan` shapes); the OpenAPI YAML is the precise
  contract.
- `docs/VISION.md` — What you get (a finalized branch you push on
  your own terms)

## Inherited epic design decisions

- **Concurrent finalize lock**: 30-min idle auto-release; override flow
  creates a new lock that supersedes; per-account hold.
- **Plan determinism**: plan pins to curated SHAs at generation time.
- **Mark-shipped is manual**: portal can't detect the source-remote
  push; explicit click is the user's signal.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
