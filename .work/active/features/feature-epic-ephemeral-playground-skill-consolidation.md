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
