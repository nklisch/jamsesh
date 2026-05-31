---
name: jam
description: Start, create, or join a jam session in any form — durable or playground, new or existing
argument-hint: "[new|join] [flags]"
---

# /jamsesh:jam

> **First-time setup gate.** Before invoking `jamsesh` for the first
> time in a session, check `JAMSESH_PORTAL_URL` is set. If unset, run
> the one-time setup flow in the root `jamsesh` skill (section 0,
> "Pre-flight — portal URL must be configured") *before* the command
> below. Without it the binary hits a placeholder URL and fails.

When the user wants to start, create, or join a jam session in any form —
durable or playground, new or existing — invoke `jamsesh jam $ARGUMENTS`.
The binary's `jam` subcommand routes to the right underlying operation
based on what the user requested:

- "create a new session" / "spin up a jam" / "let's start a session"
  → `jamsesh jam new [flags]`
- "create a playground" / "throwaway session" / "try playground"
  → `jamsesh jam new --playground [flags]`
- "join this session" / "<URL>" / "join the jam at <id>"
  → `jamsesh jam join <url-or-id>`

**Required-arg discipline:** the binary's `jam new` subcommand requires
`--org <id>` for durable sessions when no TTY is available (i.e. always,
when invoked from the CC bash tool). If the user's request doesn't pin
an org, **ask them which one** in your reply before invoking — don't
silently default. For playground sessions (`--playground` flag), no org
is needed.

**Optional flags for `jam new`:**
- `--goal '<text>'` — session goal (recommended; helps every joining agent)
- `--scope '<glob>'` — writable scope; defaults to `**` if not provided
- `--mode sync|isolated` — ref mode; defaults to `sync`
- `--invite alice@x,bob@y` — comma-separated emails to invite at create
- `--open` — open the created session in your browser **as your CLI identity** (durable: opens the session view signed in as you; playground: resumes your anonymous participant, not a fresh one). Non-interactive. On mint failure, warns and falls back to a token-free open.

**For `jam join`:**
- `<url-or-id>` — required; the session URL or just the session ID
- `--as <branch>` — optional ref-branch name (defaults to `main`)
- `--from <commit>` — optional fork point
- `--open` — open the joined session in your browser **as your CLI identity** (signed in as you). Non-interactive. On mint failure, warns and falls back to a token-free open.

## Opening in the browser

Both `jam new` and `jam join` accept `--open`. The CLI never prompts — the
flag is non-interactive. When `jam` is invoked, **offer to open the session
for the human**: fold the offer into the questions you are already asking
(org, goal, etc.) and pass `--open` if they say yes.

`--open` adopts the CLI identity: it mints a single-use resume token and
opens a `/resume#rt=<token>` URL so the browser lands in the session *as
you* — the same participant and bearer the CLI is acting as. On mint failure,
the CLI warns and falls back to opening the plain session URL without identity.

**Destruction warnings:** when a `playground.destruction_warning`
event surfaces in your UserPromptSubmit digest (the session is ~5
minutes from idle/hard-cap destruction), surface the warning to the
human in your reply, including `ends_at` and the imperative:
"Run `jamsesh finalize --local` now if you want to keep this work."

## Status

When the user wants to inspect a jam session ("what's the state",
"show me the session", "who's online"), invoke
`jamsesh status [--json]`. Output groups durable and playground
sessions separately.

If the user has only playground sessions (no account-wide OAuth),
status still works — it enumerates per-session tokens. No
"sign in first" friction.

## Fork

When the user wants to fork from a peer's ref or commit
("fork from amber-otter's tip", "branch off f02ac41"), invoke
`jamsesh fork <commit-sha> [--as <branch>] [--mode sync|isolated]`.

Default mode is `sync` (auto-merger will weave the new ref into draft);
isolated mode keeps the fork private until promoted.

## Mode

When the user wants to flip the current ref's mode ("switch to
isolated", "rejoin sync"), invoke `jamsesh mode sync|isolated`. The
flip takes effect on the next push.

Mode-flip semantics:
- `isolated → sync`: subsequent pushes are auto-merger candidates;
  expect conflicts proportional to drift while isolated
- `sync → isolated`: subsequent pushes don't auto-merge; existing
  merged commits remain in draft

## Resume

When the user wants to reopen an existing session in the browser as their CLI
identity ("reopen the session", "open the browser", "resume the session"),
invoke `jamsesh resume [session-id]`.

- **Bare** (`jamsesh resume`) — resumes the session bound to the current
  Claude Code instance (`CLAUDE_SESSION_ID`). Errors if the instance is not
  mapped to a session (hint: `jamsesh status`). Outside CC context, resumes
  the single session if exactly one exists; errors on zero or multiple.
- **Explicit id** (`jamsesh resume <session-id>`) — resumes that specific
  session regardless of the current CC instance. Useful when managing multiple
  sessions.
- On mint failure the command exits with an error and opens nothing. Run the
  command again to mint a fresh token.
- `jamsesh status` lists all sessions and their IDs when disambiguation is
  needed.
