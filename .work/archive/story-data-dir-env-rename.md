---
id: story-data-dir-env-rename
kind: story
stage: done
tags: [refactor, plugin, documentation]
parent: null
depends_on: []
release_binding: v0.4.1
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

## Review (2026-05-24)

**Verdict**: Approve with comments

**Blockers**: none
**Important**:
- **Breaking change without user-facing migration documentation**: users
  whose CC plugin runtime previously set `CLAUDE_PLUGIN_DATA` to a
  CC-managed directory will find their state orphaned at the old location
  post-upgrade. The story's "strict cutover, no back-compat" decision is
  honored in code, but the user-facing migration story needs release-notes
  language and possibly an auto-migrate helper.
  → Item: `idea-data-dir-migration-helper` (parked in backlog)

**Nits**:
- The "warning: token migration encountered errors" branch at
  `cmd/jamsesh/main.go:39` was not literally dropped per the story
  instruction, but the agent's judgment is sound — the *probe* on missing
  env var is gone (DataDir() self-defaults), and the warning now serves
  the different purpose of logging actual per-session-token migration
  failures. Acceptable.
- Wrapper binary cache moved from `${CLAUDE_PLUGIN_DATA:-$HOME/.cache/jamsesh}/bin`
  to `${XDG_CACHE_HOME:-$HOME/.cache}/jamsesh/bin`. For users whose CC
  runtime set CLAUDE_PLUGIN_DATA, the cached binary now lives elsewhere
  post-upgrade and re-downloads on first invocation. Minor.

**Notes**:
- Comprehensive rename across 47 files: Go source + tests + wrapper +
  bats tests + 5 foundation docs. Acceptance grep clean.
- New `DataDir()` correctly implements XDG resolution order:
  `JAMSESH_DATA_DIR` → `${XDG_DATA_HOME}/jamsesh` → `${HOME}/.local/share/jamsesh`.
  `os.MkdirAll(dir, 0o700)` ensures directory exists with appropriate mode.
- Wrapper change cleanly separates cache (XDG_CACHE_HOME) from data (XDG_DATA_HOME)
  tiers — old code conflated them by reading CLAUDE_PLUGIN_DATA for both.
- Foundation docs rolled forward in place (PROTOCOL, SPEC, ARCH, UX, RELEASING).
  No "previously" prose. ARCHITECTURE.md "Local state layout" path updated
  correctly.
- Full Go suite green; bats wrapper tests updated to use XDG_CACHE_HOME.

**Next**: Release notes for the next version (v0.5.0 likely) need to
prominently call out the env var rename + state-location change.
