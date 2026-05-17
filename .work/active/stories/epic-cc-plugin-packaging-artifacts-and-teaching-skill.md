---
id: epic-cc-plugin-packaging-artifacts-and-teaching-skill
kind: story
stage: implementing
tags: [plugin, documentation]
parent: epic-cc-plugin-packaging
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# CC Plugin — Packaging Artifacts and Teaching Skill

## Scope

Author all the static plugin artifacts: manifest, hook
registration, MCP config, five user-facing slash-command skills,
and the auto-loaded teaching skill. After this story, the plugin
package directory tree is complete and ready for the marketplace
publishing pipeline to consume.

## Units delivered

- `.claude-plugin/plugin.json` per Unit 1
- `hooks/hooks.json` per Unit 2
- `.mcp.json` per Unit 3
- `skills/jamsesh/SKILL.md` (auto-loaded teaching skill) per Unit 4
- `skills/join/SKILL.md`, `skills/status/SKILL.md`,
  `skills/fork/SKILL.md`, `skills/mode/SKILL.md`,
  `skills/finalize/SKILL.md` per Unit 5

## Acceptance Criteria

- [ ] `jq . < .claude-plugin/plugin.json` parses without error;
      required fields present
- [ ] `jq . < hooks/hooks.json` parses; all 6 hook entries
      registered
- [ ] `jq . < .mcp.json` parses; jamsesh server registered with
      `headersHelper`
- [ ] Each `skills/*/SKILL.md` has valid YAML frontmatter (`name`,
      `description` at minimum)
- [ ] `wc -w < skills/jamsesh/SKILL.md` ≤ 2500
- [ ] Teaching skill covers all 8 sections listed in parent feature
      body Unit 4
- [ ] Teaching skill references `docs/VISION.md`, `docs/PROTOCOL.md`,
      `docs/UX.md` for deeper reading (rather than duplicating
      their content)

## Notes

- The 5 user-facing skills are intentionally minimal. Their job
  is to be a discoverable entry point — the real work happens in
  the `jamsesh` binary subcommands (in `session-commands` and
  `finalize-command` features).
- The teaching skill is THE highest-leverage artifact in the
  package — it loads into every agent turn. Optimize for
  operational clarity, not exhaustiveness.
- Plugin install path: this directory tree is what gets packed by
  `epic-distribution-marketplace` and shipped to the marketplace
  repo on every tag.
- CC plugin schema specifics (e.g., exact frontmatter keys for
  auto-load triggers) should be verified against the current CC
  plugin docs. If a key name differs from what's sketched in the
  parent feature body, prefer the verified key.
