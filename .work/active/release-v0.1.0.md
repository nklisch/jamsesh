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
