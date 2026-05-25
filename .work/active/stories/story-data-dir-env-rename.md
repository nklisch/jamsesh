---
id: story-data-dir-env-rename
kind: story
stage: review
tags: [refactor, plugin, documentation]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Rename CLAUDE_PLUGIN_DATA → JAMSESH_DATA_DIR with XDG self-default

## Brief

The local `jamsesh` binary currently keys its state directory off
`CLAUDE_PLUGIN_DATA`, which is set by the Claude Code plugin runtime for
hook subprocesses but **not** for `Bash`-tool subprocesses where the
skills actually invoke the CLI. The result: every skill-invoked
`jamsesh ...` call fails with `CLAUDE_PLUGIN_DATA is not set; this
binary must be invoked by the Claude Code plugin runtime`.

The state being persisted (OAuth tokens, per-session bearer tokens,
retry queues, `portal_url` override, instance-id bindings, local
session refs) is jamsesh's own data — it has no semantic dependency on
Claude Code. The dependency was a convenience that became a hard
coupling.

This story renames the env var and self-defaults its value:

- Rename `CLAUDE_PLUGIN_DATA` → `JAMSESH_DATA_DIR` throughout the Go
  source and tests.
- Rename `state.PluginDataDir()` → `state.DataDir()`.
- When the env var is unset, default to
  `${XDG_DATA_HOME:-$HOME/.local/share}/jamsesh` (create the directory
  if absent). No more error.
- Update the wrapper script `plugins/jamsesh/bin/jamsesh` so the
  binary-cache path also resolves via XDG (drop the
  `CLAUDE_PLUGIN_DATA` fallback there too).
- Drop the token-migration "warning: token migration encountered
  errors..." branch that explicitly probes `CLAUDE_PLUGIN_DATA`.
- Update the three `plugins/jamsesh/skills/*/SKILL.md` files: the
  setup gate stays (still need `JAMSESH_PORTAL_URL` configured) but
  the `CLAUDE_PLUGIN_DATA` workaround language is removed.
- Roll forward foundation docs: `docs/PROTOCOL.md`, `docs/SPEC.md`,
  `docs/ARCHITECTURE.md`, `docs/UX.md`, `docs/RELEASING.md` (in-place
  per the rolling-foundation principle — no "previously" prose).

Strict cutover, no back-compat shim — existing CC-runtime invocations
will land state in the XDG path, same as Bash-invoked calls, giving one
canonical location everywhere.

## Acceptance

- `jamsesh ...` succeeds from a fresh shell with neither
  `JAMSESH_DATA_DIR` nor `CLAUDE_PLUGIN_DATA` set; state lands under
  `~/.local/share/jamsesh/`.
- `JAMSESH_DATA_DIR=/tmp/foo jamsesh ...` honors the override.
- `grep -rn 'CLAUDE_PLUGIN_DATA' cmd/ internal/ docs/ plugins/`
  returns empty (test fixtures excepted only if they intentionally
  test legacy behavior — which they shouldn't post-cutover).
- All Go tests pass.
- Foundation docs roll forward without "previously" / migration prose.

## Implementation notes

- `state.PluginDataDir()` renamed to `state.DataDir()`; self-defaults to
  `${XDG_DATA_HOME:-$HOME/.local/share}/jamsesh` when `JAMSESH_DATA_DIR`
  is unset, creating the directory with `os.MkdirAll(dir, 0o700)`.
- `plugins/jamsesh/bin/jamsesh` wrapper cache path changed from
  `${CLAUDE_PLUGIN_DATA:-${HOME}/.cache/jamsesh}/bin` to
  `${XDG_CACHE_HOME:-${HOME}/.cache}/jamsesh/bin` (XDG-compliant split
  between data and cache tiers).
- Bats wrapper tests updated to export `XDG_CACHE_HOME` instead of
  `CLAUDE_PLUGIN_DATA`; cache path assertions updated to
  `${XDG_CACHE_HOME}/jamsesh/bin/`.
- Acceptance grep `grep -rn 'CLAUDE_PLUGIN_DATA' cmd/ internal/ docs/ plugins/ tests/`
  returns empty.
