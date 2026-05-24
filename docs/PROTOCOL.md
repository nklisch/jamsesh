# Protocol

The contracts between jamsesh components.

## MCP tools (portal-hosted, HTTPS-MCP)

Four tools, all thin proxies to portal API endpoints. Every tool call carries
`session_id` so the portal applies session-scoped authorization.

### `post_comment`

Post a comment on a commit, file, or line range, with optional addressing.

**Parameters:**
- `session_id` (string, required)
- `commit_sha` (string, required) — the commit being commented on
- `file_path` (string, optional) — file within the commit's tree
- `line_range` (object, optional) — `{start: int, end: int}` 1-indexed
- `body` (string, required) — the comment text (markdown)
- `addressed_to` (string, optional) — `@<user>`, `@<user>/<branch>`,
  `@all-agents`, `@all-humans`, `@everyone`, `@auto-merger`. Omitted = fyi to
  the session at large.
- `kind` (string, optional) — one of `question`, `suggestion`,
  `action-request`, `fyi`. Defaults to `fyi`.

**Returns:** `{id: string, created_at: string}`

### `resolve_comment`

Mark a comment resolved.

**Parameters:**
- `session_id` (string, required)
- `comment_id` (string, required)
- `resolution_note` (string, optional)

**Returns:** `{resolved_at: string}`

### `fork`

Server-side ref manipulation. Creates or moves a ref under the user's
namespace, parented at the specified commit. Required because pre-receive
forbids force-pushes on shared refs and the portal can authorize this on
behalf of the user with its own privileged write access to the session repo.

**Parameters:**
- `session_id` (string, required)
- `target_commit_sha` (string, required) — parent for the new/moved ref
- `target_ref` (string, optional) — branch name under `<user>/`. If absent,
  moves the user's currently-bound ref. If present, creates or overwrites
  `jam/<session>/<user>/<target_ref>`.
- `mode` (string, optional) — `sync` or `isolated`. Defaults to the session's
  default mode.

**Returns:** `{ref: string, sha: string}`

### `query_session_state`

Escape hatch for on-demand queries when the digest didn't carry what the
agent needs. Returns a flexible payload filtered by the supplied criteria.

**Parameters:**
- `session_id` (string, required)
- `include` (array of strings, optional) — any of `presence`, `goal`,
  `scope`, `members`, `refs`, `draft_tip`, `unresolved_comments`,
  `open_conflicts`, `recent_events`. If absent, returns a default summary set.
- `filter` (object, optional) — narrows results. E.g.,
  `{comments_addressed_to: "@<user>/<branch>"}`,
  `{events_since_seq: 1234}`.

**Returns:** an object keyed by the requested `include` items.

## REST API

The portal's REST API. All routes are HTTPS, Bearer-auth via user OAuth
token (or org admin token for management endpoints). Routes are org-scoped
implicitly via the token; session-scoped routes take `session_id` in the path.

**Authoritative spec**: `docs/openapi.yaml` is the canonical OpenAPI 3.0.3
description of every route below. The route catalog in this document is a
human-readable summary; the YAML carries the precise request/response
schemas, error codes, and parameter validation. Go server stubs are
generated via `oapi-codegen`; TypeScript client types via
`openapi-typescript`. Drift between the YAML and either side is a build-
time error. The WebSocket envelope and event payload schemas
(`components/schemas/EventEnvelope`, `Comment`, `ConflictEvent`, etc.) live
in the same YAML so Go and TypeScript share types across REST and
WebSocket.

### Auth

- `POST /api/auth/oauth/start` — initiate OAuth flow, returns redirect URL
- `POST /api/auth/oauth/callback` — OAuth callback, exchanges code for token
- `POST /api/auth/magic-link/request` — request magic link (alternative auth)
- `POST /api/auth/magic-link/exchange` — exchange magic link for token
- `POST /api/auth/revoke` — revoke current token

### Orgs & accounts

- `GET /api/me` — current account info
- `GET /api/orgs/<org_id>/members` — list members (admin)
- `POST /api/orgs/<org_id>/invites` — invite a member (admin)

### Sessions

- `POST /api/orgs/{orgID}/sessions` — create a session; body includes name, goal, scope,
  default_mode
- `GET /api/orgs/{orgID}/sessions` — list sessions visible to the user (active + recent)
- `GET /api/orgs/{orgID}/sessions/{sessionID}` — session metadata
- `PATCH /api/orgs/{orgID}/sessions/{sessionID}` — update goal, scope (widen only), default_mode
- `POST /api/orgs/{orgID}/sessions/{sessionID}/finalize` — mark session as finalizing, acquire
  a finalize lock for curation (see finalize-plan endpoint for the plan
  body, which is squash-by-default with a preserve-all opt-in)
- `POST /api/orgs/{orgID}/sessions/{sessionID}/abandon` — close session without finalize
- `POST /api/orgs/{orgID}/sessions/{sessionID}/invites` — invite participants
- `GET /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}` — get a specific pending invite
- `POST /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept` — accept a session invite
- `POST /api/orgs/{orgID}/sessions/{sessionID}/members/{accountID}/remove` — remove a member
- `POST /api/orgs/{orgID}/sessions/{sessionID}/mark-shipped` — mark a finalizing session as shipped (transitions to ended with reason "shipped")

### Comments

- `GET /api/orgs/{orgID}/sessions/{sessionID}/comments` — list comments in a session, cursor-paginated with optional filters
- `POST /api/orgs/{orgID}/sessions/{sessionID}/comments` — post a comment on a commit, file, or line range
- `POST /api/orgs/{orgID}/sessions/{sessionID}/comments/{commentId}/resolve` — mark a comment resolved

### Session state (used by the local binary)

- `GET /api/orgs/{orgID}/sessions/{sessionID}/digest?since=<seq>` — formatted digest for the next
  turn. Returns text suitable for `additionalContext` injection.
- `GET /api/orgs/{orgID}/sessions/{sessionID}/refs` — all refs in the session with mode and tip
- `GET /api/orgs/{orgID}/sessions/{sessionID}/files` — list files in the session's draft tree
- `POST /api/orgs/{orgID}/sessions/{sessionID}/ref-modes` — change the ref mode for a session member's ref
- `GET /api/orgs/{orgID}/sessions/{sessionID}/finalize-plan` — the finalize plan: mode-aware
  shell script body (squash via `cherry-pick --no-commit` + composed
  commit, or per-commit `cherry-pick` in preserve mode), plain-English
  summary, composed commit message + `Co-authored-by` list (squash mode),
  lock status, and HTTPS-fallback fetch source. See the OpenAPI YAML for
  the precise response schema.

### Finalize machinery

- `POST /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock` — acquire the finalize lock for curation (idempotent for the caller; 409 if another holds a fresh lock)
- `PATCH /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}` — update curation state on a held lock (selected commits, target branch, commit message)
- `DELETE /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}` — release the finalize lock
- `POST /api/orgs/{orgID}/sessions/{sessionID}/finalize/fetch-token` — obtain an HTTPS fetch token for pulling the session repo during finalize

### Playground

Anonymous-friendly endpoints for the ephemeral playground org. Schemas live
in `docs/openapi.yaml` under `/api/playground/sessions*`.

- `POST /api/playground/sessions` — create a new ephemeral session in the reserved playground org; mints the creator's anonymous bearer in the response (no auth required; rate-limited per source IP)
- `POST /api/playground/sessions/{id}/join` — join an existing playground session; mints a fresh anonymous bearer scoped to that session; 409 `playground.session_full` if at `MaxParticipants` cap (response includes `retry_after_seconds`); 410 `playground.session_ended` if past `hard_cap_at`
- `GET /api/playground/sessions/{id}` — fetch playground session metadata (countdown timestamps, participant nicknames); requires a valid anonymous bearer for this session
- `GET /api/playground/sessions/{id}/tombstone` — fetch the post-destruction tombstone view (reason, ended_at); no auth required

### Git smart-HTTP (separate path tree)

- `POST /git/<org_id>/<session_id>.git/git-receive-pack` (push)
- `POST /git/<org_id>/<session_id>.git/git-upload-pack` (fetch)
- `GET  /git/<org_id>/<session_id>.git/info/refs` (capability advertisement)

HTTP Basic auth; password is the user OAuth token (username can be anything
or `x-access-token`).

## Lifecycle hook contracts (portable runtime contract)

Four lifecycle touchpoints, documented as a portable contract so non-CC agent
runtimes can implement against a spec. The Claude Code plugin implements
these via CC's hook system; a Cursor or Cline runtime would implement them
via their equivalent.

### `session_bootstrap`

Equivalent to CC's `SessionStart`. Fires once at agent-runtime startup if a
jamsesh session is active.

**Responsibilities:**
- Verify the session remote is configured and the user's ref is checked out
- Inject context describing: session goal, writable scope, current draft tip,
  peer ref tips, the user's refs and modes, unresolved comments addressed to
  this agent

**Outputs (returned to runtime):**
- `additionalContext` (string) — opening context for the agent

### `pre_turn_digest`

Equivalent to CC's `UserPromptSubmit` (or `UserPromptExpansion`; whichever
fires before context is finalized). Fires before each agent turn.

**Responsibilities:**
- `git fetch` from the session remote
- Call portal digest API
- Format combined output as text

**Outputs:**
- `additionalContext` (string) — "since you last spoke" block

The digest response carries an optional `urgent_events` array for
time-sensitive events the binary renders in a prominent "Urgent" section
above the regular digest text. Currently `playground.destruction_warning`
is the sole member of that class — when a playground session is approaching
its idle-timeout or hard-cap deadline, the warning surfaces here so the
agent's next reply can acknowledge or react before destruction lands.

### `push_gate`

Equivalent to CC's `PreToolUse` filtered to Bash. Fires before each tool call
the agent attempts.

**Responsibilities:**
- Detect attempts to run `git push` directly → deny
- Detect attempts to modify `git config remote.*` → deny
- Allow everything else

**Outputs:**
- `permissionDecision: "deny" | "allow" | "pass"`
- `reason` (string, optional)

### `commit_observed`

Equivalent to CC's `PostToolUse` filtered to successful `git commit` calls.
Fires after each tool call.

**Responsibilities:**
- Detect that a `git commit` just succeeded
- Push the user's ref to the session remote

**Outputs:** none (side-effecting only).

### `turn_end`

Equivalent to CC's `Stop`. Fires when the agent yields control back to the
human.

**Responsibilities:**
- Auto-commit any dirty working tree with a generic message
- Push the user's ref one more time
- POST `turn.ended` event to the portal

**Outputs:** none.

### `session_end`

Equivalent to CC's `SessionEnd`. Fires when the agent runtime exits.

**Responsibilities:**
- Clean up in-memory caches
- Optionally post presence-offline to the portal

**Outputs:** none.

## Commit trailer conventions

All session commits carry structured trailers. The pre-receive hook enforces
presence of the required ones on every collaborative push — the only
exemption is the inaugural push to a session's base ref (see "Base-ref
bootstrap exemption" below).

**Required on every session commit:**

```
Jam-Session: <session-id>
Jam-Turn: <turn-number>
Jam-Author: <user-id-or-handle>
```

**Base-ref bootstrap exemption.** The very first push to
`refs/heads/jam/<session-id>/base` (with `OldSHA` empty — i.e. creating
the ref rather than updating it) is exempted from per-commit trailer and
scope validation. The seed commits a user pushes here are their
pre-session working-tree commits — they predate the session existing and
so cannot carry session-aware trailers naming this session. Subsequent
updates to the base ref (rare; usually force-push for reseeding, which
is independently rejected by the force-push check) and every push to
non-base refs still go through full per-commit validation.

**Optional, recognized by the system:**

```
Resolves-Conflict: <conflict-event-id>
   - Tells the auto-merger this commit is a proposed resolution. When the
     merge succeeds, the conflict event is marked resolved automatically.

Auto-Merger: true
   - Set on commits the auto-merger creates. Not human-meaningful but useful
     for filtering tree views.

Source-Commit: <sha>
   - On auto-merger merge commits, names the source commit being merged.
```

## Comment schema

Comments are first-class entities in the portal database.

```
{
  "id": "<uuid>",
  "session_id": "<session-id>",
  "author_id": "<account-id>",
  "author_kind": "human" | "agent",     // agent comments are MCP-posted
  "anchor": {
    "commit_sha": "<sha>",
    "file_path": "<path>",              // optional
    "line_range": {start: 1, end: 5}    // optional
  },
  "body": "<markdown>",
  "addressed_to": "@<recipient>",        // optional
  "kind": "fyi" | "question" | "suggestion" | "action-request",
  "created_at": "<iso-8601>",
  "resolved_at": "<iso-8601 | null>",
  "resolved_by": "<account-id | null>",
  "resolution_note": "<string | null>"
}
```

**Addressing syntax** for `addressed_to`:

- `@<user-handle>` — addressed to that human (and they may have their agents
  read it)
- `@<user-handle>/<branch>` — addressed to that specific agent instance
- `@all-humans` — broadcast to humans
- `@all-agents` — broadcast to agents
- `@everyone` — broadcast to everyone in the session
- `@auto-merger` — addressed to the auto-merger (informational; the
  auto-merger does not act on comments)

Anonymous session participants use the same `@<nickname>` form as durable
accounts. The addressing layer does not distinguish identity kind — it looks
up by `accounts.display_name` for the session, which works identically for
anonymous and durable accounts. A comment addressed to `@amber-otter` reaches
the anonymous participant with that display name the same way it reaches a
durable account holder.

## Conflict event schema

Emitted by the auto-merger when a three-way merge fails.

```
{
  "id": "<uuid>",
  "session_id": "<session-id>",
  "source_commit_sha": "<sha>",         // the commit that couldn't merge
  "source_ref": "jam/<session>/<user>/<branch>",
  "draft_tip_sha": "<sha>",             // draft tip at time of attempt
  "ancestor_sha": "<sha>",              // common ancestor
  "conflicts": [
    {
      "file": "<path>",
      "ranges": [
        {start: 12, end: 24}
      ]
    }
  ],
  "addressed_to": [
    "@<user>/<branch>",                 // source ref's owner
    "@<other-user>/<other-branch>"      // owner of the conflicting draft commit
  ],
  "status": "open" | "resolved" | "abandoned",
  "resolving_commit_sha": "<sha | null>",  // set when Resolves-Conflict trailer matches
  "resolved_at": "<iso-8601 | null>",
  "created_at": "<iso-8601>"
}
```

Conflict events appear in agent digests for the addressed users.

## WebSocket event types

The portal pushes events to subscribed UI clients over WebSocket. The upgrade
request at `/ws/sessions/{sessionID}` is authenticated via a short-lived
single-use ticket presented in the `Sec-WebSocket-Protocol` header as
`jamsesh-ticket.<ticket>`. The SPA obtains the ticket by calling
`POST /api/auth/ws-ticket` (authenticated with the regular bearer) immediately
before opening the socket. The ticket is valid for 60 seconds and is consumed
atomically on upgrade — it cannot be replayed. Because the long-lived bearer
token is never placed in `Sec-WebSocket-Protocol`, proxy log exposure of a
long-lived credential is not a concern.

All events share a common envelope:

```
{
  "seq": <int>,                         // monotonic per session
  "version": 1,                         // envelope-schema version, always 1
  "session_id": "<session-id>",
  "type": "<event-type>",
  "payload": { ... },
  "timestamp": "<iso-8601>"
}
```

`version` is the envelope-schema version (currently always `1`); it allows future envelope format changes to be signalled without breaking existing clients.

**Event types:**

- `commit.arrived` — payload: `{ref, sha, author_id, summary}` (schema: [CommitArrivedPayload](./openapi.yaml#/components/schemas/CommitArrivedPayload))
- `merge.succeeded` — payload: `{source_sha, draft_sha, merge_commit_sha}` (schema: [MergeSucceededPayload](./openapi.yaml#/components/schemas/MergeSucceededPayload))
- `conflict.detected` — payload: full conflict event (schema: [ConflictDetectedPayload](./openapi.yaml#/components/schemas/ConflictDetectedPayload))
- `conflict.resolved` — payload: `{event_id, resolving_commit_sha}` (schema: [ConflictResolvedPayload](./openapi.yaml#/components/schemas/ConflictResolvedPayload))
- `comment.added` — payload: full comment (schema: [CommentAddedPayload](./openapi.yaml#/components/schemas/CommentAddedPayload))
- `comment.resolved` — payload: `{comment_id, resolved_by, note}` (schema: [CommentResolvedPayload](./openapi.yaml#/components/schemas/CommentResolvedPayload))
- `ref.forked` — payload: `{ref, parent_sha, mode}` (schema: [RefForkedPayload](./openapi.yaml#/components/schemas/RefForkedPayload))
- `mode.changed` — payload: `{ref, old_mode, new_mode}` (schema: [ModeChangedPayload](./openapi.yaml#/components/schemas/ModeChangedPayload))
- `turn.ended` — payload: `{user_id, ref, final_sha}` (schema: [TurnEndedPayload](./openapi.yaml#/components/schemas/TurnEndedPayload))
- `presence.updated` — payload: `{user_id, ref, current_sha, last_active}` (schema: [PresenceUpdatedPayload](./openapi.yaml#/components/schemas/PresenceUpdatedPayload))
- `session.finalizing` — payload: `{by_user_id}` (schema: [SessionFinalizingPayload](./openapi.yaml#/components/schemas/SessionFinalizingPayload))
- `session.ended` — payload: `{reason: "finalize" | "abandon" | "timeout"}` (schema: [SessionEndedPayload](./openapi.yaml#/components/schemas/SessionEndedPayload))
- `playground.destruction_warning` — payload: `{reason: "idle_timeout" | "hard_cap", ends_at, remaining_seconds, session_id}` (schema: [PlaygroundDestructionWarningPayload](./openapi.yaml#/components/schemas/PlaygroundDestructionWarningPayload))
- `auto-merger.backpressure` — payload: `{session_id, dropped_ref}` (schema: [AutoMergerBackpressurePayload](./openapi.yaml#/components/schemas/AutoMergerBackpressurePayload))
- `session.created` — payload: full session created event (schema: [SessionCreatedPayload](./openapi.yaml#/components/schemas/SessionCreatedPayload))

## Local state schema (`${CLAUDE_PLUGIN_DATA}/`)

The local binary's state on disk.

```
${CLAUDE_PLUGIN_DATA}/
├── token                 root-level account OAuth token (mode 0600);
│                         a `MIGRATED_TO_PER_SESSION` stub for accounts
│                         whose per-session tokens have superseded it
├── refresh_token         OAuth refresh token (mode 0600)
├── portal_url            configured portal URL (one line)
└── sessions/
    └── <session-id>/
        ├── token         per-session bearer (mode 0600) — durable session
        │                 bearer or playground anonymous bearer
        ├── instance_id   the CC session_id this binding belongs to
        ├── ref           the (user/branch) this CC instance is bound to
        ├── org_id        org the session belongs to
        ├── account_id    account on whose behalf this binding acts
        └── last_seen_seq portal event log cursor
```

## HTTP error contract

Portal API errors return JSON:

```
{
  "error": "<machine-readable code>",
  "message": "<human-readable message>",
  "details": { ... }                    // optional, error-specific
}
```

Common error codes:
- `auth.invalid_token`
- `auth.expired_token`
- `auth.insufficient_permission`
- `session.not_found`
- `session.not_member`
- `session.ended`
- `session.invalid_writable_scope` (400; `writable_scope` glob failed validation at session-create or session-patch time; emitted by durable and playground session-create paths and durable session-patch)
- `session.scope_narrowing_rejected` (400; `writable_scope` patch removed a pattern present on the existing session)
- `push.scope_violation` (with `details.paths` listing offenders)
- `push.ref_namespace_violation`
- `push.missing_trailer` (with `details.missing` listing absent trailers)
- `fork.target_not_found`
- `fork.invalid_target_ref`
- `org.protected` (409; `PATCH /api/orgs/{orgID}` rejected because the target org carries `org_protected=true`; the playground org is the only protected org by default)
- `playground.session_full` (409; playground `join` rejected because the session is at the `MaxParticipants` cap; response body includes `retry_after_seconds`)
- `playground.session_ended` (410; playground `join` rejected because the session's `hard_cap_at` has elapsed or the session is no longer active)
- `playground.size_exceeded` (pre-receive; playground push rejected because the session's accumulated repo size would exceed the configured content cap; `details` includes `current_bytes`, `pack_bytes`, `total_bytes`, `max_bytes`, `session_id`)
- `oauth.invalid_grant` (400; the OAuth provider rejected the authorization code at the business layer — RFC 6749 `invalid_grant` / GitHub `bad_verification_code`. The code is expired, reused, or malformed; the user must re-initiate sign-in. Distinct from `dep.oauth_provider_unavailable` (503), which signals a transport-level outage where a retry may succeed.)

### Dependency-failure codes

The `dep.*` family signals that a runtime dependency the portal needs to
serve the request is unavailable. The request itself is well-formed; the
caller should retry after a brief delay, except where noted. Every `dep.*`
response except `dep.git_subprocess_failed` carries a `Retry-After`
header with a coarse retry hint in seconds.

| Code                              | Status | Retry-After | Meaning                                                                 |
|-----------------------------------|--------|-------------|-------------------------------------------------------------------------|
| `dep.smtp_unavailable`            | 503    | `5`         | Outbound email delivery (magic link, invite) failed at the transport.   |
| `dep.db_unavailable`              | 503    | `2`         | A database query failed for a non-business reason (connection refused, timeout, I/O error). `store.ErrNotFound` and `store.ErrUniqueViolation` continue to surface as their existing 404 / 409 codes. |
| `dep.oauth_provider_unavailable`  | 503    | `10`        | Outbound HTTP to an OAuth provider (e.g. GitHub) failed (non-2xx response, transport error). Distinct from `oauth.provider_not_configured` (503 startup-time config gap). |
| `dep.git_subprocess_failed`       | 500    | —           | The local `git-upload-pack` / `git-receive-pack` / `git http-backend` subprocess failed (spawn error, non-zero exit). Not transient — operator intervention is typically required. |

A 503 from this family communicates retryability at the transport level;
the `error` code disambiguates which specific dependency is down so the
SPA / plugin can surface a targeted message. The portal logs the
underlying cause (pg connection error, SMTP handshake failure, etc.) at
error level; the response body never includes internal trace detail.
