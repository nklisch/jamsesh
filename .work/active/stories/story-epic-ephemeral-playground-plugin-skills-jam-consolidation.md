---
id: story-epic-ephemeral-playground-plugin-skills-jam-consolidation
kind: story
stage: review
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

## Implementation notes

**Design discovery: urfave/cli v3 parent-pointer aliasing.**
urfave/cli v3 sets `subCmd.parent = cmd` on every command during setup
(see `command_setup.go`). Registering the same `*cli.Command` instance
under two parents would cause the parent pointer to be overwritten by
whichever parent processes it last, silently breaking help text and flag
inheritance for the earlier registration.

**Resolution:** `JamCommand()` calls `sessioncmd.NewCommand()` and
`sessioncmd.JoinCommand()` again to obtain fresh instances. These
factory functions return `&cli.Command{...}` each time, so the resulting
values are pointer-distinct but semantically identical (same Name, Flags,
Action). This is safe: the two registrations share no mutable state.
`TestJamCommand_IndependentInstances` explicitly asserts pointer
inequality to prevent future regressions.

**Files delivered:**
- `plugins/jamsesh/skills/jam/SKILL.md` — intent-vocabulary skill body
- `cmd/jamsesh/jamcmd/jam.go` — jam parent subcommand dispatcher
- `cmd/jamsesh/jamcmd/jam_test.go` — 5 tests covering help structure,
  dispatch errors, top-level preservation, and pointer independence
- `cmd/jamsesh/main.go` — registered `jamcmd.JamCommand()`
- `plugins/jamsesh/skills/join/SKILL.md` — deleted via `git rm`

**Verification:**
- `go build ./cmd/jamsesh/...` — passes
- `go test ./cmd/jamsesh/jamcmd/...` — passes (ok 0.003s)
- `go vet ./...` — passes
- `jamsesh --help` — shows `jam` alongside all existing commands
- `jamsesh jam --help` — lists `new` and `join` sub-subcommands
- `ls plugins/jamsesh/skills/` — no `join/` directory
