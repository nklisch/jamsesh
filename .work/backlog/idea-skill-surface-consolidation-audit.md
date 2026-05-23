---
id: idea-skill-surface-consolidation-audit
created: 2026-05-23
tags: [plugin]
---

Audit the CC plugin's skill surface for consolidation opportunities where
agent intelligence can replace flag-driven branching across multiple
narrow skills. Surfaced during `/agile-workflow:feature-design
--only-questions` on `feature-epic-ephemeral-playground-plugin-skills`,
where the user redirected from "add `/jamsesh:new` +
`/jamsesh:playground:new` + extend `/jamsesh:join`" to "collapse them
into a single `/jam` skill and let the agent interpret intent." The
broader pattern likely applies to other existing skills.

Candidates to audit (initial enumeration, expand during scoping):

- `/jamsesh:status` — could fold into `/jam status` or stay standalone
  if the read vs. write distinction matters
- `/jamsesh:fork` — short-form: "fork from here"; agent translates to
  the right `--from <ref>` flag
- `/jamsesh:mode` — "switch this ref to isolated" / "rejoin sync" —
  pure intent translation
- `/jamsesh:finalize` — multi-step (preview, confirm, run); could
  remain standalone or fold into `/jam finalize`

The principle: keep the **binary subcommand surface** rich and explicit
(arguments, flags, deterministic parameter resolution); keep the **skill
surface** thin and intent-driven, letting the agent translate
natural-language requests to subcommand invocations. The skills exist to
teach the agent the binary's contract, not to multiply that contract by
the cartesian product of user intents.

When scoping this idea: weigh the discoverability cost of fewer named
skills vs. the agent-friendliness benefit; look for natural skill
groupings; consider whether existing users of the narrow slashes need
backward-compatible aliases.
