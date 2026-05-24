---
id: story-epic-ephemeral-playground-plugin-skills-jam-consolidation
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

# `/jamsesh:jam` skill + `jam` binary subcommand dispatcher

## Scope

Story 1 of the parent feature. Creates the single intent-driven
`/jamsesh:jam` SKILL.md, adds the `jam` parent subcommand to the binary
that dispatches to `new` and `join` sub-subcommands, and deletes
`plugins/jamsesh/skills/join/SKILL.md` per the pre-launch-reality
no-aliases decision.

Full design in the parent feature body's "Story 1" section.

## Files delivered

- `plugins/jamsesh/skills/jam/SKILL.md` (new)
- `cmd/jamsesh/jamcmd/jam.go` (new)
- `cmd/jamsesh/jamcmd/jam_test.go` (new)
- `cmd/jamsesh/main.go` (modify) — register JamCommand()
- `plugins/jamsesh/skills/join/SKILL.md` (delete)

## Acceptance criteria

See parent feature body's "Story 1 acceptance criteria" section.

## Notes

- The top-level `jamsesh new` and `jamsesh join` binary subcommands
  STAY (locked decision: binary surface stays rich; skill surface
  gets thin). `jamsesh jam new` and `jamsesh new` are equivalent.
- The wave-4 skill-consolidation feature extends the same SKILL.md
  additively. Per its hand-off contract, this story's writes are
  appended-to, not replaced, by wave 4. Read the wave-4 story body
  before any future edits.
- The SKILL.md body content (see parent feature body) teaches the
  agent the intent vocabulary AND the destruction-warning response
  protocol — keep both sections present.
