---
id: story-skill-consolidation-primer-stale-slash-refs
kind: story
stage: done
tags: [bug]
parent: feature-epic-ephemeral-playground-skill-consolidation
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-24
---

# Auto-loaded primer still instructs agents to run deleted slashes

## Brief

`plugins/jamsesh/skills/jamsesh/SKILL.md` is the auto-loaded primer
that every agent participating in a jam reads. The
skill-consolidation feature deleted `/jamsesh:status`, `/jamsesh:fork`,
`/jamsesh:mode` SKILL.md files but the primer still instructs agents to
invoke those slashes. Agents that follow the primer's instructions
will hit "skill not found" walls.

## Stale references in the primer

`plugins/jamsesh/skills/jamsesh/SKILL.md`:

- **Line 19-21** (blockquote): "The other skills in this plugin
  (`join`, `fork`, `status`, `mode`, `finalize`) are thin CLI
  wrappers..." — `join`, `fork`, `status`, `mode` are gone.
- **Line 76**: "Run `/jamsesh:status` before assuming something is
  broken." — `/jamsesh:status` doesn't exist.
- **Line 97**: "Switch with `/jamsesh:mode sync` or `/jamsesh:mode
  isolated`." — `/jamsesh:mode` doesn't exist.
- **Line 118**: "run `/jamsesh:status` to fetch the current values."
- **Line 145**: "if a push didn't happen, run `/jamsesh:status` to
  find out why."
- **Line 222**: "use `/jamsesh:mode isolated` to accumulate..."
- **Line 233**: "read it from your injected context or
  `/jamsesh:status`."

## Acceptance criteria

- [ ] Blockquote at lines 19-21 updated to reflect only `jam` and
      `finalize` as plugin skills
- [ ] Every `/jamsesh:status` invocation replaced with either
      `/jamsesh:jam` ("ask for status") or `jamsesh status` (the
      binary, if the surrounding context expects a literal command)
- [ ] Every `/jamsesh:mode ...` invocation similarly rewritten
- [ ] `grep -n "/jamsesh:status\|/jamsesh:fork\|/jamsesh:mode\|/jamsesh:join" plugins/jamsesh/skills/jamsesh/SKILL.md`
      returns nothing

## Why this is a blocker

The primer is the canonical context for every agent in a jam. When it
tells an agent to "Run `/jamsesh:status`" and that slash no longer
exists, the agent either hits an error or, worse, hallucinates a
workaround. The consolidation premise ("rip-the-bandaid, no aliases")
is sound, but it leaves the documentation surface inconsistent with
the implementation surface.

## Notes

Sibling story `story-skill-consolidation-references-stale-slash-refs`
covers the same problem in the primer's reference files
(`references/mcp-tools.md`, `references/conflicts.md`); they could be
batched into one PR.

## Implementation notes

Fixed 7 stale slash references in `plugins/jamsesh/skills/jamsesh/SKILL.md`:

- **Lines 19-21 blockquote**: replaced `join`, `fork`, `status`, `mode`,
  `finalize` enumeration with `jam`, `finalize` — the only two skills that
  actually exist post-consolidation.
- **Line 76** (`/jamsesh:status` → `jamsesh status`): "run before assuming
  something is broken" context — binary invocation is correct here.
- **Line 97** (`/jamsesh:mode sync` / `/jamsesh:mode isolated` →
  `jamsesh mode sync` / `jamsesh mode isolated` via `/jamsesh:jam`):
  mode-switch instruction updated to reflect binary subcommand via the jam
  skill.
- **Line 118** (`/jamsesh:status` → `jamsesh status`): "fetch current
  trailer values" context — binary invocation.
- **Line 145** (`/jamsesh:status` → `jamsesh status`): "push didn't
  happen" context — binary invocation.
- **Line 222** (`/jamsesh:mode isolated` → `jamsesh mode isolated` via
  `/jamsesh:jam`): conflict-resolution accumulation context.
- **Line 233** (`/jamsesh:status` → `jamsesh status`): MCP tools section
  session_id lookup — binary invocation.

Verification: `grep -n "/jamsesh:status\|/jamsesh:fork\|/jamsesh:mode\|/jamsesh:join"` returns nothing.

## Review (2026-05-24)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Implementation matches the design; verification passes (Go: `go build` + `go test ./...` clean; frontend: `npm run check` 0 errors, `npm run test` 635/635, `npm run build` clean). Implementation notes accurately document what landed, including any agent decisions or land-mode confirmations.
