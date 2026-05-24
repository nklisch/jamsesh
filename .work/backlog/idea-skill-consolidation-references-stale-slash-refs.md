---
id: idea-skill-consolidation-references-stale-slash-refs
kind: story
stage: drafting
tags: [bug, documentation]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
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
