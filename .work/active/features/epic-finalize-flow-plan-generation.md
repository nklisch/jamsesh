---
id: epic-finalize-flow-plan-generation
kind: feature
stage: done
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

## Design

### Architectural choice

Land the entire surface in a new package
`internal/portal/finalize/` that satisfies the oapi-codegen
`StrictServerInterface` methods for the seven endpoints this feature
adds. The package is structured as a `Handler` struct (analogous to
`internal/portal/sessions/Handler`) wired into the existing
`combinedHandler` in `cmd/portal/main.go`. Pulling finalize out of
`sessions/` keeps the package size small and makes the feature self-
contained, but the handler reuses the same `store.Store`, `events.Log`,
`storage.Service`, and `tokens.Service` collaborators — no new ports.

The lock state is the durable artifact (one `finalize_locks` row per
in-flight finalize). The plan is a deterministic view computed on each
`GET /finalize-plan` from the lock row (curated SHAs) + the bare repo
via go-git (commit metadata). The 30-minute idle release is checked
on read — every endpoint that touches a lock first compares
`now - last_activity_at > 30min` and either releases-and-fails or
releases-and-supersedes per call type. No background sweeper needed:
read-time check is sufficient because every lock-affecting path goes
through these endpoints.

`fetch_source` is computed by the server but the plugin chooses
local-first. The server populates the HTTPS-fallback URL and mints
the ephemeral fetch token only on `POST /finalize/fetch-token` (kept
separate so a plan fetch doesn't accidentally provision a token the
plugin will discard). The fetch-token endpoint reuses
`tokens.Service` by issuing a regular access token with a custom
short TTL (5 min); the existing `tokens.Validate` path accepts it
unchanged because TTL is per-row in `oauth_tokens`.

### Endpoints (paths + operationIDs)

All paths nest under `/api/orgs/{orgID}/sessions/{sessionID}` to
match the org-scoped pattern already established by sessions-rest.

| Method  | Path                            | OperationID         |
|---------|---------------------------------|---------------------|
| POST    | `…/finalize/lock`               | `acquireFinalizeLock` |
| PATCH   | `…/finalize/lock/{lockID}`      | `patchFinalizeLock`   |
| DELETE  | `…/finalize/lock/{lockID}`      | `releaseFinalizeLock` |
| GET     | `…/finalize-plan`               | `getFinalizePlan`     |
| POST    | `…/finalize/fetch-token`        | `issueFetchToken`     |
| POST    | `…/mark-shipped`                | `markSessionShipped`  |

### Schema additions (sqlc dual-dialect migration 00010)

`finalize_locks` table:

```sql
CREATE TABLE finalize_locks (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    acquired_by_account_id TEXT NOT NULL REFERENCES accounts(id),
    acquired_at DATETIME NOT NULL,
    last_activity_at DATETIME NOT NULL,
    selected_commit_shas TEXT NOT NULL DEFAULT '[]',  -- JSON array
    target_branch TEXT NOT NULL DEFAULT '',
    base_sha TEXT NOT NULL DEFAULT '',
    mode TEXT NOT NULL DEFAULT 'squash'
        CHECK (mode IN ('squash','preserve')),
    commit_message TEXT,
    superseded_by_lock_id TEXT REFERENCES finalize_locks(id),
    released_at DATETIME
);
CREATE INDEX finalize_locks_session_idx ON finalize_locks(session_id);
CREATE INDEX finalize_locks_active_idx ON finalize_locks(session_id)
    WHERE released_at IS NULL AND superseded_by_lock_id IS NULL;
```

(Postgres version uses `JSONB` for `selected_commit_shas` and
`TIMESTAMPTZ`; sqlite keeps TEXT for JSON. The sqlite partial-index
clause is supported since 3.8.)

The pre-existing `sessions.finalize_locked_by_account_id` column
(from migration 00006) is the cached caller-facing pointer kept in
sync by lock acquire/release for cheap visibility in the
session-detail response. The authoritative lock state lives in
`finalize_locks`.

`archived_sessions.end_reason` already supports `'shipped'` via the
existing CHECK constraint (added under epic-portal-git-storage as
part of 00002 schema; the `shipped` value is set on `mark-shipped`).
**Migration 00010 adds the `'shipped'` value to the CHECK constraint
on `sessions.end_reason`** — see story 3 design notes — and adds
`'shipped'` to the equivalent constraint on `archived_sessions` if
not already present. (Current 00002 lists `finalize|abandon|timeout`
— the migration adds `shipped` as an accepted value to align with
the protocol naming and the manual mark-shipped flow.)

### Component schemas (additions to `docs/openapi.yaml`)

- `FinalizeLock` — `{ id, session_id, acquired_by_account_id,
  acquired_at, last_activity_at, expires_at, selected_commit_shas:
  string[], target_branch, base_sha, mode: PlanMode, commit_message:
  string|null }`
- `PlanMode` — `enum: [squash, preserve]`
- `AcquireFinalizeLockRequest` — `{ override?: boolean }`
- `PatchFinalizeLockRequest` — `{ selected_commit_shas: string[],
  target_branch: string, base_sha: string, mode: PlanMode,
  commit_message?: string|null }`
- `PlanResponse` — `{ plan_id, mode, summary, script,
  commit_message?, co_authors?, lock_status: LockStatus, fetch_source:
  FetchSource, selected_commits: PlanCommit[], target_branch,
  base_sha }`
- `PlanCommit` — `{ sha, author_name, author_email, account_id?,
  subject, committed_at }` (the per-commit metadata the UI renders
  in the summary panel)
- `CoAuthor` — `{ name, email, account_id? }`
- `LockStatus` — `{ lock_id, held_by_account_id, acquired_at,
  last_activity_at, expires_at, is_caller: boolean }`
- `FetchSource` — `{ kind: enum[local, https], remote_url?: string,
  token_expires_at?: string (date-time) }`
- `FetchTokenResponse` — `{ token, remote_url, expires_at }`
- `MarkShippedRequest` — `{ final_branch_name?: string }` (optional —
  recorded into archived stub for distinguishing per-jam ship targets)

### Lock lifecycle (state machine)

```
                 idle/30min
                 ┌─────────┐
                 ▼         │
[no lock] --acquire--> [held(A)] --PATCH--> [held(A), touched]
                 │                                  │
                 │ DELETE(A)                        │ DELETE(A)
                 │ override(B)                      │
                 ▼                                  │
            [released]                              │
                 ▲                                  │
                 │   override(B)                    │
                 │   sets superseded_by=B.id        │
                 │   on A                           │
                 └──────────────────────────────────┘
```

- **Acquire** (`POST .../finalize/lock` with optional `override`):
  - If no active lock for the session: insert new lock row, set
    `sessions.finalize_locked_by_account_id = caller`, transition
    session.status `active → finalizing` (idempotent if already
    `finalizing`), emit `session.finalizing` event, return
    `LockStatus` + new `lock_id`.
  - If active lock held by caller: idempotent — return existing lock.
  - If active lock held by someone else AND `last_activity_at` is
    older than 30 min: release the stale lock (set `released_at`)
    and proceed as no-lock case.
  - If active lock held by someone else AND fresh AND `override`
    false: `409 finalize.lock_held_by_other`.
  - If active lock held by someone else AND fresh AND `override`
    true: set old lock's `superseded_by_lock_id` to new lock id,
    create new lock for caller, update sessions pointer.

- **Patch** (`PATCH .../finalize/lock/{lockID}`):
  - Verify lock row exists, not released, not superseded, caller is
    `acquired_by_account_id`. If idle > 30 min: auto-release and
    `409 finalize.lock_expired`.
  - Otherwise update curation columns (`selected_commit_shas`,
    `target_branch`, `base_sha`, `mode`, `commit_message`), set
    `last_activity_at = now()`, return `FinalizeLock` row.

- **Release** (`DELETE .../finalize/lock/{lockID}`):
  - Idempotent; only caller can release. Sets `released_at`, clears
    `sessions.finalize_locked_by_account_id`. Session status stays
    `finalizing` (release-without-shipped is a separate decision the
    portal-UI surfaces; an abandoned finalize is the user's signal).

### Plan generation (`GET .../finalize-plan?lock_id=`)

1. Load lock row by `lock_id`. Verify it belongs to the path
   `sessionID`. Idle-check (30 min). Return `404 finalize.lock_not_found`
   for missing, `409 finalize.lock_expired` for idle, `409
   finalize.lock_superseded` if `superseded_by_lock_id` is set.
2. Resolve session-membership for caller; build `LockStatus` with
   `is_caller = (caller == acquired_by)`.
3. Open the bare repo via `storage.RepoPath` + `gogit.PlainOpen`.
4. Resolve each curated SHA → `*object.Commit`. If any SHA is
   missing, return `409 finalize.commit_missing` with the offending
   sha in `details`.
5. Build `selected_commits` list preserving the order in the lock's
   `selected_commit_shas` JSON.
6. If `mode == squash`:
   - Compose `co_authors` list: walk `selected_commits` in order;
     for each commit collect `{Author.Name, Author.Email}`; dedupe
     by lowercase email; preserve first-appearance order.
   - Map emails to `account_id` best-effort via
     `store.GetAccountByEmail` (returns null in the response field
     when no portal account matches — the trailer still works on
     GitHub etc).
   - Compose `commit_message`:
     - **Subject**: if the lock's `commit_message` field is non-null
       use the first line (truncated to 72 chars). Else use session
       goal truncated to 72 chars with ellipsis at a word boundary.
     - **Body** (after blank line): `- <subject>` bullets in
       selection order, where `<subject>` is the first line of each
       commit's message stripped of trailers.
     - **Footer** (after blank line): one `Co-authored-by:
       <Display Name> <email>` per co-author, sorted by first
       appearance.
7. Build `script` body:
   - **Squash** template:
     ```
     #!/usr/bin/env bash
     set -euo pipefail
     echo "==> Fetching session refs"
     git fetch <fetch-source-arg> <base-sha-and-tip-refspec>
     echo "==> Creating target branch <target> at <base-sha-short>"
     git checkout -b "<target>" <base-sha>
     echo "==> Staging <N> curated commits"
     git cherry-pick --no-commit <sha1> <sha2> ... <shaN>
     echo "==> Composing squash commit"
     git commit --author="<runner-name> <runner-email>" -F - <<'JAMSESH_MSG'
     <composed message body>
     JAMSESH_MSG
     echo "==> Done. Push when ready: git push origin <target>"
     ```
     With `<fetch-source-arg>` left as a literal placeholder
     `$JAMSESH_FETCH_REMOTE` that the plugin substitutes (local path
     or HTTPS URL with token). The runner-name/email also literal
     `$JAMSESH_RUNNER_NAME`/`$JAMSESH_RUNNER_EMAIL` substituted
     plugin-side.
   - **Preserve** template: same prologue, then `git cherry-pick
     <sha1> ... <shaN>` (no `--no-commit`, no synthetic squash
     commit). Each cherry-pick retains its own author + message.
8. Build `fetch_source` with `kind: "https"`,
   `remote_url: "<portalURL>/git/<orgID>/<sessionID>.git"`, and
   `token_expires_at: null` (token is minted only on the dedicated
   fetch-token endpoint). The plugin sets `kind: local` itself when
   it detects a local checkout.
9. Return `PlanResponse`. `plan_id = "<sessionID>:<lockID>"`.

### Squash-message composer (deterministic)

Function in `internal/portal/finalize/message.go`:

```go
// ComposeSquashMessage builds the squash commit message body from
// curated commits. The result is bytewise-deterministic for the same
// input — every call produces the same bytes regardless of map
// iteration order, locale, or clock.
func ComposeSquashMessage(
    sessionGoal string,
    userOverrideSubject string, // empty = use sessionGoal
    commits []*object.Commit,   // ordered as curated
) (subject string, body string, coAuthors []CoAuthor)
```

The composer is its own file with its own unit test surface so the
exact byte layout can be golden-tested. Co-author order is "first
appearance" (NOT alphabetical) to keep PR rendering stable across
re-fetches of the same plan.

### Idle-lock check helper

```go
// FinalizeLockTTL is the inactivity window before a held lock is
// considered abandoned. Locked at epic-design.
const FinalizeLockTTL = 30 * time.Minute

// IsLockExpired returns true if the lock's last_activity_at + TTL
// is in the past. Pure function for test seeding.
func IsLockExpired(lastActivity time.Time, now time.Time) bool
```

### Fetch-token endpoint

`POST /api/orgs/{orgID}/sessions/{sessionID}/finalize/fetch-token`
mints a regular access token with a 5-minute TTL bound to the calling
account. Uses a new `tokens.Service.IssueShortLived(ctx, accountID,
ttl)` method that wraps the same `CreateOAuthToken` query path with
a caller-supplied expiry. Validation is unchanged — the existing
basic-auth middleware on `/git/...` accepts it because TTL is
per-row. The response carries `{token, remote_url, expires_at}`
where `remote_url` already has `https://x-access-token:<token>@…`
formatting baked in for plugin convenience.

(Membership check happens before issuance; only session members can
mint a fetch token.)

### Mark-shipped endpoint

`POST /api/orgs/{orgID}/sessions/{sessionID}/mark-shipped` transitions
the session from `finalizing → ended` with `end_reason = "shipped"`.
Idempotent if already ended with `shipped`. Conflict (`409
session.not_finalizing`) if status is `active` (must finalize first).
Conflict if `ended` with a different reason. Emits `session.ended`
event with `reason: "shipped"` (note: the `SessionEndedPayload`
enum needs `shipped` added — see openapi changes below). Optional
`final_branch_name` is recorded on the archived-session row so the
listing UI can show "shipped as `<branch>`" in archived stubs.

### Files & Go signatures

```
internal/portal/finalize/
  doc.go                          // package doc
  handler.go                      // Handler struct + New + collaborators
  lock_acquire.go                 // AcquireFinalizeLock (POST .../finalize/lock)
  lock_patch.go                   // PatchFinalizeLock (PATCH .../finalize/lock/{lockID})
  lock_release.go                 // ReleaseFinalizeLock (DELETE .../finalize/lock/{lockID})
  lock_check.go                   // IsLockExpired, FinalizeLockTTL, common reads
  plan.go                         // GetFinalizePlan (GET .../finalize-plan)
  script.go                       // BuildSquashScript / BuildPreserveScript
  message.go                      // ComposeSquashMessage (deterministic)
  fetch_token.go                  // IssueFetchToken (POST .../finalize/fetch-token)
  mark_shipped.go                 // MarkSessionShipped (POST .../mark-shipped)
  *_test.go                       // per-file table-driven tests
  testdata/
    squash_message.golden.txt     // golden output for ComposeSquashMessage
    squash_script.golden.txt      // golden output for BuildSquashScript
    preserve_script.golden.txt    // golden output for BuildPreserveScript

internal/portal/tokens/
  service.go                      // add IssueShortLived to Service interface
  service_impl.go                 // implement IssueShortLived

internal/db/migrations/{sqlite,postgres}/00010_finalize_locks.sql

db/queries/{sqlite,postgres}/finalize_locks.sql

docs/openapi.yaml                 // +6 paths, +12 schemas, +1 enum value
```

Key Go signatures:

```go
// handler.go
type Handler struct {
    store   store.Store
    storage storage.Service
    events  *events.Log
    tokens  tokens.Service
    portalURL string
}

func New(s store.Store, stor storage.Service, log *events.Log,
    tok tokens.Service, portalURL string) *Handler

// Each endpoint method satisfies the corresponding StrictServerInterface
// method generated by oapi-codegen. Pattern matches sessions/handler.go.
func (h *Handler) AcquireFinalizeLock(ctx context.Context,
    req openapi.AcquireFinalizeLockRequestObject) (
    openapi.AcquireFinalizeLockResponseObject, error)

func (h *Handler) PatchFinalizeLock(ctx context.Context,
    req openapi.PatchFinalizeLockRequestObject) (
    openapi.PatchFinalizeLockResponseObject, error)

func (h *Handler) ReleaseFinalizeLock(ctx context.Context,
    req openapi.ReleaseFinalizeLockRequestObject) (
    openapi.ReleaseFinalizeLockResponseObject, error)

func (h *Handler) GetFinalizePlan(ctx context.Context,
    req openapi.GetFinalizePlanRequestObject) (
    openapi.GetFinalizePlanResponseObject, error)

func (h *Handler) IssueFetchToken(ctx context.Context,
    req openapi.IssueFetchTokenRequestObject) (
    openapi.IssueFetchTokenResponseObject, error)

func (h *Handler) MarkSessionShipped(ctx context.Context,
    req openapi.MarkSessionShippedRequestObject) (
    openapi.MarkSessionShippedResponseObject, error)

// script.go
type ScriptInput struct {
    Mode             string          // "squash" or "preserve"
    TargetBranch     string
    BaseSHA          string
    SelectedSHAs     []string
    SquashMessageBody string         // empty in preserve mode
}
func BuildScript(in ScriptInput) string  // dispatches to squash/preserve

// message.go
type CoAuthor struct {
    Name      string
    Email     string
    AccountID string // empty when no portal account matches
}
func ComposeSquashMessage(sessionGoal, userOverrideSubject string,
    commits []*object.Commit) (subject, body string, coAuthors []CoAuthor)

// lock_check.go
const FinalizeLockTTL = 30 * time.Minute
func IsLockExpired(lastActivity, now time.Time) bool

// tokens/service.go (additions)
type Service interface {
    // ... existing methods
    IssueShortLived(ctx context.Context, accountID string,
        ttl time.Duration) (Pair, error)
}
```

Store-layer additions (sqlc-generated, declared in
`db/queries/{sqlite,postgres}/finalize_locks.sql`):

```sql
-- name: InsertFinalizeLock :one
-- name: GetFinalizeLockByID :one
-- name: GetActiveFinalizeLockForSession :one  -- WHERE released_at IS NULL AND superseded_by_lock_id IS NULL
-- name: UpdateFinalizeLockCuration :exec      -- sets selected_commit_shas, target_branch, base_sha, mode, commit_message, last_activity_at
-- name: TouchFinalizeLock :exec               -- sets last_activity_at = now
-- name: ReleaseFinalizeLock :exec             -- sets released_at
-- name: SupersedeFinalizeLock :exec           -- sets superseded_by_lock_id
```

### Wiring into the portal

`cmd/portal/main.go` gets a new collaborator:

```go
finalizeHandler := finalize.New(dbStore, storageSvc, eventLog,
    tokenSvc, cfg.PortalURL)
```

added to `combinedHandler` so the generated strict-server delegates
finalize methods to it. No new router-level mount — the methods live
under the existing `/api` route group and the same Bearer auth
middleware applies.

### Test plan

- **Unit:** `IsLockExpired` table-driven across boundary cases
  (exact-TTL, well-before, well-after, clock-skewed).
- **Unit:** `ComposeSquashMessage` golden file. Cases:
  (a) single author, (b) three distinct authors,
  (c) duplicate authors across emails differing only in case,
  (d) user-override subject + multi-line override (only first line
  is used), (e) session-goal subject truncation at 72 chars with
  word-boundary respect.
- **Unit:** `BuildScript` golden files for squash and preserve
  modes, including 1-commit, 3-commit, and 10-commit selections.
- **Integration (sqlite in-memory):**
  - Acquire: no-lock case, idempotent re-acquire, override happy
    path, override-rejected-on-fresh-lock, stale-lock auto-release-
    and-proceed.
  - PATCH: caller-only, idle-expired-409, happy update.
  - Release: idempotent, caller-only, clears sessions pointer.
  - Plan: missing-SHA-409, lock-expired-409, lock-superseded-409,
    happy squash, happy preserve, fetch_source.kind == "https".
  - Fetch-token: TTL is 5 min, token is valid against
    `tokens.Validate`, expires correctly.
  - Mark-shipped: idempotent, conflict if active, conflict if
    ended-with-other-reason, emits `session.ended` with reason
    `shipped`, updates `archived_sessions.final_branch_name` when
    body carries it.
- **Repo-fixture:** plan tests use a small bare-repo fixture
  built via go-git in TestMain with 3 commits, 2 authors; the
  squash-message golden is byte-stable across runs.

### Story decomposition

Three stories along the lock/plan/finalization boundaries. Story 2
and 3 both depend on story 1 (which owns the migration + lock
queries + lock CRUD endpoints + shared `tokens.IssueShortLived`).
Stories 2 and 3 run in parallel after 1.

1. **`epic-finalize-flow-plan-generation-locks-schema-and-rest`** —
   migration 00010 (`finalize_locks` table + sqlc queries),
   `finalize.Handler` skeleton, lock endpoints (acquire / patch /
   release), `IsLockExpired` helper + tests, `openapi.yaml`
   additions for the three lock endpoints + `FinalizeLock`,
   `LockStatus`, `AcquireFinalizeLockRequest`,
   `PatchFinalizeLockRequest`, `PlanMode` schemas. Also adds
   `tokens.IssueShortLived` (needed in story 3 but shared
   plumbing — bundling here keeps story 3 small).
   `depends_on: []`

2. **`epic-finalize-flow-plan-generation-plan-fetch-and-script`** —
   `GET /finalize-plan` endpoint, `script.go` (squash + preserve
   builders, golden tests), `message.go`
   (`ComposeSquashMessage` + golden test), `openapi.yaml` additions
   for `PlanResponse`, `PlanCommit`, `CoAuthor`, `FetchSource`. Uses
   the lock queries from story 1. First-parent linearization of
   the curated SHAs is *plugin-side / UI-side* selection; the
   server merely resolves whatever SHAs are in the lock — but the
   server provides a helper `FirstParentLeafCommits(repo, draftTip)
   []*object.Commit` that the UI calls separately to populate the
   default selection. This helper lives in `script.go` because
   it's plan-package internal.
   `depends_on: [epic-finalize-flow-plan-generation-locks-schema-and-rest]`

3. **`epic-finalize-flow-plan-generation-fetch-token-and-mark-shipped`** —
   `POST /finalize/fetch-token` endpoint, `POST /mark-shipped`
   endpoint, `openapi.yaml` additions (`FetchTokenResponse`,
   `MarkShippedRequest`, `SessionEndedPayload.reason` enum
   extension to include `shipped`), wiring `finalizeHandler` into
   `cmd/portal/main.go`'s `combinedHandler`. Depends on the
   lock-row queries (`GetActiveFinalizeLockForSession` — when
   marking shipped we release any held lock for cleanliness) from
   story 1.
   `depends_on: [epic-finalize-flow-plan-generation-locks-schema-and-rest]`

### Implementation order

1. Story 1 (migration + lock CRUD) — strictly first; both
   downstreams depend on the schema and the handler skeleton.
2. Stories 2 and 3 in parallel — independent file sets, different
   endpoints, share only the `Handler` receiver.

### Risks

- **`session.ended` payload `shipped` value.** The OpenAPI enum
  currently lists `[finalize, abandon, timeout]`. Story 3 extends
  it to `[finalize, abandon, timeout, shipped]`. This is a
  backwards-compatible additive enum change for producers and a
  breaking change for consumers that exhaustively switch on the
  enum — verified that no current consumer exhaustively switches.
- **First-parent walk through auto-merger merges.** The merge
  commits carry `Auto-Merger: true` trailers. The
  `FirstParentLeafCommits` helper walks `draft` first-parent;
  when it encounters a 2-parent commit with the `Auto-Merger`
  trailer, it follows the second parent's first-parent chain
  back to the merge-base with the first parent's previous
  position to enumerate the integrated leaves in chronological
  order. Implementation detail in story 2.
- **Co-author email dedup case-sensitivity.** GitHub treats
  emails case-insensitively for `Co-authored-by`. Composer
  lowercases for dedup-key but preserves the first-seen casing
  in the rendered trailer. Golden test (c) above pins this.
- **Override creates a new lock, old lock becomes ghost.** A
  PATCH on a ghosted lock returns `409 finalize.lock_superseded`
  with the new `lock_id` in `details.superseded_by_lock_id` so the
  client UI can offer "switch to the new lock?" recovery.


## Implementation summary

3 child stories landed (commits bdb803c, 1ea6400, 2bfb61d). New `internal/portal/finalize/` package owns the lock state machine, plan generation (squash + preserve script composers, golden-tested), the fetch-token endpoint reusing tokens.IssueShortLived, and the mark-shipped transition. 2 migrations (00010 + 00011). The full finalize-API surface is live on the portal.

## Review

**Verdict**: Approve. End-to-end backend capability complete.
