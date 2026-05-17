---
id: epic-cc-plugin
kind: epic
stage: done
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

Locked at epic-design time (this pass):

- **Multi-arch binary distribution**: 5 binaries per release (darwin-
  amd64/arm64, linux-amd64/arm64, windows-amd64). `plugin.json` carries
  per-arch entries; the marketplace fetches the right one. The CI
  pipeline that builds these lives in `epic-distribution`; this epic's
  `packaging` feature delivers the manifest shape.
- **Local state file format**: plain text per-file for single-value
  files (`token`, `refresh_token`, `portal_url`); JSON for per-session
  structured state. Token files mode 0600. Atomic writes (temp + rename).
  Matches the layout already pinned in ARCHITECTURE.md.
- **`jamsesh status` output**: human-readable text by default;
  `--json` flag for scripted consumption.
- **OAuth client security**: PKCE (S256) + state parameter on both
  the local-listener and `--device-code` flows. Cheap, universal best
  practice, forward-compatible.
- **Auto-commit on turn end**: message format `"<turn summary>
  [jamsesh auto-commit at turn end]"` where `<turn summary>` is the
  truncated first line of the last user prompt (or "WIP"); commits
  carry the `Jam-Auto-Commit: true` trailer for git-log
  distinguishability.
- **`headersHelper` shape**: synchronous read of token file, outputs
  `{"Authorization": "Bearer <token>"}` as JSON. Refresh happens
  asynchronously on any 401 elsewhere — never in `mcp-headers` itself.
- **Auto-loaded teaching skill scope**: operational, ≤2500 words.
  Covers dual-mode summary, trailer conventions, addressed-comment
  syntax + use patterns, conflict-resolution flow, digest reading, MCP
  tool usage. Points at foundation docs for deeper context; doesn't
  duplicate them.

## Decomposition

Four child features, cleanly separable by what they produce:

- **packaging** is static artifact authoring — manifest, slash-command
  skills, hooks.json, .mcp.json, the auto-loaded teaching skill. No Go
  code; the artifacts reference subcommand names only, so packaging is
  independent of the binary's implementation.
- **binary-foundation** is the Go binary's scaffold — subcommand
  router, JSON hook IO, local state package, OAuth flows, token
  refresh, `mcp-headers` subcommand. Foundation for the other binary
  subcommands.
- **session-commands** wraps user-facing slash commands (`join`,
  `status`, `fork`, `mode`).
- **hooks** implements the six CC lifecycle hooks plus the shared
  retry queue.

Critical path: `binary-foundation → {session-commands || hooks}`.
`packaging` parallelizes from day one (no deps).

### Child features

- `epic-cc-plugin-packaging` — plugin.json, hooks.json, .mcp.json,
  slash-command SKILL.md files, auto-loaded teaching skill — depends
  on: `[]`
- `epic-cc-plugin-binary-foundation` — Go binary subcommand router,
  JSON hook IO scaffold, local state package, OAuth client (local-
  listener + device-code), token refresh, `mcp-headers` subcommand —
  depends on: `[]`
- `epic-cc-plugin-session-commands` — `jamsesh join`, `status`,
  `fork`, `mode` — depends on: `[epic-cc-plugin-binary-foundation]`
- `epic-cc-plugin-hooks` — 6 lifecycle hooks + per-session retry
  queue + transient/permanent classifier — depends on:
  `[epic-cc-plugin-binary-foundation]`

### Decomposition risks

- **`hooks` is at the size ceiling.** 12-15 units. If retry-queue
  interactions compound (parent-before-child ordering, persistence
  under crashes, queue-too-large handling in `stop`), the design pass
  may split out a `retry-queue` sub-feature.
- **Device-code OAuth flow has subtle timing concerns.** Polling
  cadence, code expiry, UX during wait. Design pass references RFC 8628
  and locks the polling cadence.
- **Auto-loaded teaching skill is loaded into every agent turn.**
  Verbose teaching is expensive context. ≤2500-word budget is the
  safety valve; design pass enforces it as a hard limit.
- **`headersHelper` timing assumption.** If CC caches headers per
  connection and a token refreshes mid-session, MCP calls keep going
  with stale Authorization until reconnect. CC's MCP client spec is
  the source of truth on 401-retry behavior; design pass documents the
  assumption.

## Final review (2026-05-17)

**Verdict**: Approve

**Notes**: All 4 child features at done: binary-foundation (subcommand router + OAuth + state + portal client + refresh), packaging (manifest + hooks.json + .mcp.json + skills + teaching skill), hooks (6 CC lifecycle hooks with retry queue + push gate), session-commands (join/status/fork/mode). The CC plugin is complete and integrates end-to-end with the portal.
