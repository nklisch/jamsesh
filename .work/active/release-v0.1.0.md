---
id: release-v0.1.0
kind: release
stage: quality-gate
tags: []
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Release v0.1.0

Initial release. Bundles the jamsesh foundation: portal backend (auth, sessions,
git smart-HTTP, MCP, websocket gateway, auto-merger), Claude Code plugin
(binary, hooks, session commands, packaging), portal UI (foundation, design
system, session list/view, ref actions, comments), finalize flow,
e2e-test infrastructure and coverage, cloud-native deploy (routing layer,
hydration handoff, lease fencing, object storage sync, operational polish),
e2e CND coverage, and distribution (build pipeline, docker, marketplace,
self-host docs).

## Bound items

271 items total — 11 epics, 59 features, 201 stories. Canonical list via
`.work/bin/work-view --release v0.1.0 --paths`.

### Epics

- epic-portal-foundation
- epic-portal-git
- epic-auto-merger
- epic-portal-api
- epic-cc-plugin
- epic-portal-ui
- epic-finalize-flow
- epic-cloud-native-deploy
- epic-e2e-tests
- epic-e2e-cnd-coverage
- epic-distribution

## Gate runs

- **gate-security** (2026-05-18) — 15 findings (2 Critical, 3 High, 6 Medium, 4 Low)
  - 11 items into `.work/active/stories/` (gate-security-*) — Critical/High at `implementing`, Medium at `drafting`
  - 4 items into `.work/backlog/` (Low severity)
- **gate-tests** (2026-05-18) — 25 findings (7 Critical, 7 High, 9 Medium, 2 Low)
  - 23 items into `.work/active/stories/` (gate-tests-*) — Critical/High at `implementing`, Medium at `drafting`
  - 2 items into `.work/backlog/` (Low severity)
  - 1 tautological test flagged (`TestGitHub_Exchange_PicksPrimaryVerifiedEmail`) — to be reworked under `gate-tests-github-oauth-unverified-email`
- **gate-cruft** (2026-05-18) — 16 findings (13 High, 3 Medium, 0 Low)
  - 16 items into `.work/active/stories/` (gate-cruft-*) — High at `implementing`, Medium at `drafting`
- **gate-docs** (2026-05-18) — 15 findings (13 High foundation-doc drift, 1 High repo-skill drift, 1 Medium, 1 changelog-gap no-op)
  - 14 items into `.work/active/stories/` (gate-docs-*) — High at `implementing`, Medium at `drafting`
  - Changelog gap (Finding 15) addressed automatically by release-deploy phase 5.5
- **gate-patterns** (2026-05-18) — 8 patterns extracted (0 inconsistencies, no pre-existing catalog)
  - 8 pattern files written to `.claude/skills/patterns/`
  - `.claude/rules/patterns.md` index created; `.claude/skills/patterns/SKILL.md` catalog created
  - Tracking item `gate-patterns-v0.1.0` at `stage: done`
