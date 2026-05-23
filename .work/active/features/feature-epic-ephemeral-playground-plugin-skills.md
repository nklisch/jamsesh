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
