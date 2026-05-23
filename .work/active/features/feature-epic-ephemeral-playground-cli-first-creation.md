---
id: feature-epic-ephemeral-playground-cli-first-creation
kind: feature
stage: drafting
tags: [plugin, portal]
parent: epic-ephemeral-playground
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# CLI-first session creation

## Brief

Adds the `jamsesh new` Go subcommand that drives session creation
end-to-end from the user's local checkout. The subcommand interactively
prompts for goal, writable scope, default mode, and org (when the user
has multiple), with flag overrides for every prompt for non-interactive
use (`--org`, `--goal`, `--scope`, `--mode`, `--invite`). After the
portal accepts the create call, it pushes the local HEAD as the session
base ref and writes the per-session state file under
`${CLAUDE_PLUGIN_DATA}/sessions/<session-id>/`.

This feature is the foundation that unifies session creation onto a
CLI-first pattern (replacing the current "create-in-portal, then
join-from-CLI" two-step). It is intentionally durable-only — no
playground concerns. The `session-lifecycle` sibling feature layers
`--playground` flag handling on top once anon bearers and the reserved
org exist.

The portal's session-create REST handler may need a small refactor: the
current shape assumes the join step seeds the base ref, while the
CLI-first flow expects to push HEAD immediately after creation. Confirm
the handler accepts a session with `base_sha: NULL` and stamps it on the
first push (the schema already allows nullable base_sha, so this may be
a no-op).

## Epic context
- Parent epic: `epic-ephemeral-playground`
- Position in epic: **wave 1 foundation** — no dependencies; required by
  `session-lifecycle` (wave 2) for the `--playground` flag handling and
  by `plugin-skills` (wave 3) for the `/jamsesh:new` skill.

## Foundation references
- `docs/SPEC.md` § Lifecycle — current durable creation flow
- `docs/ARCHITECTURE.md` § The `jamsesh` binary — existing subcommand
  surface that `jamsesh new` slots into
- `docs/UX.md` § Flow: creating a session — the flow this feature
  reworks; UX.md roll-forward is owned by this feature's design pass

## Mockups
- Inherits parent epic flow:
  `.mockups/flows/playground-onboarding/index.html` (step 02 is the
  representative CLI-output state). For durable creation, the CLI output
  is parallel: same shape, different identity (real account vs.
  anonymous handle) and different lifecycle indicators (no countdown).
  No additional mocks needed at the feature tier.

## Design decisions

Locked at `--only-questions` time. Feature-design Phase 5 inherits these
as fixed input.

### Overarching: agent-primary mental model

The primary user of `jamsesh new` is **a human in a Claude Code session
whose agent invokes the binary on their behalf**, not a human typing
in bash directly. Every UX decision below has two arms:

- **Agent path** (primary): the agent reads the human's natural-language
  request ("spin up a session for the auth refresh, scope to `docs/auth/**`,
  goal: tighten the OAuth callback contract"), maps that to explicit
  flags, and invokes `jamsesh new --org <id> --goal '<text>'
  --scope '<glob>' --mode <sync|isolated>`. The agent never sees stdin
  prompts because the CC bash tool doesn't drive a TTY; flags are the
  only deterministic interface. If params are missing, the agent
  asks the human inside CC, not via the binary's prompt.
- **Direct-human path** (fallback): a developer in their own terminal
  runs `jamsesh new` directly — the binary detects TTY, drops into
  interactive prompts. Useful for ops, debugging, scripting; not the
  primary case.

This framing belongs in the `/jamsesh:new` SKILL.md body (owned by the
`plugin-skills` sibling feature) so the agent is taught the parameter
mapping and the "ask in CC, never via stdin" rule.

### Decisions

- **Org picker when user has multiple orgs**: interactive picker with
  the most-recently-used org pre-selected (TTY only); `--org <id>` flag
  required when stdin is not a TTY (i.e. agent invocation). The agent
  pattern: parse the human's request for the org reference, error out
  early if ambiguous (the skill body teaches "if the human's request
  doesn't pin an org, ask them which one"). Hard-fails on non-TTY
  without `--org` rather than silently picking — silent picks risk
  agent-driven creates in the wrong tenant.

- **Invite handling**: both inline `--invite alice@x,bob@x` flag on
  `jamsesh new` AND a separate `jamsesh invite <session-id> <emails>`
  subcommand. The agent uses `--invite` when the human's create request
  mentions collaborators in one breath; the separate subcommand
  exists for follow-up adds. Both produce identical invite rows.

- **Post-create HEAD-push failure**: the session row stays live with
  `base_sha: NULL`; the CLI prints a clear retry command
  (`git push <session-remote> HEAD:jam/<session-id>/base`). The
  `/jamsesh:new` skill body instructs the agent to: (1) retry the push
  automatically once, (2) on second failure, surface the error to the
  human with the explicit retry command. The portal's `pre-receive`
  validates the first push and stamps `base_sha` then — schema already
  allows nullable `base_sha`, so this is consistent with existing
  semantics. No transactional packfile-on-create refactor.

- **Required vs optional fields at creation**: goal and writable scope
  are **optional with defaults** — goal defaults to empty (settable
  later via the portal UI or a planned `jamsesh edit`), writable scope
  defaults to `**` (permissive). Interactive prompts ask but `enter`
  accepts defaults; flags (`--goal`, `--scope`, `--mode`) override. Same
  agent-friendliness: when the human's create request omits a field,
  the agent passes the flag-default value (or asks if scope-default
  `**` is too permissive for the human's intent) rather than blocking
  on a required field.
