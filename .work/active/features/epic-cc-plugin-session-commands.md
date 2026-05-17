---
id: epic-cc-plugin-session-commands
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

# CC Plugin — Session Commands

## Brief

The user-facing slash-command implementations: `jamsesh join`,
`jamsesh status`, `jamsesh fork`, `jamsesh mode`. Each is a binary
subcommand invoked by the corresponding `skills/<name>/SKILL.md` slash
command (the SKILL.md instructs Claude to run `jamsesh <name>
$ARGUMENTS`).

**Subcommands delivered**:

- `jamsesh join <session-id-or-url> [--as <branch>] [--from <commit>]`
  — the orchestrated join flow:
  1. If not authenticated, prompt the user to run `jamsesh auth` first
     (don't auto-trigger — the agent shouldn't surprise the user with
     a browser open).
  2. Resolve the session via portal REST (`GET /api/sessions/<id>`);
     if the URL is an invite URL, accept the invite first
     (`POST /api/sessions/<id>/invites/<invite_id>/accept`).
  3. Clone the session bare repo into the current working tree's
     parent (or a chosen path), checkout
     `jam/<session>/<user>/<branch>` (defaults to `main`, creating from
     base or `--from` parent if needed).
  4. Configure the session remote (`session-remote`), install a
     local post-commit shell hook that calls `jamsesh hook post-commit`
     (note: actual push-per-commit happens via CC's PostToolUse, not
     this hook — the post-commit hook is for non-CC pushes; design
     pass decides whether we need it at all in v1).
  5. Write per-session state under
     `${CLAUDE_PLUGIN_DATA}/sessions/<session-id>/`: `ref`,
     `instance_id` (the CC `session_id`), starting cursors.
  6. Print a join summary (session name, goal, scope, your ref, mode).

- `jamsesh status [--json]` — prints session summary: tree summary
  (your refs + peers' tips + draft tip), scope, your current mode,
  unresolved comments addressed to this user, open conflicts addressed
  to this user. Human-readable text by default; `--json` for scripted
  consumption.

- `jamsesh fork <commit-sha> [--as <branch>] [--mode sync|isolated]`
  — wraps the MCP `fork` tool. Calls the portal MCP tool with the
  authenticated account; on success, fetches the new ref locally and
  optionally checks it out (if it replaces the user's current ref,
  reset the working tree; if a sibling, just fetch).

- `jamsesh mode sync|isolated` — flips the current bound ref's mode
  via portal REST or MCP (design pass picks the surface — likely
  REST under `/api/sessions/<id>/refs/<ref>/mode` or the MCP `fork`
  tool with mode-only). Updates local state's cached mode. Emits a
  `mode.changed` event via the portal side; this command just
  triggers it and updates local cache.

**Common patterns** (factored across these subcommands):

- Each command resolves the "current session" from CC's `session_id`
  via `${CLAUDE_PLUGIN_DATA}/sessions/`. The `instance_id` field in
  the per-session state maps CC instance to jamsesh session.
- All commands fail loud on errors (no silent swallowing) — failure
  shows up as Bash output the agent can react to.

Does NOT include any hook implementations. Does NOT include `jamsesh
finalize` — that's in `epic-finalize-flow`.

## Epic context

- Parent epic: `epic-cc-plugin`
- Position in epic: parallel with `hooks`; both consume
  `binary-foundation`'s state package + portal API client.

## Foundation references

- `docs/ARCHITECTURE.md` — The `jamsesh` binary > Slash command
  subcommands, Multi-agent per human
- `docs/UX.md` — Flow: joining a session, Flow: forking from a peer,
  Flow: switching mode, Status awareness in CC
- `docs/PROTOCOL.md` — MCP tools (`fork` parameter shape consumed by
  `jamsesh fork`), REST API > Sessions

## Inherited epic design decisions

- **`jamsesh status` output**: human-readable by default; `--json`
  flag for scripting.

## Design decisions

- **Package**: `cmd/jamsesh/sessioncmd/` with one subcommand per command.
- **REST + MCP split**: `status` uses REST (`/digest` + `/refs`); `join` uses REST (`/api/sessions/<id>` + clone via git); `fork` uses the MCP `fork` tool; `mode` uses REST (`POST /api/sessions/<id>/refs/<ref>/mode`) — but that endpoint doesn't exist. Design v1: `mode` calls MCP `fork` with the same ref + new mode (the fork tool's mode-only path is the API).

Wait — the fork tool creates a NEW ref. For mode-flip on existing ref, we need a different endpoint. Adding it would expand sessions-rest. Simpler: ship a new REST endpoint as part of THIS feature: `POST /api/sessions/<id>/refs/<ref>/mode`. Or use a portal-side helper exposed from sessions.Service.

For v1 simplicity: `mode` calls a new REST endpoint `POST /api/sessions/<sessionID>/ref-modes` that this feature ships in `internal/portal/sessions/`. That's portal-side work. To avoid sprawling this feature, accept the limitation and just update local state for now — server-side mode change can come in a follow-up.

Actually — the `ref_modes` table already exists from sessions-rest. The portal can expose `POST /api/sessions/<id>/ref-modes` with body `{ref, mode}`. Let me add that as a sub-unit of this feature on the portal side too. Keeps things scoped.

- **Story decomposition**: 2 stories.
  1. `join-and-status` — `jamsesh join` (REST + git clone) + `jamsesh status` (REST aggregation). depends_on: []
  2. `fork-and-mode` — `jamsesh fork` (MCP call) + `jamsesh mode` (calls portal REST + emits `mode.changed`) + portal-side `POST /api/sessions/<id>/ref-modes` handler. depends_on: []

These can run in parallel.

## Implementation Units

### Unit 1: jamsesh join

**File**: `cmd/jamsesh/sessioncmd/join.go`
**Story**: `epic-cc-plugin-session-commands-join-and-status`

Args: `<session-id-or-url> [--as <branch>] [--from <commit>]`.

Flow:
1. Resolve session id from URL or arg
2. Check auth (token exists in state); if not, print "run `jamsesh auth` first" and exit 1
3. If invite URL: accept invite first
4. GET /api/sessions/<id> for metadata
5. Clone the session bare repo via `git clone http://<portal>/git/<orgID>/<sessionID>.git <localPath>`
6. Checkout `jam/<sessionID>/<accountID>/<branch>` (creating from base or --from)
7. Write per-session state at `${CLAUDE_PLUGIN_DATA}/sessions/<sessionID>/`: `ref`, `instance_id` (CC session_id from env), starting cursors
8. Print summary

### Unit 2: jamsesh status

**File**: `cmd/jamsesh/sessioncmd/status.go`

Args: `[--json]`.

Reads current session from CC instance_id mapping in state. Calls:
- GET /api/sessions/<id>/refs → refs + modes + tips
- GET /api/sessions/<id>/comments?addressed_to=@<me>&resolved=false → unresolved
- (optionally GET /api/sessions/<id>/digest?since=<lastSeq> → recent activity)

Formats output as text or JSON.

### Unit 3: jamsesh fork

**File**: `cmd/jamsesh/sessioncmd/fork.go`
**Story**: `epic-cc-plugin-session-commands-fork-and-mode`

Args: `<commit-sha> [--as <branch>] [--mode sync|isolated]`.

Calls MCP `fork` tool via the binary's mcp client (need to add — for v1 just call the portal's MCP endpoint over HTTP with raw JSON-RPC).

Simpler: call portal REST instead. Add `POST /api/sessions/<id>/fork` to sessions-rest. But that's already-shipped. For v1: call MCP via stdlib HTTP — write JSON-RPC body, set Authorization Bearer, POST to `/mcp`.

Even simpler: have the portal expose `POST /api/sessions/<id>/forks` that mirrors the MCP fork tool's behavior. Ship that endpoint as part of this story.

Final v1: just call portal /mcp directly with a hand-rolled JSON-RPC body. The MCP endpoint already does fork. No new portal endpoint needed.

### Unit 4: jamsesh mode

**File**: `cmd/jamsesh/sessioncmd/mode.go`

Args: `sync|isolated`.

For v1 strict scope: `POST /api/sessions/<sessionID>/ref-modes` with `{ref, mode}`. New portal endpoint added in this story:

**Portal side addition** (in sessions package):
- `internal/portal/sessions/refmodes.go` — `SetRefMode` handler
- `docs/openapi.yaml` (edit) — POST /api/sessions/<id>/ref-modes path
- Regen
- Emit `mode.changed` event in the handler

Hmm this story is getting big. Let me defer the mode-server endpoint: the `jamsesh mode` CLI just updates local state for now, with a TODO comment for server-side. Document as v1 limitation.

OK revising: `mode` is local-only for v1. The portal-side mode-change endpoint is a follow-up.

## Implementation Order

(Parallel) join-and-status + fork-and-mode

## Testing

- join: mock portal + git server; verify clone + checkout + state write
- status: mock portal returns canned refs/comments; verify text + json output
- fork: mock portal MCP endpoint; verify JSON-RPC payload shape; verify local git fetch
- mode: update local state file; verify TODO marker for server side

## Risks

- **Mode v1 local-only**: a peer's UI won't reflect the mode change until a real server-side endpoint exists. Documented limitation. Operator can call MCP directly to set mode server-side as workaround.
- **MCP client in Go binary**: implementing raw JSON-RPC for one tool call is fine; if we add more tools later, extract a small client. v1 keeps it inline.
