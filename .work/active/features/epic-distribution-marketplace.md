---
id: epic-distribution-marketplace
kind: feature
stage: drafting
tags: [infra]
parent: epic-distribution
depends_on: [epic-distribution-build-pipeline]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Distribution — CC Plugin Marketplace

## Brief

The CC plugin distribution channel. A separate GitHub repository
(`jamsesh-cc-plugin`) per the Claude Code plugin marketplace's
GitHub-based discovery model, populated by the release pipeline on
every tagged release.

**Marketplace repo layout** (mirrors what `epic-cc-plugin-packaging`
designs locally; this feature ships it):

```
jamsesh-cc-plugin/
├── .claude-plugin/
│   └── plugin.json           manifest with per-arch binary entries
├── bin/
│   ├── jamsesh-darwin-amd64
│   ├── jamsesh-darwin-arm64
│   ├── jamsesh-linux-amd64
│   ├── jamsesh-linux-arm64
│   └── jamsesh-windows-amd64.exe
├── skills/
│   ├── jamsesh/SKILL.md      auto-loaded teaching skill
│   ├── join/SKILL.md
│   ├── status/SKILL.md
│   ├── fork/SKILL.md
│   ├── mode/SKILL.md
│   └── finalize/SKILL.md
├── hooks/
│   └── hooks.json
├── .mcp.json
├── README.md                 install instructions
└── CHANGELOG.md              versioned change log
```

**Release process**:

- On every tag in the main `jamsesh` repo, the build-pipeline
  workflow's downstream step:
  1. Checks out the marketplace repo as a side-by-side workspace.
  2. Updates `plugin.json` version to match the tag.
  3. Replaces `bin/*` with the freshly built binaries from
     build-pipeline outputs.
  4. Syncs `skills/`, `hooks/`, `.mcp.json` from the main repo's
     plugin source (where `epic-cc-plugin-packaging` authors them).
  5. Appends a CHANGELOG entry (auto-generated from the tag's
     release notes).
  6. Commits with message `Release vX.Y.Z` + signs the commit.
  7. Tags the marketplace repo with the same version.
  8. Pushes; the CC plugin marketplace discovery picks up the tag.

**Versioning policy** (synchronized — locked at epic-design): every
portal tag corresponds to a plugin release with the same version.
Plugin-only changes still bump portal version (and v.v.).

**Plugin source vs distribution split**: source lives in the main
`jamsesh` repo under (per `epic-cc-plugin-packaging`'s scope) — the
canonical place for development. The marketplace repo holds only
release artifacts. Developers don't edit the marketplace repo
directly; the release pipeline is the only writer.

Does NOT cover the binary build itself (`build-pipeline`). Does NOT
cover hosted-SaaS or any non-marketplace distribution channel.

## Epic context

- Parent epic: `epic-distribution`
- Position in epic: consumes `build-pipeline` plugin-binary outputs;
  end-of-line consumer in the CC distribution chain.

## Foundation references

- `docs/SPEC.md` — Local client (CC plugin distribution model)
- `docs/ARCHITECTURE.md` — Claude Code plugin package layout (the
  canonical structure this feature ships)

## Inherited epic design decisions

- **Marketplace repo: separate** (e.g., `jamsesh-cc-plugin`) per
  CC's marketplace convention. Source stays in main repo; releases
  are pushed by the build pipeline.
- **Versioning: synchronized** portal + plugin.

## Decomposition risks

- **CC marketplace conventions are evolving.** The manifest format
  and discovery mechanics could change. Mitigation: keep the
  publishing tooling lightweight and re-runnable — adapting to
  format changes shouldn't require code archeology.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
