---
id: story-epic-ephemeral-playground-plugin-skills-destruction-warning
kind: story
stage: implementing
tags: [plugin, playground]
parent: feature-epic-ephemeral-playground-plugin-skills
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# UserPromptSubmit destruction-warning surfacing + auto-loaded SKILL.md

## Scope

Story 4 of the parent feature. Two coordinated changes:

1. **UserPromptSubmit hook** — recognizes `playground.destruction_warning`
   events in the digest response and surfaces them in the "urgent"
   section of the formatted `additionalContext` block
2. **Auto-loaded `plugins/jamsesh/skills/jamsesh/SKILL.md`** —
   teaches the agent about playground semantics + the
   destruction-warning response protocol

These two changes ship together because they're a coordinated contract:
the hook surfaces the event in a specific format, and the SKILL.md
teaches the agent to recognize and respond to that format.

Full design in the parent feature body's "Story 4" section.

## Files delivered

- `cmd/jamsesh/hookcmd/user_prompt_submit.go` (modify) — recognize and
  surface `playground.destruction_warning` events
- `cmd/jamsesh/hookcmd/user_prompt_submit_test.go` (extend) — test
  the new event-surfacing path with a fixture digest containing the
  warning event
- `plugins/jamsesh/skills/jamsesh/SKILL.md` (modify) — append the
  "Playground sessions" section per the parent feature body

## Acceptance criteria

See parent feature body's "Story 4 acceptance criteria" section.

## Notes

- The event payload shape `{ kind, reason, ends_at, remaining_seconds,
  session_id }` is owned by the session-lifecycle feature's
  rest-endpoints + destruction stories. Import the generated TS/Go
  types from the OpenAPI codegen rather than redefining inline.
- Non-playground digests must be unchanged (regression test in
  user_prompt_submit_test.go).
- The auto-loaded SKILL.md edit is APPEND, not REPLACE — the existing
  body content stays intact; the new "Playground sessions" section is
  inserted at an appropriate place (probably after "Multi-agent per
  human" or wherever the existing body discusses session semantics).
- The SKILL.md edit IS expected to be touched again by the wave-4
  skill-consolidation feature (which generalizes the consolidation
  pattern). Coordinate by leaving clear section boundaries.

## Cross-story note

This story is independent (`depends_on: []`). The two changes are
coordinated but don't require sequencing with the other plugin-skills
stories. Can run in sub-wave A alongside Stories 1 and 2.
