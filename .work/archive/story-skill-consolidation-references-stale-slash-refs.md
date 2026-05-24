---
id: story-skill-consolidation-references-stale-slash-refs
kind: story
stage: done
tags: [bug, documentation]
parent: feature-epic-ephemeral-playground-skill-consolidation
depends_on: []
release_binding: v0.4.0
gate_origin: null
created: 2026-05-23
updated: 2026-05-24
---

# Stale slash references in primer's reference files

## Brief

The skill-consolidation feature deleted `/jamsesh:status`,
`/jamsesh:fork`, `/jamsesh:mode` SKILL.md files but two reference
files inside `plugins/jamsesh/skills/jamsesh/references/` still
mention them.

## Findings

`plugins/jamsesh/skills/jamsesh/references/mcp-tools.md`:
- **Line 8**: "from your injected context or `/jamsesh:status`."
- **Line 68**: "Equivalent to `/jamsesh:fork`. Prefer the MCP tool..."

`plugins/jamsesh/skills/jamsesh/references/conflicts.md`:
- **Line 42**: "`/jamsesh:status` if unsure."
- **Line 102**: "Consider `/jamsesh:mode isolated` to stop the
  auto-merger from..."
- **Line 117**: "Run `/jamsesh:status` to verify..."

## Acceptance criteria

- [ ] All five references rewritten to either invoke the binary
      subcommand (`jamsesh status`, etc.) or point to `/jamsesh:jam`
      with the appropriate intent phrasing
- [ ] `grep -rn "/jamsesh:status\|/jamsesh:fork\|/jamsesh:mode" plugins/jamsesh/skills/jamsesh/references/`
      returns nothing

## Why important (not blocker)

Reference files are loaded on-demand by the agent (not auto-loaded
like SKILL.md). Lower blast radius than the primer itself — agents
only see these when they pull them in for deep-dive lookups. Still
worth fixing, but doesn't block the feature advancing once the primer
and foundation docs are corrected.

## Notes

Could be batched with `story-skill-consolidation-primer-stale-slash-refs`
into a single doc-cleanup PR.

## Implementation notes

Fixed 5 stale slash references across the two reference files, following the
same pattern established by the sibling story:

`plugins/jamsesh/skills/jamsesh/references/conflicts.md`:
- **Line 42** (`/jamsesh:status` → `jamsesh status`): "run if unsure about
  tracking ref name" context — binary invocation.
- **Line 102** (`/jamsesh:mode isolated` → `jamsesh mode isolated` via
  `/jamsesh:jam`): conflict-pile-up advisory context.
- **Line 117** (`/jamsesh:status` → `jamsesh status`): "verify stale event"
  context — binary invocation.

`plugins/jamsesh/skills/jamsesh/references/mcp-tools.md`:
- **Line 8** (`/jamsesh:status` → `jamsesh status`): session_id lookup note
  in the file header — binary invocation.
- **Line 68** (`/jamsesh:fork` → `fork action via /jamsesh:jam`): fork MCP
  tool equivalence note updated to point to the consolidated skill.

Verification: `grep -rn "/jamsesh:status\|/jamsesh:fork\|/jamsesh:mode" plugins/jamsesh/skills/jamsesh/references/` returns nothing.

## Review (2026-05-24)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Implementation matches the design; verification passes (Go: `go build` + `go test ./...` clean; frontend: `npm run check` 0 errors, `npm run test` 635/635, `npm run build` clean). Implementation notes accurately document what landed, including any agent decisions or land-mode confirmations.
