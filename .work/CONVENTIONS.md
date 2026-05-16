# Project Conventions

## Release mapping

tag-based

Releases are git tags (e.g., `v0.1.0`). Items reference the release via
`release_binding: v0.1.0` in their frontmatter.

## Tag taxonomy

System-routing tags (required for gate skills and design-family routing):

- `security`        auth, scope enforcement, threat-model items; produced by gate-security
- `refactor`        pure-refactor with no behavior change; routes to refactor-design
- `perf`            performance optimization; routes to perf-design
- `testing`         test coverage and quality work; produced by gate-tests
- `cleanup`         debris, dead code, compat shims; produced by gate-cruft
- `documentation`   foundation-doc drift, doc updates; produced by gate-docs

Component tags (jamsesh subsystem boundaries):

- `plugin`          Claude Code plugin: manifest, hooks, skills, local `jamsesh` binary
- `portal`          Go server backend: REST API, MCP endpoint, git smart-HTTP, auto-merger, WS gateway
- `ui`              portal web frontend
- `infra`           deployment, CI, packaging, marketplace, multi-arch binary builds

Add tags as needs emerge (likely candidates: `auth`, `protocol`,
`auto-merger`, `git`). Don't pre-decide.

## Slug conventions

kebab-case with parent-prefix. Children inherit a prefix from the parent so
relationships are visible at a glance:

- Epic: `epic-<scope>` (e.g., `epic-portal`)
- Feature under epic: `feature-<epic-scope>-<feature>` (e.g.,
  `feature-portal-mcp-endpoint`)
- Story under feature: `story-<feature-scope>-<story>` (e.g.,
  `story-portal-mcp-endpoint-auth-validation`)

Long slugs are acceptable. Readability and parent inference at-a-glance win
over brevity.

## Stage overrides

None. Use the master stage progressions from the rules file:

- epic / feature: `drafting → implementing → review → done`
- story: `implementing → review → done` (often skips drafting)
- release: `planned → quality-gate → released`

## Gate config

gates_for_release: [security, tests, cruft, docs, patterns]

Default order. Security findings first (most blocking), then test gaps, then
cruft cleanup, then doc drift, then pattern extraction (least urgent). Each
gate produces items rather than emitting a pass/fail report.
