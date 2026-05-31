---
id: feature-cli-jam-open-in-browser-skill-and-docs
kind: story
stage: done
tags: [plugin, documentation]
parent: feature-cli-jam-open-in-browser
depends_on: [feature-cli-jam-open-in-browser-cli-open-flag]
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
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
- `docs/UX.md`: reflect the `--open` affordance in the durable create flow, the
  playground create flow, AND the durable join flow (rolling-foundation —
  describe the present; no "previously" prose). Document the playground
  `new --open` behavior: the opened join page mints a fresh browser participant
  via `JoinerPicker`; it does not resume the CLI's anonymous identity.

## Acceptance criteria

- [x] `SKILL.md` documents `--open` for both `jam new` and `jam join` and the
      agent-offer behavior; describes no interactive CLI prompt.
- [x] `docs/UX.md` mentions `--open` in the durable create, playground create,
      and durable join flows, including the playground "fresh participant" note.
- [x] Copy matches the shipped flag behavior from the `cli-open-flag` story.

## Implementation notes

Files changed:

- `plugins/jamsesh/skills/jam/SKILL.md`: added `--open` bullet under
  "Optional flags for `jam new`"; added `--open` bullet under "For `jam join`";
  added new "Opening in the browser" subsection instructing the agent to offer
  `--open` when invoking `jam`, fold the offer into existing questions, and
  notes that playground `new --open` mints a fresh browser participant via
  `JoinerPicker` (does not resume CLI anonymous identity).

- `docs/UX.md`: added `--open` to the durable create flow CLI example block;
  added step 5 to the durable create on-success list (browser open + graceful
  degradation); added `--open` note to the playground create flow (step 3)
  including the fresh-participant caveat; added step 6 to the durable join flow
  (browser open + graceful degradation, renumbered the final "start prompting"
  step to 7). No legacy prose; doc describes the present.
