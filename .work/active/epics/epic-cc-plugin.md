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
