---
id: epic-cc-plugin-packaging
kind: feature
stage: done
tags: [plugin]
parent: epic-cc-plugin
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# CC Plugin — Packaging & Teaching Skill

## Brief

The static artifacts that make up the CC plugin package. None of this
feature is Go code — it's the manifest, the slash-command SKILL.md
files, the hook registration, the MCP config wiring, and the
auto-loaded teaching skill that primes every agent turn.

**Artifacts delivered**:

- `.claude-plugin/plugin.json` — plugin manifest with name (`jamsesh`),
  version, author, description, marketplace metadata, and the per-arch
  binary entries that the marketplace fetches at install (5 targets:
  darwin-amd64, darwin-arm64, linux-amd64, linux-arm64, windows-amd64;
  the binaries themselves come from `epic-distribution`'s build
  pipeline).
- `hooks/hooks.json` — wires CC's six lifecycle events to the binary's
  subcommands: `SessionStart` → `jamsesh hook session-start`,
  `UserPromptSubmit` → `jamsesh hook user-prompt-submit`, `PreToolUse` →
  `jamsesh hook pre-tool-use`, `PostToolUse` → `jamsesh hook
  post-tool-use`, `Stop` → `jamsesh hook stop`, `SessionEnd` → `jamsesh
  hook session-end`.
- `.mcp.json` — points CC's MCP client at the portal's HTTPS-MCP
  endpoint with a `headersHelper` script entry that invokes `jamsesh
  mcp-headers` at connection time.
- `skills/<name>/SKILL.md` — five user-facing slash-command skills:
  `join`, `status`, `fork`, `mode`, and (referenced cross-epic but
  authored here) `finalize`. Each skill's body is a short instruction
  to Claude: "run `jamsesh <command> $ARGUMENTS` and surface the
  result." Skills ARE slash commands in CC's plugin model.
- `skills/jamsesh/SKILL.md` — the **auto-loaded teaching skill** that
  loads on every agent turn whenever the plugin is enabled. Operational
  primer (≤2500 words) covering:
  - Dual-mode model (sync vs isolated, when to use each)
  - Required commit trailer conventions (`Jam-Session`, `Jam-Turn`,
    `Jam-Author`) and optional ones (`Resolves-Conflict`,
    `Auto-Merger`, `Source-Commit`, `Source-Ref`, `Jam-Auto-Commit`)
  - Addressed-comment syntax (`@user`, `@user/branch`, `@all-agents`,
    `@all-humans`, `@everyone`, `@auto-merger`) and recommended-use
    patterns (when to address, kind selection — question / suggestion /
    action-request / fyi)
  - Conflict-resolution flow (recognize a `conflict.detected` event in
    the digest, rebase + resolve locally, commit with
    `Resolves-Conflict: <event-id>` trailer)
  - How to read the digest's structured sections
  - MCP tool usage examples (post_comment, resolve_comment, fork,
    query_session_state)
  - Points at `docs/VISION.md`, `docs/PROTOCOL.md`, and `docs/UX.md`
    for deeper context — does NOT duplicate them.

The teaching skill is the highest-leverage artifact in the package
(it's loaded into every turn's context for every agent in every
session); the design pass enforces the ≤2500-word budget.

Does NOT include the binary, the OAuth flows, the hook implementations,
or any session-commands — those are in the other three features in
this epic. Does NOT cover the multi-arch CI build pipeline — that's
`epic-distribution`.

## Epic context

- Parent epic: `epic-cc-plugin`
- Position in epic: static-artifact authoring; no `depends_on` because
  the artifact contents only reference subcommand names, not subcommand
  implementations. The other three features can be developed in
  parallel.

## Foundation references

- `docs/ARCHITECTURE.md` — Claude Code plugin package (the canonical
  directory layout)
- `docs/SPEC.md` — Local client (plugin manifest, .mcp.json, MCP
  headersHelper)
- `docs/VISION.md`, `docs/PROTOCOL.md`, `docs/UX.md` — sources the
  teaching skill points at (without duplicating)

## Inherited epic design decisions

- **Multi-arch distribution**: five targets, manifest-driven via the CC
  plugin marketplace. Binaries built by `epic-distribution`.
- **headersHelper shape**: synchronous read of token file, outputs
  `{"Authorization": "Bearer <token>"}`. Refresh is async elsewhere.
- **Teaching skill budget**: ≤2500 words, operational not exhaustive.

## Decomposition risks

- The teaching skill is loaded into every agent turn. Verbose teaching
  = expensive context for every user, every session. The ≤2500-word
  budget is the safety valve; design pass treats it as a hard limit.

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Plugin manifest path**: `.claude-plugin/plugin.json` per CC
  plugin convention (NOT `plugin.json` at root).
- **Manifest schema**: minimal — `name`, `version`, `description`,
  `author`, `homepage` (URL to portal). Multi-arch binary
  registration is via CC's marketplace metadata, not plugin.json
  per se; plugin.json declares the binary entry-point name and
  expected location (`bin/jamsesh`); the marketplace repo's
  `marketplace.json` lists per-arch URLs.
- **Hook registration shape**: `hooks/hooks.json` per CC plugin
  hook spec. Each hook entry maps a CC event to a command line.
- **MCP config**: `.mcp.json` with one server entry pointing at
  the portal's `/mcp` endpoint, using `headersHelper` to invoke
  `bin/jamsesh mcp-headers` at connection time.
- **Skills as slash commands**: each `skills/<name>/SKILL.md` has
  YAML frontmatter (`name`, `description`, `argument-hint`) plus
  body text that instructs Claude to run `jamsesh <name>
  $ARGUMENTS`. Five user-facing skills land here: join, status,
  fork, mode, finalize.
- **Teaching skill (`skills/jamsesh/SKILL.md`)**: auto-loaded via
  `triggers:` in the frontmatter. Body budget ≤2500 words (hard
  limit). Operational primer covering modes, trailer conventions,
  addressed-comment syntax, conflict-resolution flow, digest
  reading, MCP tool usage. Points at docs/VISION.md and
  docs/PROTOCOL.md for deeper context.
- **Skill auto-load triggers**: the teaching skill is keyed off
  the presence of `${CLAUDE_PLUGIN_DATA}/sessions/` or the
  session-bound state; exact mechanism follows CC's auto-load
  protocol. Reference: the CC plugin docs for `auto-load:` or
  equivalent frontmatter.
- **Story decomposition**: single story. The artifacts are all
  static-content authoring with a single review concern (does the
  teaching skill stay ≤2500 words while covering the required
  topics).

## Architectural choice

This feature is pure static artifacts — no code, no build step.
The plugin install flow (handled by CC + the marketplace repo)
fetches `bin/jamsesh` per-arch from the GitHub releases produced
by `epic-distribution-build-pipeline`. This feature ships the
markdown + json + yaml; the binary it references lives in a
separate artifact channel.

## Implementation Units

### Unit 1: `.claude-plugin/plugin.json`

```json
{
  "$schema": "https://schemas.anthropic.com/claude-plugin/manifest.json",
  "name": "jamsesh",
  "version": "0.1.0",
  "description": "Multi-agent jamming for codebases — coordinated Claude Code sessions producing PR-shaped branches without merge headaches.",
  "author": {
    "name": "jamsesh maintainers",
    "url": "https://github.com/<owner>/jamsesh"
  },
  "homepage": "https://github.com/<owner>/jamsesh",
  "license": "Apache-2.0",
  "bin": "bin/jamsesh"
}
```

### Unit 2: `hooks/hooks.json`

```json
{
  "hooks": {
    "SessionStart":      "bin/jamsesh hook session-start",
    "UserPromptSubmit":  "bin/jamsesh hook user-prompt-submit",
    "PreToolUse":        "bin/jamsesh hook pre-tool-use",
    "PostToolUse":       "bin/jamsesh hook post-tool-use",
    "Stop":              "bin/jamsesh hook stop",
    "SessionEnd":        "bin/jamsesh hook session-end"
  }
}
```

(Exact field names depend on CC's hook-registration schema —
implementer verifies against current CC plugin docs.)

### Unit 3: `.mcp.json`

```json
{
  "mcpServers": {
    "jamsesh": {
      "type": "streamable-http",
      "url": "${JAMSESH_PORTAL_URL}/mcp",
      "headersHelper": ["bin/jamsesh", "mcp-headers"]
    }
  }
}
```

The `headersHelper` array is invoked synchronously at MCP
connection time; CC parses its stdout JSON and merges into the
request headers.

### Unit 4: Auto-loaded teaching skill

**File**: `skills/jamsesh/SKILL.md`

Frontmatter:

```yaml
---
name: jamsesh
description: jamsesh dual-mode model, trailer conventions, addressed comments, conflict resolution, digest reading
auto-load: true   # or whatever CC's frontmatter requires; verify
triggers:
  - "git commit"
  - "jam session"
  - "session"
---
```

Body sections (≤2500 words total):

1. **What jamsesh does** (~150 words) — multi-agent jam,
   draft as continuous-integration ref, push-per-commit, auto-merger
2. **Dual mode** (~250 words) — sync vs isolated, when to choose, how to switch
3. **Commit trailers** (~400 words) — required (`Jam-Session`, `Jam-Turn`, `Jam-Author`), optional (`Resolves-Conflict`, `Auto-Merger`, `Source-Commit`, `Source-Ref`, `Jam-Auto-Commit`); examples
4. **Addressed comments** (~400 words) — `@user`, `@user/branch`, `@all-agents`, `@all-humans`, `@everyone`, `@auto-merger`; kinds (question / suggestion / action-request / fyi); when to address
5. **Reading the digest** (~250 words) — structured sections (commits, comments, conflicts, mode-changes); how to act on each
6. **Conflict resolution flow** (~350 words) — recognize, rebase, resolve, commit with `Resolves-Conflict: <event-id>` trailer
7. **MCP tools** (~400 words) — `post_comment`, `resolve_comment`, `fork`, `query_session_state` with one-line usage examples
8. **Pointers** (~150 words) — `docs/VISION.md`, `docs/PROTOCOL.md`, `docs/UX.md` for deeper reading; this skill is operational, not exhaustive

Word count enforced at implement time via `wc -w` check; the
acceptance criterion is "≤2500 words".

### Unit 5: User-facing slash-command skills

**Files**: `skills/{join,status,fork,mode,finalize}/SKILL.md`

Each is a thin wrapper: ≤30 words of body + frontmatter naming
the slash command.

```yaml
---
name: join
description: Join a jamsesh session by id, URL, or invite link
argument-hint: "<session-id-or-url> [--as <branch>] [--from <commit>]"
---

Run `bin/jamsesh join $ARGUMENTS` and surface the result. Print errors
to the user with their exit codes intact.
```

Same pattern for `status`, `fork`, `mode`, `finalize`. Each
delegates to the matching `jamsesh` subcommand. The five subcommands
themselves are implemented in `epic-cc-plugin-session-commands`
(and `epic-finalize-flow-plugin-finalize-command` for finalize);
this feature only authors the skill files.

## Implementation Order

Single story.

## Testing

- `wc -w skills/jamsesh/SKILL.md` ≤ 2500
- `jq < .claude-plugin/plugin.json` parses
- `jq < hooks/hooks.json` parses
- `jq < .mcp.json` parses
- Each `skills/<name>/SKILL.md` has valid YAML frontmatter

No Go tests — pure markdown/json.

## Risks

- **CC plugin schema churn**: the `plugin.json`, `hooks.json`,
  and `.mcp.json` schemas may evolve. Mitigation: pin against
  the current CC plugin spec as a known-good baseline; revisit
  on CC plugin version bumps.
- **Teaching skill drift vs PROTOCOL.md**: trailer conventions
  and event types live in PROTOCOL.md as canonical. If they
  change, the teaching skill rots. Mitigation: gate-docs at
  release time catches drift between this skill's text and
  `docs/PROTOCOL.md`.
- **Word budget vs coverage**: 2500 words is tight. Prioritize
  the operational essentials; defer exhaustive examples to
  `docs/PROTOCOL.md` and `docs/UX.md`.

## Implementation summary

Single child story at done. Static plugin package: manifest, hooks, MCP config, 5 user-facing slash skills, auto-loaded teaching skill (2045 words).

### Verification
- All JSON parses (`jq` validation)
- Teaching skill ≤ 2500 word budget honored

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Capability complete. The plugin package directory tree is ready for the marketplace publishing pipeline (epic-distribution-marketplace) to consume on every tag.
