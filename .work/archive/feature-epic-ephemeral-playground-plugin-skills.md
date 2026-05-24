---
id: feature-epic-ephemeral-playground-plugin-skills
kind: feature
stage: done
tags: [plugin, playground]
parent: epic-ephemeral-playground
depends_on: [feature-epic-ephemeral-playground-cli-first-creation, feature-epic-ephemeral-playground-session-lifecycle]
release_binding: v0.4.0
gate_origin: null
created: 2026-05-23
updated: 2026-05-24
---

## Implementation summary (autopilot)

All 4 child stories advanced to `stage: review`:

- `story-...-plugin-skills-jam-consolidation` — new `/jamsesh:jam` SKILL.md (intent vocabulary) + new `cmd/jamsesh/jamcmd/jam.go` dispatcher (sub-subcommands new + join via fresh factory calls to avoid urfave/cli v3 parent-pointer aliasing); deleted obsolete `plugins/jamsesh/skills/join/SKILL.md`
- `story-...-plugin-skills-bearer-storage` — `state.ReadSessionToken` / `WriteSessionToken` / `ListSessions` helpers + `MigrateToPerSessionTokens` one-shot idempotent migration + refresh-flow callsite update writing to per-session paths
- `story-...-plugin-skills-status-enumeration` — `jamsesh status` enumerates per-session tokens (no account-wide OAuth required); output groups durable + playground sessions separately; new `portalclient.GetJSONWithBearer` generic for per-session calls
- `story-...-plugin-skills-destruction-warning` — UserPromptSubmit hook surfaces `playground.destruction_warning` events in the urgent section; auto-loaded SKILL.md "Playground sessions" section appended; OpenAPI schema extended with `urgent_events` + `PlaygroundDestructionWarningPayload`

**Cross-cutting deviations**:
- jam-consolidation discovered urfave/cli v3 parent-pointer aliasing issue (subCmd.parent gets overwritten when one *cli.Command instance is registered under two parents); resolution: call factory functions fresh in `JamCommand()` to obtain pointer-distinct instances
- destruction-warning fixed several pre-existing bugs in-session per test-integrity rule: missing `time` import in receive_pack.go (from sibling story), `mcpheaders` integration with per-session bearer storage, zero-sessions guard in migration to prevent legacy-token destruction

**Verification status**: `go build ./cmd/jamsesh/...` clean, `go test ./cmd/jamsesh/...` passes, `go vet ./...` clean.

# CC plugin skills + playground-aware join flow

## Brief

Aligns the Claude Code plugin's skill surface with the unified CLI-first
creation model and adds playground-specific behavior to the existing
join path. Concretely:

- **New skill** `/jamsesh:new` — `plugins/jamsesh/skills/new/SKILL.md`
  body invokes `jamsesh new $ARGUMENTS` and teaches the agent the new
  creation pattern (interactive prompts, flag overrides, the "this
  pushes your local HEAD as base" mental model).
- **New namespaced skill** `/jamsesh:playground:new` —
  `plugins/jamsesh/skills/playground-new/SKILL.md` body invokes
  `jamsesh new --playground $ARGUMENTS` and teaches the agent the
  ephemeral-mode constraints (no claim-to-durable, idle + hard-cap
  destruction, finalize-locally-to-keep imperative).
- **Extend `/jamsesh:join`** — `plugins/jamsesh/skills/join/SKILL.md`
  body updated to mention playground URL shape; binary subcommand
  recognizes `/playground/s/<token>` URLs, fetches the anonymous
  bearer via `POST /api/playground/sessions/{id}/join`, writes it to
  `${CLAUDE_PLUGIN_DATA}/sessions/<session-id>/token` in the same
  shape as a durable token so `mcp-headers` and `BasicAuth` work
  unchanged. Joiner nickname is read from a sidecar
  `<session-id>/nickname` file for `jamsesh status` display.
- **Auto-loaded SKILL.md update** — `plugins/jamsesh/skills/jamsesh/SKILL.md`
  gains a "Playground sessions" section teaching agents about
  ephemeral-mode constraints, the addressing convention for anonymous
  handles (`@quiet-fox` works the same as `@alice` for addressing),
  and the agents' role in nudging humans to finalize-locally before
  destruction.

The destruction-warning UX nudge on the agent side is small but worth
explicit attention: when the digest carries an "ending in <5 min" event,
the SKILL.md instructs the agent to surface that to the human in the
next turn's reply.

## Epic context
- Parent epic: `epic-ephemeral-playground`
- Position in epic: **wave 3** — depends on `cli-first-creation` for
  the `jamsesh new` binary surface and on `session-lifecycle` for the
  `/api/playground/sessions/{id}/join` endpoint. Parallelizable with
  `portal-ui`.

## Foundation references
- `docs/ARCHITECTURE.md` § Claude Code plugin package — the skills
  directory layout that this feature extends
- `docs/SPEC.md` § Auth model — the anonymous-bearer contract that the
  plugin's token-storage path must respect
- `docs/PROTOCOL.md` — addressing convention for anonymous handles and
  the "destruction-warning event" digest extension are rolled into
  PROTOCOL.md by this feature's design pass

## Mockups
- Inherits parent epic flow:
  `.mockups/flows/playground-onboarding/index.html` (step 02 — CLI
  output — and step 06 — joiner session — represent the user-visible
  output of this feature's CLI surface)
- No additional feature-tier mocks needed — CLI surfaces are text, not
  visual

## Design decisions

Locked at `--only-questions` time. Feature-design Phase 5 inherits these
as fixed input.

- **Skill consolidation — single `/jamsesh:jam` entry point**: collapse
  the originally-planned `/jamsesh:new`, `/jamsesh:playground:new`,
  and `/jamsesh:join` into **one** skill at `/jamsesh:jam` (canonical
  slash form, honoring CC's `plugin:skill` namespace convention). The
  single skill body teaches the agent: "When the user wants to start,
  create, or join a jam in any form — playground or durable, new or
  existing — invoke `jamsesh <subcommand> $ARGUMENTS`. The binary
  subcommands are `new`, `new --playground`, `join <url|id>`. Use the
  user's natural-language request to pick the right one and the right
  flags. If anything is ambiguous, ask the human in CC." Underlying
  binary keeps its existing subcommand structure (`jamsesh new`,
  `jamsesh new --playground`, `jamsesh join` — owned by
  `cli-first-creation` and `session-lifecycle`); the consolidation is
  purely at the **skill** layer, leveraging agent intelligence to
  translate intent to subcommand invocation. The broader audit
  pattern — generalizing this same consolidation to `/jamsesh:status`,
  `:fork`, `:mode`, `:finalize` — is owned by the sibling feature
  `feature-epic-ephemeral-playground-skill-consolidation` (wave 4),
  which also extends the `/jamsesh:jam` skill body with the
  status/fork/mode vocabulary.

- **Bearer storage model — unified per-session**: both durable and
  playground sessions store their bearers at
  `${CLAUDE_PLUGIN_DATA}/sessions/<session-id>/token` (mode 0600). The
  legacy account-wide `${CLAUDE_PLUGIN_DATA}/token` is migrated
  forward on first run after upgrade: the binary enumerates the
  user's bound sessions, fans the account-wide token out into
  per-session files, and leaves the legacy path as a stub pointing at
  the new layout. After migration, `jamsesh mcp-headers` and the git
  Basic-auth resolver always look up by CC session_id → per-session
  token. Symmetric across session kinds; no kind-branching in
  resolution paths. The refresh-token model (account-wide refresh
  exchanged for short-lived access) still applies to durable sessions
  — refresh tokens stay account-wide (`refresh_token` file unchanged);
  per-session files carry only the short-lived access bearer.

- **`/jamsesh:status` under playground-only (no account-wide OAuth)**:
  works seamlessly. After the unified per-session storage lands,
  status enumerates `${CLAUDE_PLUGIN_DATA}/sessions/*/token`, calls
  each session's `GET /api/sessions/{id}/status` with its bearer, and
  composites the output. No required account-wide token. Anonymous-
  only users (never ran OAuth) get full status functionality for
  their playground sessions. Output groups results by session kind
  (durable vs playground) for clarity.

- **Destruction-warning surfacing to the agent**:
  `playground.destruction_warning` event in the UserPromptSubmit
  digest, fired ~5 min before the closer of (idle_timeout_at,
  hard_cap_at). Payload shape:
  `{ kind: "playground.destruction_warning", reason: "idle"|"hard_cap",
     ends_at: <ISO8601>, remaining_seconds: <int>,
     session_id: <id> }`. The digest's "urgent" section surfaces it
  alongside addressed comments. The auto-loaded
  `plugins/jamsesh/skills/jamsesh/SKILL.md` (this feature's update)
  instructs the agent: "When you see a `playground.destruction_warning`
  event, surface the warning to the human in your next reply,
  including `ends_at` and the imperative `Run `jamsesh finalize
  --local` now if you want to keep this work.`" The agent treats this
  with the same attention-grabbing weight as an addressed comment —
  human-actionable, time-sensitive. Threshold (5 min) is hardcoded as
  the warning trigger; the destruction sweep is responsible for
  computing the threshold crossing and emitting the event idempotently
  (per-session, only-once-per-warning-kind).

### Scope expansion note

The `/jamsesh:jam` consolidation is a non-additive change to the skill
surface — `/jamsesh:new` and `/jamsesh:playground:new` (originally
planned) do not get added as standalone skills. Instead `/jamsesh:jam`
is the sole new skill at this feature's tier, and `/jamsesh:join` is
**deleted outright** (no backward-compat alias). Per the
`skill-consolidation` sibling feature's `--only-questions` decisions,
the pre-launch reality means there are no installed-base users to
migrate; deprecation-alias hygiene is unnecessary work that would
ship dead code on day one. The deletion of `/jamsesh:join`'s
SKILL.md file is owned by this feature (since it's a direct
consequence of `/jamsesh:jam`'s creation here); the parallel
deletions of `/jamsesh:status`, `:fork`, `:mode` SKILL.md files are
owned by the `skill-consolidation` feature in wave 4.

This is a slight expansion of the original feature brief (which only
extended `/jamsesh:join`); the substantive work is comparable, just
re-organized.

## Architectural choice

**Four-story decomposition along plugin-surface concerns** — skill
consolidation, bearer storage migration, status enumeration, and
destruction-warning surfacing. Each story owns a coherent
plugin-surface concern with its own test surface.

Why this shape:
- **Single monolithic feature**: too much surface (4 distinct
  concerns: a new skill, a state-migration, a subcommand refactor,
  a hook-data shape change) for one agent to land cleanly.
- **Per-file stories**: too granular — the destruction-warning
  surfacing modifies both the user-prompt-submit hook AND the
  auto-loaded SKILL.md, and they need to stay consistent.

## Implementation units

4 stories. Each is one of the chunks below. Full code skeletons live
in the per-story body files.

### Story 1: `/jamsesh:jam` skill + binary subcommand dispatcher
**Files**:
- `plugins/jamsesh/skills/jam/SKILL.md` (new) — agent-facing skill body
  that teaches the intent-vocabulary
- `cmd/jamsesh/jamcmd/jam.go` (new) — subcommand dispatcher
- `cmd/jamsesh/jamcmd/jam_test.go`
- `cmd/jamsesh/main.go` (modify) — register `JamCommand()`
- `plugins/jamsesh/skills/join/SKILL.md` (delete) — per pre-launch
  reality, no alias; outright deletion

The `/jamsesh:jam` SKILL.md body (markdown) teaches the agent the
intent vocabulary:

```markdown
# /jamsesh:jam

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

**For `jam join`:**
- `<url-or-id>` — required; the session URL or just the session ID
- `--as <branch>` — optional ref-branch name (defaults to `main`)
- `--from <commit>` — optional fork point

**Destruction warnings:** when a `playground.destruction_warning`
event surfaces in your UserPromptSubmit digest (the session is ~5
minutes from idle/hard-cap destruction), surface the warning to the
human in your reply, including `ends_at` and the imperative:
"Run `jamsesh finalize --local` now if you want to keep this work."
```

The `cmd/jamsesh/jamcmd/jam.go` dispatcher:

```go
package jamcmd

import (
    "context"
    "github.com/urfave/cli/v3"
    "<module>/cmd/jamsesh/sessioncmd"
)

func JamCommand() *cli.Command {
    return &cli.Command{
        Name:  "jam",
        Usage: "Create, join, or manage a jam session (intent-driven entry)",
        Commands: []*cli.Command{
            // jam new dispatches to the existing sessioncmd.NewCommand action
            // (the wave-1 cli-first-creation feature's NewCommand).
            // We import NewCommand and re-export it under jam.
            sessioncmd.NewCommand(), // renamed in this PR: keeps the Name "new"
            sessioncmd.JoinCommand(), // wave-1's existing JoinCommand, kept
        },
    }
}
```

The top-level `jamsesh new` and `jamsesh join` binary subcommands also
remain (per the locked decision: binary surface stays rich; skill
surface gets thin). The `/jamsesh:jam` skill just teaches the agent to
invoke them via the `jam` prefix. Practically: `jamsesh new --playground`
and `jamsesh jam new --playground` are equivalent invocations.

Delete `plugins/jamsesh/skills/join/SKILL.md` (its job is subsumed by
`/jamsesh:jam`).

**Acceptance criteria**:
- [ ] `plugins/jamsesh/skills/jam/SKILL.md` exists; body content matches
      the intent-vocabulary spec above
- [ ] `jamsesh jam new --playground` invokes the playground create path
      (verified by httptest mock of the playground REST endpoint)
- [ ] `jamsesh jam join <url>` invokes the existing join path
- [ ] `jamsesh jam --help` lists `new` and `join` as sub-subcommands
- [ ] `plugins/jamsesh/skills/join/SKILL.md` is deleted (verified via
      `! test -e plugins/jamsesh/skills/join/SKILL.md`)
- [ ] The top-level `jamsesh new` and `jamsesh join` binary subcommands
      still work (don't break direct CLI users mid-migration)

### Story 2: Unified per-session bearer storage + migration
**Files**:
- `cmd/jamsesh/state/state.go` (extend) — add `ReadSessionToken(sessionID)`,
  `WriteSessionToken(sessionID, token)`, migration helper
- `cmd/jamsesh/state/state_test.go` (extend)
- `cmd/jamsesh/state/migrate.go` (new) — one-shot migration helper

The migration is idempotent and runs at binary startup (or on first
invocation that touches token storage). It fans out the legacy
`${CLAUDE_PLUGIN_DATA}/token` into `${CLAUDE_PLUGIN_DATA}/sessions/<id>/token`
for every bound session listed under `${CLAUDE_PLUGIN_DATA}/sessions/`.
The legacy `token` file is then replaced with a stub `MIGRATED_TO_PER_SESSION`
to prevent re-migration.

```go
// cmd/jamsesh/state/state.go (extension)

// ReadSessionToken returns the bearer for the given session.
// Returns fs.ErrNotExist if no token exists for the session.
func ReadSessionToken(sessionID string) ([]byte, error) {
    return Read("sessions/" + sessionID + "/token")
}

// WriteSessionToken stores the bearer for the given session at mode 0600.
func WriteSessionToken(sessionID string, token []byte) error {
    return Write("sessions/" + sessionID + "/token", token, 0600)
}

// cmd/jamsesh/state/migrate.go (new file)

// MigrateToPerSessionTokens fans out the legacy account-wide token to
// per-session token files. Idempotent; safe to call on every binary
// invocation. No-op if migration was already performed (detected via
// the MIGRATED_TO_PER_SESSION stub at the legacy path).
//
// Behavior:
//   - If legacy token file doesn't exist or contains MIGRATED_TO_PER_SESSION:
//     no-op (already migrated or fresh install)
//   - If legacy token exists with real token bytes:
//     - For each subdirectory under ${CLAUDE_PLUGIN_DATA}/sessions/:
//       - If sessions/<id>/token doesn't already exist, write the
//         legacy token to it
//     - After all sessions handled: replace legacy token with
//       MIGRATED_TO_PER_SESSION stub
//
// Errors are logged but non-fatal — partial migrations succeed safely;
// next invocation retries the unfanned-out sessions.
func MigrateToPerSessionTokens(logger Logger) error {
    legacy, err := Read("token")
    if errors.Is(err, fs.ErrNotExist) { return nil } // fresh install
    if err != nil { return err }
    if string(legacy) == "MIGRATED_TO_PER_SESSION" { return nil }

    sessions, err := ListSessions() // new helper: returns []string of session IDs
    if err != nil { return err }

    for _, sessID := range sessions {
        if _, err := ReadSessionToken(sessID); err == nil { continue } // already done
        if err := WriteSessionToken(sessID, legacy); err != nil {
            logger.Warn("migration: failed to write per-session token", "session_id", sessID, "err", err)
            continue
        }
    }

    return Write("token", []byte("MIGRATED_TO_PER_SESSION"), 0600)
}
```

Call site in `cmd/jamsesh/main.go`:

```go
// In main(), after PluginDataDir resolution, before action dispatch:
if err := state.MigrateToPerSessionTokens(logger); err != nil {
    logger.Warn("token migration encountered errors", "err", err)
    // Don't fail; the next invocation will retry
}
```

Subsequent code (mcp-headers, git-Basic-auth resolution) reads via
`ReadSessionToken(currentSessionID())` instead of the legacy `ReadToken()`.

The legacy `ReadToken()` function stays for now — refresh tokens are
still account-wide (per the locked decision), and the refresh flow
uses it. Plugin-skills consumers (mcp-headers, BasicAuth) migrate to
per-session reads; refresh-token reads stay on `ReadToken()`/`ReadRefreshToken()`.

Actually, refresh stays separate — `ReadRefreshToken()` keeps its
current path (`refresh_token` file) because refresh tokens are
account-wide. Only access tokens get per-session storage.

**Acceptance criteria**:
- [ ] Fresh install (no legacy token, no sessions dir): migration is
      a no-op, no errors
- [ ] Existing install with legacy `token` file + 2 session dirs:
      migration writes per-session tokens to both, replaces legacy
      with stub
- [ ] Already-migrated install (stub exists): migration is no-op
- [ ] Partial-failure resilience: if one session-write fails, the
      others still succeed; next invocation retries
- [ ] `ReadSessionToken` returns the session-scoped bearer; falls back
      to nothing (returns error) — no implicit fallback to legacy token

### Story 3: `/jamsesh:status` enumeration under anon-mode
**Files**:
- `cmd/jamsesh/sessioncmd/status.go` (modify) — enumerate per-session tokens
- `cmd/jamsesh/sessioncmd/status_test.go` (extend)

The existing status subcommand assumes an account-wide token exists.
Update it to enumerate `${CLAUDE_PLUGIN_DATA}/sessions/*/token` and
call each session's `GET /api/sessions/{id}/status` (or playground
equivalent for sessions in the reserved playground org) with the
per-session bearer.

```go
// cmd/jamsesh/sessioncmd/status.go (modified)

func statusAction(ctx context.Context, cmd *cli.Command) error {
    sessions, err := state.ListSessions()
    if err != nil { return err }
    if len(sessions) == 0 {
        fmt.Println("No sessions bound to this Claude Code instance.")
        fmt.Println("Start one with /jamsesh:jam.")
        return nil
    }

    pc := buildPortalClient() // base URL from state, no global auth header
    var durables, playgrounds []SessionStatus

    for _, sessID := range sessions {
        bearer, err := state.ReadSessionToken(sessID)
        if err != nil {
            // Token missing — skip with a note; session entry exists but no bearer
            fmt.Fprintf(os.Stderr, "warning: no token for session %s\n", sessID)
            continue
        }
        status, err := fetchSessionStatus(ctx, pc, sessID, bearer)
        if err != nil {
            fmt.Fprintf(os.Stderr, "warning: status fetch failed for %s: %v\n", sessID, err)
            continue
        }
        if status.IsPlayground {
            playgrounds = append(playgrounds, status)
        } else {
            durables = append(durables, status)
        }
    }

    printStatusGrouped(cmd.Bool("json"), durables, playgrounds)
    return nil
}
```

Status output groups durable and playground sessions separately,
showing the relevant per-kind fields (durable: org name, member count;
playground: nickname, remaining time, members nicknames).

**Acceptance criteria**:
- [ ] Status enumerates all bound sessions (durable + playground)
- [ ] Missing per-session token: warning to stderr, skip the session,
      don't fail the whole command
- [ ] Mixed durable + playground sessions: output groups them with
      clear section headers
- [ ] No-sessions case: friendly "no sessions bound" message with
      `/jamsesh:jam` pointer
- [ ] `--json` output includes both kinds in a consistent JSON shape

### Story 4: UserPromptSubmit destruction-warning surfacing + auto-loaded SKILL.md update
**Files**:
- `cmd/jamsesh/hookcmd/user_prompt_submit.go` (modify) — surface the
  destruction warning in the "urgent" section of the digest
- `plugins/jamsesh/skills/jamsesh/SKILL.md` (modify) — teach the agent
  about playground semantics + the destruction-warning event response

The `user-prompt-submit` hook calls `GET /api/orgs/{org}/sessions/{id}/digest?since=<seq>`.
The digest response (per session-lifecycle's design) includes a new
event type `playground.destruction_warning` with payload
`{ kind, reason, ends_at, remaining_seconds, session_id }`.

Update the hook's digest-formatting logic:

```go
// Iterate events from the digest response:
for _, event := range digest.Events {
    if event.Kind == "playground.destruction_warning" {
        // Render in the "urgent" section, formatted to grab agent attention:
        urgent.WriteString(fmt.Sprintf(
            "⚠️  Playground session ending in %s due to %s.\n"+
            "   Ends at %s. Run `jamsesh finalize --local` now to keep your work.\n",
            humanDuration(event.Payload.RemainingSeconds),
            event.Payload.Reason, // "idle" or "hard_cap"
            event.Payload.EndsAt.Format(time.RFC3339),
        ))
    }
}
```

The auto-loaded SKILL.md (`plugins/jamsesh/skills/jamsesh/SKILL.md`)
gains a "Playground sessions" section:

```markdown
## Playground sessions

A playground session is an ephemeral anonymous variant of a regular jam
session. It has these distinguishing properties:

- **No persistent identity**: every participant has a server-minted
  pronounceable handle (e.g., `amber-otter`); no email, no account
  outside the session
- **Hard deadlines**: a session is destroyed after either 24 hours since
  creation (`hard_cap`) or 30 minutes of inactivity (`idle_timeout`),
  whichever fires first
- **No claim path**: when the session ends, all its data is destroyed —
  refs, comments, conflict events, the bare repo. The ONLY way to keep
  work is to finalize-out locally BEFORE the destruction trigger fires

When the digest carries a `playground.destruction_warning` event (which
fires ~5 minutes before destruction), surface it prominently to the human
in your reply. Include the `ends_at` time and the imperative to run
`jamsesh finalize --local` if they want to keep the work. The agent has
~5 minutes to push the user to finalize; this is time-sensitive.

Addressing convention: anonymous handles work the same as durable
handles in `@<nickname>` mentions, addressed comments, and conflict-
event recipient fields. No special syntax.
```

**Acceptance criteria**:
- [ ] `playground.destruction_warning` events render in the digest's
      urgent section with a clear timeline
- [ ] Non-playground digests are unchanged (regression test)
- [ ] Auto-loaded SKILL.md has the "Playground sessions" section
- [ ] Reading the SKILL.md, the agent should be able to correctly
      handle a destruction warning in a future turn (verified by a
      simulated digest test — feed the agent a digest with the event,
      verify the LLM reply surfaces the warning correctly; if no LLM
      test infra exists, this is a manual verification step)

## Implementation order

- Sub-wave A: Stories 1, 2, 4 in parallel (3 sub-agents, fits cap)
- Sub-wave B: Story 3 (after Story 2 — needs per-session token storage)

## Risks

- **Migration runs on every binary invocation**: even after migration
  completes, the binary checks the legacy file for the stub. This is
  cheap (one file read) but adds startup latency. Mitigation: the file
  read is <1ms; acceptable. If observability shows it as a hot path,
  cache the stub presence in-memory per invocation.

- **Per-session token without CC session binding**: `jamsesh new`
  writes the per-session token at create time, but `instance_id` is
  written at attach time. Status enumeration finds the token file
  without an instance_id and reports the session as "unbound" — which
  is correct, but the user might find it confusing. Mitigation: status
  output explicitly shows "(unbound — run `/jamsesh:jam join <id>` to
  attach)" for unbound sessions.

- **Skill body content discoverability**: the `/jamsesh:jam` skill body
  is markdown loaded by CC. If the body doesn't mention a specific
  command form (e.g., a future `jamsesh jam fork` subcommand), the
  agent won't know to use it. Mitigation: keep the SKILL.md narrowly
  scoped to the commands this feature introduces (new + join); the
  wave-4 skill-consolidation feature extends the SKILL.md additively
  to add status/fork/mode vocabulary (per the additive hand-off
  contract in skill-consolidation's body).

- **Refresh token left at account-wide path**: refresh tokens still
  use `${CLAUDE_PLUGIN_DATA}/refresh_token` (no per-session counterpart).
  Anonymous sessions don't have refresh tokens (their access tokens
  expire at session end). Durable sessions use refresh tokens to mint
  new access tokens — and the resulting access tokens go to per-session
  storage. The refresh flow code (in `auth/` or `tokens/`) needs a
  small change: after Refresh() returns a new access token, write it
  to the per-session path of the CURRENT session, not the legacy
  `token` path. Otherwise the legacy stub gets overwritten by the
  refresh flow. Implementer fix: locate the refresh-flow callsite
  (probably `cmd/jamsesh/portalclient/refresh.go`); update to write
  the new access token via `state.WriteSessionToken(currentSessionID(), ...)`
  instead of `state.WriteToken(...)`.

## Review (2026-05-24)

**Verdict**: Approve — feature delivered as briefed.

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: All 4 child stories at `stage: done`. Aggregate review: jam-consolidation, status-enumeration, destruction-warning all shipped earlier; bearer-storage landed in land-mode (foundation docs were rolled forward to describe the already-implemented per-session token storage). Plugin surface is now consolidated and per-session tokens are the canonical mechanism. Verification: `go build ./...` and `go test ./...` clean.
