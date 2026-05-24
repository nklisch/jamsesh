---
id: story-spec-discipline-pattern-doc
kind: story
stage: review
tags: [documentation]
parent: feature-spec-discipline
depends_on: [story-spec-discipline-drift-ci-check]
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Document the spec-driven-event-types pattern + index it in patterns rules

## Brief

Codify the spec-first event-type discipline as a project pattern,
matching the format of every other pattern file in
`.claude/skills/patterns/`. Index it in `.claude/rules/patterns.md`
so the rule is discoverable by all agents auto-loading the patterns
skill.

The convention itself already exists per `docs/SPEC.md` (the YAML is
the single source of truth, codegen produces typed consumers).
What's missing is a pattern entry that makes the rule visible and
links the CI guardrail from the sibling story.

## Current state

No pattern file documents the spec-first event-type rule. New
contributors and agents discover it only by reading `docs/SPEC.md`
end-to-end. The CI drift check from the sibling story enforces the
rule but doesn't *explain* it.

## Target state

Two markdown files:

1. **`.claude/skills/patterns/spec-driven-event-types.md`** —
   the new pattern entry. Shape matches the existing pattern files
   (read 2-3 existing entries first to mirror format). Sections:
   - Pattern name + one-line summary
   - When it applies (any new WS event type added to the server)
   - The rule:
     - The YAML is authoritative; every Go-emitted event-type
       string MUST appear in the `EventEnvelope.type` enum
     - Adding a new event requires: schema definition, enum entry,
       `oneOf` branch, `discriminator.mapping` entry, then
       `make generate`
     - The CI test
       `TestEventTypeConstants_MatchOpenAPIYAML` (from sibling
       story) enforces both directions
   - Concrete example (one or two existing events with file
     references)
   - Failure mode (what happens when the rule is violated —
     reference the autopilot finding that prompted the pattern)

2. **`.claude/rules/patterns.md`** update — add a one-line entry to
   the pattern index, pointing at the new file. Insert in
   alphabetical position.

## Acceptance criteria

- [ ] `.claude/skills/patterns/spec-driven-event-types.md` exists,
      ~80-150 lines, matches the format of the sibling pattern
      files.
- [ ] `.claude/rules/patterns.md` includes an entry for the new
      pattern in alphabetical position.
- [ ] `.claude/skills/patterns/SKILL.md` includes the pattern in
      its list (if it has one — check the existing format first).
- [ ] `docs/SPEC.md`'s "Generated contracts" section gets a
      one-line cross-reference to the new pattern (rolling
      foundation: foundation docs describe the system as it is now
      with the CI check in place).

## Risk

**Very low.** Documentation only.

## Rollback

`git revert` the implementation commit.

## Notes

Depending on `story-spec-discipline-drift-ci-check` first ensures
the pattern doc references the *actual implemented test*, not a
hypothetical one.

## Implementation notes

- **Pattern file**: `.claude/skills/patterns/spec-driven-event-types.md`
  created at 142 lines. Format mirrored from `tx-emit-then-fanout.md` and
  `per-package-clock-interface.md` (H1 title + one-line summary paragraph,
  Rationale, rule enumeration, Examples with file references, CI test section,
  Failure mode with literal test output, Resolution flow, When to Use / When
  NOT to Use, Common Violations).
- **Index**: `.claude/rules/patterns.md` — `spec-driven-event-types` entry
  inserted in alphabetical position between `snippet-children-component` and
  `view-state-union-machine`.
- **SKILL.md**: `.claude/skills/patterns/SKILL.md` — `spec-driven-event-types.md`
  entry added in same alphabetical position.
- **SPEC.md cross-reference**: four-line bullet added to the "Generated contracts"
  section immediately after the `make generate` build-wire bullet, naming
  `events.AllTypes`, the test, and the pattern file path.
