---
id: feature-cli-jam-open-in-browser-skill-and-docs
kind: story
stage: implementing
tags: [plugin, documentation]
parent: feature-cli-jam-open-in-browser
depends_on: [feature-cli-jam-open-in-browser-cli-open-flag]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Skill offer-to-open + UX.md roll-forward

Implements **Unit 3** of `feature-cli-jam-open-in-browser`. See the feature body.

## Scope

- `plugins/jamsesh/skills/jam/SKILL.md` (repo source — the plugin cache is the
  installed copy):
  - Document `--open` under "Optional flags for `jam new`" and under "For
    `jam join`".
  - Add a terse "Opening in the browser" note: the agent should **offer** to open
    the session when `jam` is invoked (fold the offer into the org/goal questions
    it already asks) and pass `--open` on assent. The CLI itself never prompts.
- `docs/UX.md`: reflect the `--open` affordance in the create flow and the
  playground create flow (rolling-foundation — describe the present; no
  "previously" prose).

## Acceptance criteria

- [ ] `SKILL.md` documents `--open` for both `jam new` and `jam join` and the
      agent-offer behavior; describes no interactive CLI prompt.
- [ ] `docs/UX.md` mentions `--open` in the relevant create/join flow(s).
- [ ] Copy matches the shipped flag behavior from the `cli-open-flag` story.
