---
id: epic-cc-plugin-packaging
kind: feature
stage: drafting
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

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
