---
id: epic-cc-plugin-hooks
kind: feature
stage: drafting
tags: [plugin]
parent: epic-cc-plugin
depends_on: [epic-cc-plugin-binary-foundation]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# CC Plugin — Lifecycle Hooks

## Brief

The six CC lifecycle-hook subcommands plus the cross-hook retry queue
that lets push-per-commit recover from transient failures without
losing the agent's flow.

**Subcommands delivered**:

- `jamsesh hook session-start` — fires once at CC SessionStart. Emits
  `additionalContext` describing: session goal, writable scope,
  current draft tip, peer ref tips, the user's refs and their modes,
  unresolved addressed comments. Reads from portal REST
  (`GET /api/sessions/<id>` + `GET /api/sessions/<id>/refs` +
  `GET /api/sessions/<id>/comments?addressed_to=@<user>&resolved=false`)
  + local git.

- `jamsesh hook user-prompt-submit` — fires before each agent turn.
  Performs:
  1. `git fetch` from the session remote (refreshes peer refs + draft
     tip).
  2. Drain the retry queue: any commits queued from prior failed
     pushes, attempt to push them now (FIFO, parent-before-child if
     interrelated).
  3. Call `GET /api/sessions/<id>/digest?since=<seq>` for the portal
     digest.
  4. Format combined output as `additionalContext`: peer commit
     activity from git log, social digest from portal, current state
     (goal, draft tip, your refs and modes, open conflicts).
  5. Advance local `last_seen` cursors.

- `jamsesh hook pre-tool-use` — gates Bash invocations. Returns
  `permissionDecision: "deny"` for `git push` (any form) and
  `git config remote.*`. Returns `permissionDecision: "pass"` for
  everything else (CC's other PreToolUse hooks may still run).

- `jamsesh hook post-tool-use` — fires after each Bash call. Detects
  successful `git commit`, runs `git push` to the session remote.
  Hybrid retry policy (locked at epic-design):
  - **Transient errors** (network unreachable, 5xx, timeouts): retry
    up to 3 times with exponential backoff (250ms, 1s, 4s). If all
    three fail, surface "push queued for retry — last error: <message>"
    to the agent and enqueue the commit in the per-session retry queue
    (drained on the next `user-prompt-submit`).
  - **Permanent errors** (4xx with structured error code:
    `push.scope_violation`, `push.ref_namespace_violation`,
    `push.missing_trailer`, `push.size_limit`,
    `push.force_push_rejected`, `auth.invalid_token`): fail loud
    immediately. Surface the full rejection payload (paths /
    trailers / refs) so the agent can react (e.g., scope violation →
    agent can revert the offending change).

- `jamsesh hook stop` — fires when the agent yields to the human.
  Performs:
  1. If the working tree is dirty: auto-commit the remainder with
     message `"<turn summary> [jamsesh auto-commit at turn end]"` +
     `Jam-Auto-Commit: true` trailer. `<turn summary>` is the first
     line of the last user prompt, truncated to N chars, or "WIP" if
     unavailable.
  2. Final `git push` (with the same retry policy).
  3. Check retry-queue size: if > 10 queued commits, refuse and emit
     a "session is wedged, run `jamsesh status` to investigate"
     error so the user notices.
  4. POST `turn.ended` to the portal REST.

- `jamsesh hook session-end` — fires on CC SessionEnd. Clears
  in-memory caches; optionally posts a presence-offline event to the
  portal. Does NOT delete local state — session state survives across
  CC sessions.

**Per-session retry queue**: shared component used by `post-tool-use`
(enqueue on transient failure) and `user-prompt-submit` (drain
before digest). Stored under
`${CLAUDE_PLUGIN_DATA}/sessions/<session-id>/retry-queue.json` as an
ordered list of `{commit_sha, attempts, last_error_at}` entries. FIFO;
if a queued push has a parent commit that's also queued, parent goes
first.

**Transient vs permanent classification**: based on HTTP status +
structured error code in the response body. The standard error
contract from `docs/PROTOCOL.md > HTTP error contract` makes
classification mechanical.

Does NOT include `jamsesh auth`, `mcp-headers`, or the local state
package (`binary-foundation`). Does NOT include `join`, `status`,
`fork`, `mode` (`session-commands`).

## Epic context

- Parent epic: `epic-cc-plugin`
- Position in epic: parallel with `session-commands`; both consume
  `binary-foundation`'s state package, portal API client, and JSON
  IO scaffold.

## Foundation references

- `docs/ARCHITECTURE.md` — Hook subcommands, Data flow: a turn
  (the canonical step-by-step hook firing sequence)
- `docs/PROTOCOL.md` — Lifecycle hook contracts (the portable contract
  this implementation satisfies), HTTP error contract (the error codes
  classified as transient vs permanent)
- `docs/UX.md` — Flow: an agent turn
- `docs/SPEC.md` — Local client (hook subcommands)

## Inherited epic design decisions

- **Push-failure retry policy**: hybrid — 3 retries with exponential
  backoff for transient; fail-loud immediately for permanent.
  Permanent classification via structured error code.
- **Retry queue**: per-session FIFO, parent-before-child if both
  queued, max 10 queued before `stop` refuses.
- **Auto-commit on turn end**: generic message with turn-summary prefix
  + `Jam-Auto-Commit: true` trailer.
- **Push-gate denial**: `git push` and `git config remote.*` always
  denied; everything else passes.

## Decomposition risks

- This feature is at the size ceiling (12-15 implementation units). If
  retry-queue interactions surface complexity (parent-before-child
  ordering algorithm, queue persistence under crashes, queue-too-large
  handling), the design pass may split out a `retry-queue` feature.
  Capacity reserved.
- The auto-commit logic in `stop` interacts with `pre-tool-use`'s
  denial: the auto-commit must complete its own push without being
  denied. Design pass locks the carve-out (the hook subcommand is
  running in-process, not via Bash, so push-gate doesn't apply).

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
