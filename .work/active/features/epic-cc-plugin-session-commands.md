---
id: epic-cc-plugin-session-commands
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

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
