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
