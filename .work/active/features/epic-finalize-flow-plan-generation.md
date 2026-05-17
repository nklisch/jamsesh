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
updated: 2026-05-17
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
  curation state. Body:
  ```
  {
    selected_commit_shas: [],           // ordered list
    target_branch: string,
    base_sha: string,
    mode: "squash" | "preserve",        // defaults to "squash" on first PATCH
    commit_message: string | null       // required when mode=squash
  }
  ```
  Resets the 30-min idle timer.
- `DELETE /api/sessions/<id>/finalize/lock/<lock_id>` — release the
  lock manually.
- `GET /api/sessions/<id>/finalize-plan?lock_id=<id>` — returns the
  plan computed from the lock's curation state + live bare-repo state
  (per-commit author/message via go-git from `epic-portal-git-storage`).
  Response shape:
  - `plan_id` — `<session_id>:<lock_id>`, opaque to clients
  - `mode` — `"squash"` (default) or `"preserve"`
  - `summary` — plain-English summary the portal UI and the plugin's
    `finalize-run` both display: target branch, base sha + short
    message, N commits in order with author + subject + sha, and
    (squash mode) the composed commit message + Co-authored-by
    trailer list
  - `script` — bash script body. In squash mode: `cherry-pick
    --no-commit <c1> <c2> ... <cN>` + `git commit -F <heredoc>` with
    the composed message. In preserve mode: per-commit `git
    cherry-pick`. Both shapes include verbose per-step logging and
    clean-failure messages.
  - `commit_message` — composed message body (squash mode only):
    subject = curated commit message field, body = bulleted list of
    selected commit subjects in order, footer = `Co-authored-by:
    Name <email>` for every distinct author across the selection
  - `co_authors` — `[{name, email, account_id}]` (squash mode only)
    so the UI can render the contributor chip row
  - `lock_status` — `held_by: account_id`, `expires_at`, `is_caller:
    bool`
  - `fetch_source` — `{kind: "local" | "https", path?: string,
    remote_url?: string, token_expires_at?: string}`. When the
    binary fetches the response and detects a local session
    checkout, it ignores this and uses the local path. When the
    binary needs the HTTPS fallback, it mints an ephemeral
    fetch-only token via `POST /api/sessions/<id>/finalize/
    fetch-token` (separate endpoint, ~5 min TTL) and uses the
    returned URL.
- `POST /api/sessions/<id>/finalize/fetch-token` — issue a
  short-TTL (5-min) fetch-only token usable as the HTTP Basic
  password against the portal's git smart-HTTP endpoint for this
  session's bare repo. Used by the plugin's HTTPS-fallback path
  when no local session checkout is present. Token is single-use
  on the issuance call but the underlying credential is valid for
  the TTL window so `git fetch` can complete.
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

- `finalize_locks` — `id`, `org_id`, `session_id`,
  `acquired_by_account_id`, `acquired_at`, `last_activity_at`,
  `selected_commit_shas` (JSON, ordered), `target_branch`, `base_sha`,
  `mode` (enum: `squash | preserve`, default `squash`),
  `commit_message` (TEXT, nullable; required in app logic when
  `mode = squash`), `superseded_by_lock_id` (nullable).
- `archived_sessions` adds `end_reason` enum
  (`shipped | abandoned | timeout`) — owned by `epic-portal-git-storage`
  schema but this feature ensures the `shipped` value is set on the
  mark-shipped transition.

**Plan determinism**: the script pins to the curated SHAs at the moment
the plan was generated. Even if draft advances between plan generation
and `jamsesh finalize-run` execution, the script references the exact
SHAs the human reviewed. Plan-generation reads SHAs from the lock
state and resolves commit metadata from the bare repo via go-git.

**Draft linearization**: when populating the default selection at
lock-acquire time (and on any subsequent draft-tip resolution),
plan-generation walks `draft` via first-parent and emits the
underlying leaf agent commits in chronological order. Auto-merger
merge commits are NOT included in the default selection — only the
leaf commits they integrated. This is what the curation view shows
as "From draft" and what feeds the cherry-pick / cherry-pick
--no-commit list.

**Squash-message composition**: when the lock is in `mode = squash`,
the server constructs the `commit_message` body on every PATCH /
plan-fetch. Subject defaults to the session goal (truncated to 72
chars); user-supplied edits via PATCH take precedence. Body is a
bulleted list of selected commit subjects in selection order.
Footer is one `Co-authored-by: <Display Name> <email>` line per
distinct author across the selection (sorted by first appearance).
The composed message is included in the plan response so the
plugin and UI can both display + echo it.

**Generated-contracts scope**: endpoints added to `docs/openapi.yaml`
per the SPEC.md generated-contracts decision. Component schemas
added: `FinalizeLock`, `LockStatus`, `PlanResponse`, `PlanMode`
(enum), `CoAuthor`, `FetchSource`, `FetchToken`,
`MarkShippedRequest`. Go server handler implements the
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
- **Finalization mode**: squash is the default; preserve-all is opt-in.
  The lock and the plan response both carry `mode` so the script body
  branches correctly.
- **Squash authorship**: `author` of the squash commit is the user
  running `finalize-run`; every distinct contributor across the
  selection gets a `Co-authored-by` trailer. Server constructs the
  trailer list.
- **Linearized merge handling**: server walks `draft` first-parent and
  emits leaf agent commits only; auto-merger merge commits never
  appear in the default selection or the cherry-pick list.
- **Commit-source strategy**: the `fetch_source` field in the plan
  response carries the HTTPS-fallback URL and ephemeral-token TTL.
  The local-first path is plugin-side — the binary ignores
  `fetch_source` when a local session checkout is on disk.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
