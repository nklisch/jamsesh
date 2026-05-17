---
id: epic-cc-plugin-hooks
kind: feature
stage: implementing
tags: [plugin]
parent: epic-cc-plugin
depends_on: [epic-cc-plugin-binary-foundation]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
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

## Design decisions

- **Package**: `cmd/jamsesh/hooks/` (subcommands under the existing binary).
- **Retry queue**: in `cmd/jamsesh/retryqueue/` — file-backed JSON FIFO at `${CLAUDE_PLUGIN_DATA}/sessions/<sessionID>/retry-queue.json`. Atomic via temp+rename. Locking: a `flock` (or sqlite-style busy retry) to coordinate concurrent enqueue+drain. For v1, the queue is touched only by post-tool-use (enqueue) and user-prompt-submit (drain); since CC fires hooks serially per session, no real concurrency. Skip locking; document.
- **Error classifier**: `cmd/jamsesh/pusherr/classify.go` — parses HTTP status + structured error body; returns `Transient | Permanent | OK`. Used by post-tool-use + stop.
- **Push retry policy**: 3 attempts with exponential backoff (250ms, 1s, 4s) on Transient; immediate fail on Permanent; final-fail enqueues.
- **Story decomposition**: 2 stories.
  1. `retry-queue-and-simple-hooks` — retry queue, classifier, `pre-tool-use`, `session-end`. depends_on: []
  2. `fetch-push-and-stop-hooks` — `session-start`, `user-prompt-submit`, `post-tool-use`, `stop`. depends_on: [retry-queue-and-simple-hooks]

## Implementation Units

### Unit 1: Retry queue

**File**: `cmd/jamsesh/retryqueue/queue.go`
**Story**: `epic-cc-plugin-hooks-retry-queue-and-simple-hooks`

```go
type Entry struct {
    CommitSHA   string    `json:"commit_sha"`
    Attempts    int       `json:"attempts"`
    LastErrorAt time.Time `json:"last_error_at"`
    LastError   string    `json:"last_error"`
}

type Queue struct {
    SessionID string
}

func (q *Queue) Path() string  // ${CLAUDE_PLUGIN_DATA}/sessions/<sid>/retry-queue.json
func (q *Queue) Load() ([]Entry, error)
func (q *Queue) Save(entries []Entry) error  // atomic via state.Write
func (q *Queue) Enqueue(e Entry) error
func (q *Queue) Drain() ([]Entry, error)  // returns and clears
func (q *Queue) Size() (int, error)
```

### Unit 2: Error classifier

**File**: `cmd/jamsesh/pusherr/classify.go`

```go
type Class int
const (
    OK Class = iota
    Transient
    Permanent
)

type Result struct {
    Class    Class
    Code     string  // error code from JSON envelope
    Message  string
    Details  map[string]any
}

func Classify(httpStatus int, body []byte) Result
```

Permanent codes: push.* + auth.invalid_token + auth.insufficient_permission.
Transient: 5xx OR network error before HTTP response.

### Unit 3: pre-tool-use

**File**: `cmd/jamsesh/hooks/pretooluse.go`
**Story**: `epic-cc-plugin-hooks-retry-queue-and-simple-hooks`

Input: `{tool_name, tool_input}` where tool_input is JSON containing the command for Bash.

Logic:
- If tool_name == "Bash" AND tool_input.command starts with "git push" → return `{"permissionDecision": "deny", "reason": "jamsesh: push is gated; commits push automatically via post-tool-use"}`
- If tool_name == "Bash" AND command matches `git config remote.*` → deny similarly
- Else: `{"permissionDecision": "pass"}` (let CC's default behavior continue)

### Unit 4: session-end

**File**: `cmd/jamsesh/hooks/sessionend.go`

Simple: optionally POST a presence-offline event. v1 no-op (returns empty additionalContext). Documented.

### Unit 5: session-start

**File**: `cmd/jamsesh/hooks/sessionstart.go`
**Story**: `epic-cc-plugin-hooks-fetch-push-and-stop-hooks`

Read CC's SessionStart input (CWD + session_id from env or state file). Call portal:
- `GET /api/sessions/<id>` → session metadata
- `GET /api/sessions/<id>/refs` → ref tips
- `GET /api/sessions/<id>/comments?addressed_to=@<user>&resolved=false` → addressed comments

Format as `additionalContext` text block.

### Unit 6: user-prompt-submit

**File**: `cmd/jamsesh/hooks/userpromptsubmit.go`

Steps:
1. `git fetch` from session remote (via os/exec)
2. Drain retry queue: for each entry, call `git push` to that commit; if Transient, re-enqueue; if Permanent, log and drop (the commit's bad)
3. GET `/api/sessions/<id>/digest?since=<lastSeq>` from portal
4. Format both as `additionalContext`
5. Save updated lastSeq to local state

### Unit 7: post-tool-use

**File**: `cmd/jamsesh/hooks/posttooluse.go`

Input: `{tool_name, tool_input, tool_response}`. Only react to Bash with `git commit` that succeeded (exit 0).

Logic:
1. Determine current commit SHA: `git rev-parse HEAD`
2. Attempt `git push origin <ref>` with retry policy (3 transient retries)
3. On success: return empty (the agent doesn't need to know)
4. On all-transient-fail: enqueue commit; return `additionalContext` warning "push queued for retry"
5. On permanent fail: return `additionalContext` with full error details for the agent to act on

### Unit 8: stop

**File**: `cmd/jamsesh/hooks/stop.go`

Steps:
1. Check working tree dirty: `git status --porcelain`
2. If dirty: `git add -A && git commit -m "<summary> [jamsesh auto-commit at turn end]" --trailer "Jam-Auto-Commit: true"`
3. Final push (same retry policy)
4. Check retry queue size; if > 10: refuse via stderr error "session wedged"
5. POST `/api/sessions/<id>/turn.ended` via portal client

## Story decomposition

- `retry-queue-and-simple-hooks` — Units 1-4 + tests
- `fetch-push-and-stop-hooks` — Units 5-8 + tests. depends_on: previous

## Testing

- Retry queue round-trip + size + drain
- Classifier table tests (each error code)
- pre-tool-use: deny git push; pass everything else
- session-start: mock portal returns canned data; verify additionalContext format
- user-prompt-submit: mock portal + git fetch stub; verify queue drain + digest assemble
- post-tool-use: mock portal returning transient → retry; permanent → fail-loud
- stop: dirty tree → auto-commit; queue too large → refuse

## Risks

- **Hook output JSON format**: each hook returns CC-specific JSON (additionalContext / permissionDecision). The `cmd/jamsesh/hookio` scaffold from binary-foundation handles the IO; hooks return typed output structs.
