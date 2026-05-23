---
id: feature-epic-ephemeral-playground-skill-consolidation
kind: feature
stage: drafting
tags: [plugin]
parent: epic-ephemeral-playground
depends_on: [feature-epic-ephemeral-playground-plugin-skills]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Skill surface consolidation audit + implementation

## Brief

Audits the CC plugin's existing skill surface for consolidation
opportunities where agent intelligence can replace flag-driven branching
across multiple narrow skills, then implements the consolidation per the
audit findings. Generalizes the `/jam` pattern that
`feature-epic-ephemeral-playground-plugin-skills` establishes —
collapsing `/jamsesh:new`, `/jamsesh:playground:new`, and the playground
extension of `/jamsesh:join` into one intent-driven skill — and applies
the same lens to the rest of the existing skill set.

The guiding principle: keep the **binary subcommand surface** rich and
explicit (arguments, flags, deterministic parameter resolution); keep
the **skill surface** thin and intent-driven, letting the agent
translate natural-language requests to subcommand invocations. The
skills exist to teach the agent the binary's contract, not to multiply
that contract by the cartesian product of user intents.

Candidates to audit (initial enumeration; the design pass refines and
ranks):

- `/jamsesh:status` — could fold into `/jam status` (single read entry)
  or stay standalone if the read vs. write distinction matters at the
  skill tier
- `/jamsesh:fork` — natural request shape: "fork from here";
  agent translates to the right `--from <ref>` flag
- `/jamsesh:mode` — natural request shape: "switch this ref to isolated"
  / "rejoin sync"; pure intent → flag translation
- `/jamsesh:finalize` — multi-step (preview, confirm, run); the
  consolidation question is whether `/jam finalize` covers the flow or
  the multi-step is enough motivation to keep a dedicated skill

Likely outcome (subject to design pass): one or two top-level skills
(`/jam` for create/join/status/fork/mode, `/jam-finalize` or
`/jamsesh:finalize` retained for the multi-step finalize flow),
backward-compatible aliases for every deprecated narrow slash to ease
the transition for users who learned the original surface.

## Epic context
- Parent epic: `epic-ephemeral-playground`
- Position in epic: **wave 4** — depends on
  `feature-epic-ephemeral-playground-plugin-skills` for the `/jam`
  pattern to be established before generalizing it. Lands last in
  the epic's implementation sequence.

## Foundation references
- `docs/ARCHITECTURE.md` § Claude Code plugin package — current
  skill directory layout this feature consolidates
- `docs/UX.md` § Status awareness in CC — references the existing
  `/jamsesh:status` slash command; UX.md roll-forward owned by this
  feature's design pass to describe the consolidated surface
- Parent epic's `plugin-skills` feature body — the `/jam` pattern's
  established conventions (intent-driven skill body, agent translates
  to subcommand invocation, binary subcommand surface stays rich)
  this feature generalizes

## Mockups
No UI surface — CLI + skill body work. CC's slash-command UI is the
"surface," which is text/agent-driven; no visual mockup needed.

## Scoping notes (from this feature's promotion from backlog)

This was originally parked as `idea-skill-surface-consolidation-audit`
during the `plugin-skills` `--only-questions` round, when the user
redirected from the planned three-new-skills approach to a single
`/jam` skill and explicitly called out the broader audit pattern. At
the user's direction, the audit was folded into this epic as a 7th
child feature rather than scoped as a standalone follow-up — the work
is interrelated (the `/jam` pattern is already being established by
`plugin-skills`; this feature generalizes it across the existing skill
set) and the deprecation story is cleanest if consolidated.

The folded-in scope shifts the epic's wave shape: wave 4 is now this
single feature, landing after `plugin-skills` (wave 3). Total epic
size grows from 6 features to 7. The CLI binary's subcommand surface
remains the responsibility of the existing wave-1/wave-2 features
(`cli-first-creation` and `session-lifecycle`); this feature touches
only the **skill** layer at `plugins/jamsesh/skills/`.

## Design decisions

Locked at `--only-questions` time. Feature-design Phase 5 inherits these
as fixed input.

### Pre-launch reality (overarching)

Jamsesh has not yet shipped a public release with the existing skill
surface in use by end users — every decision below is made on the
basis that **there are no installed-base users to migrate**. Backward-
compatible aliases, deprecation windows, picker-visibility hygiene, and
gradual cutover patterns are all moot. The consolidation is a
"rip-the-bandaid" rework that lands atomically alongside the playground
epic; the canonical slash surface emerges in the same release that
ships this epic.

If this assumption ever flips (post-launch with real users), this
feature's decisions are explicitly **not durable for that world** — a
revisit would need to introduce aliases, deprecation hints, and a
deprecation window. Documented here so a future maintainer understands
why this feature looks the way it does.

### Decisions

- **Consolidation shape**: most existing slashes fold into
  `/jamsesh:jam`; `/jamsesh:finalize` stays standalone. The
  `/jamsesh:jam` skill body becomes the single intent-driven entry
  for status, fork, mode, plus the create / join paths inherited from
  `plugin-skills` — agent reads the user's natural-language request
  (e.g., "switch this ref to isolated", "fork from amber-otter's tip",
  "what's the session state?") and invokes the right binary
  subcommand (`jamsesh status`, `jamsesh fork --from <ref>`,
  `jamsesh mode isolated`). `/jamsesh:finalize` keeps its own slash
  because the multi-step gravity (preview → confirm → run-locally)
  and the local-vs-portal split make it a genuinely different shape
  that doesn't compress cleanly into a single intent-driven entry.
  The slash form `/jamsesh:jam` is pinned (CC plugin-namespace
  convention; `/jam` was a working name and not the canonical slash).

- **Old slashes deleted outright, no aliases**: `/jamsesh:status`,
  `/jamsesh:fork`, `/jamsesh:mode`, and `/jamsesh:join` (the last
  inherited from `plugin-skills`' original decision that planned an
  alias) are removed entirely in the same release that ships
  `/jamsesh:jam`. No SKILL.md left behind, no deprecated-alias
  forwarding, no deprecation hint text. Pre-launch reality —
  there's no installed-base muscle memory to preserve.

- **No deprecation window**: the cut is atomic with the release. v0.4.0
  (or whichever release ships this epic) ships `/jamsesh:jam` +
  `/jamsesh:finalize` as the only top-level slashes for the plugin.

- **Discoverability**: trivially clean post-cut — the `/` slash-picker
  shows `/jamsesh:jam` and `/jamsesh:finalize` from this plugin, and
  nothing else. No picker-visibility flag work needed (no aliases to
  hide).

### Scope of work this resolves

In implementation terms, this feature deletes the following files:

- `plugins/jamsesh/skills/status/SKILL.md`
- `plugins/jamsesh/skills/fork/SKILL.md`
- `plugins/jamsesh/skills/mode/SKILL.md`
- `plugins/jamsesh/skills/join/SKILL.md` (inherited deletion from
  `plugin-skills`' updated decision — see that feature's body)

And creates / updates:

- `plugins/jamsesh/skills/jam/SKILL.md` (new — single intent-driven
  skill covering status / fork / mode / create / join surfaces; this
  feature owns the body, `plugin-skills` owns its creation as part of
  wave 3 and this feature extends the body in wave 4)
- `plugins/jamsesh/skills/finalize/SKILL.md` (existing — body may need
  light updates to align with the new mental model, but the skill
  stays)
- `plugins/jamsesh/skills/jamsesh/SKILL.md` (auto-loaded — updated to
  teach the agent the consolidated pattern: `/jamsesh:jam` for almost
  everything, `/jamsesh:finalize` for the finalize multi-step)

The exact body for `plugins/jamsesh/skills/jam/SKILL.md` — the
subcommand vocabulary the agent is taught, the intent-mapping
examples, the ambiguity-handling rules — is content for this feature's
full design pass (Phase 5).
