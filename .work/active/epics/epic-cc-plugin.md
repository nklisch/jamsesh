---
id: epic-cc-plugin
kind: epic
stage: drafting
tags: [plugin]
parent: null
depends_on: [epic-portal-api]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Claude Code Plugin

## Brief

The Claude Code integration that humans install to participate in jam
sessions. Ships as a single CC plugin package containing:

- **Manifest** (`.claude-plugin/plugin.json`) with name, version, author.
- **Binary** (`bin/jamsesh`, multi-arch) — the Go binary that does all the
  local work. Added to CC's Bash PATH automatically.
- **Skills** (`skills/<name>/SKILL.md`) — slash commands (which are skills
  in CC's plugin model) for `join`, `status`, `fork`, `mode`, and the
  auto-loaded `skills/jamsesh/SKILL.md` that teaches the agent the dual-mode
  model, commit trailer conventions, addressed-comment semantics, conflict
  resolution patterns, and how to read the digest.
- **Hooks** (`hooks/hooks.json`) — wires lifecycle events to `jamsesh hook
  <name>` subcommands: SessionStart (initial context), UserPromptSubmit
  (digest injection), PreToolUse (push-gate), PostToolUse (push-per-commit),
  Stop (turn-end push), SessionEnd (cleanup).
- **MCP config** (`.mcp.json`) — points CC's MCP client at the portal's
  HTTPS-MCP endpoint with a `headersHelper` script (`jamsesh mcp-headers`)
  that supplies the Bearer token from local state at connection time.

The local `jamsesh` binary owns OAuth client flow (`jamsesh auth` runs the
browser-based flow against the portal), local state at
`${CLAUDE_PLUGIN_DATA}/` (tokens, per-session ref bindings, digest
cursors), session-join orchestration, and all hook implementations.

This epic does NOT cover portal-side anything; it does NOT cover finalize
(`epic-finalize-flow` handles the cross-component slice that includes the
plugin's finalize subcommand).

## Foundation references

- `docs/ARCHITECTURE.md` — The `jamsesh` binary, Claude Code plugin package,
  Data flow: a turn
- `docs/SPEC.md` — Local client
- `docs/PROTOCOL.md` — Lifecycle hook contracts, Local state schema
- `docs/UX.md` — Flow: joining a session, Flow: an agent turn

## Design decisions

- **OAuth callback flow for `jamsesh auth`**: both — local HTTP listener
  as the default, `--device-code` flag for headless/SSH environments.
  - Default: open a browser to the portal OAuth endpoint with
    `redirect_uri=http://localhost:<port>/callback` (ephemeral port picked
    at runtime); run a one-shot HTTP server on that port to catch the code;
    exchange code for tokens; shut down listener.
  - `--device-code`: portal returns a short user code and verification URL;
    user opens the URL on any browser-capable device and enters the code;
    `jamsesh auth` polls the portal until the code is confirmed and tokens
    are issued.
  - Covers desktop CC sessions (local listener) and remote-dev sessions
    via SSH/tmux (device-code) without forcing the user to port-forward.
  - Refresh-token rotation handled silently in the background by the
    binary on any portal API call that returns 401; if refresh fails, the
    next hook surfaces "session token expired, run `jamsesh auth` to
    reauthenticate" via additionalContext.

- **Push-failure handling in PostToolUse hook**: hybrid retry policy.
  - **Transient errors** (network unreachable, 5xx from portal, timeouts):
    retry up to 3 times with exponential backoff (250ms, 1s, 4s). If all
    3 retries fail, surface to the agent via the hook's stdout: "push
    queued for retry — last error: <message>" + add the commit to a
    local retry queue (next `UserPromptSubmit` retries before generating
    the digest).
  - **Permanent errors** (pre-receive rejection 4xx with structured error,
    auth failures 401, scope violations): fail loud immediately. Surface
    the full rejection message to the agent so it can react (e.g., scope
    violation → agent can revert the offending change).
  - The local binary distinguishes transient vs permanent by HTTP status
    + structured error code in the response body. Pre-receive rejection
    messages follow the standard error contract (`error: push.scope_violation`
    + `details.paths: [...]`) so the agent gets actionable detail.
  - Retry queue is per-session, ordered, FIFO. If a queued push has a
    parent commit that's also queued, the parent goes first. If the queue
    grows beyond a sensible threshold (say 10 queued commits), Stop hook
    refuses to fire and surfaces a "session is wedged, run `jamsesh status`
    to investigate" error.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->


## Anticipated child features

Provisional — actual decomposition lands when this epic is designed.

- Plugin package layout (manifest, skills/, hooks/, .mcp.json structure)
- Binary skeleton: subcommand router, JSON in/out for hooks
- OAuth client flow (`jamsesh auth`)
- Local state management at `${CLAUDE_PLUGIN_DATA}/`
- `jamsesh join` (session join, ref bind, working-tree setup)
- Hook: `session-start` (initial context injection)
- Hook: `user-prompt-submit` (git fetch + portal digest + format)
- Hook: `pre-tool-use` (push-gate; deny `git push`, `git config remote.*`)
- Hook: `post-tool-use` (push-per-commit on successful `git commit`)
- Hook: `stop` (auto-commit remainder + turn-end push + `turn.ended` POST)
- Hook: `session-end` (presence cleanup)
- Slash commands: status, fork, mode
- Auto-loaded teaching skill (`skills/jamsesh/SKILL.md`) — the agent's
  primer on jamsesh mechanics
- MCP config with `headersHelper` integration

<!-- Design pass on each child feature will fill in specifics. -->
