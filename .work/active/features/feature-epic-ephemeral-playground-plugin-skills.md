---
id: feature-epic-ephemeral-playground-plugin-skills
kind: feature
stage: drafting
tags: [plugin, playground]
parent: epic-ephemeral-playground
depends_on: [feature-epic-ephemeral-playground-cli-first-creation, feature-epic-ephemeral-playground-session-lifecycle]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

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
