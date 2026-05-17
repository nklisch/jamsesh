---
id: epic-distribution-marketplace
kind: feature
stage: done
tags: [infra]
parent: epic-distribution
depends_on: [epic-distribution-build-pipeline]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
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

## Design decisions

- **Workflow integration**: extend `.github/workflows/release.yml` with a `marketplace` job that runs after `build` (parallel with `sign-and-release` and `docker`).
- **Marketplace repo**: hardcoded URL `https://github.com/<owner>/jamsesh-cc-plugin.git`. `<owner>` from `vars.MARKETPLACE_OWNER` GitHub variable; falls back to `github.repository_owner` if unset.
- **Auth**: deploy key OR GITHUB_TOKEN with cross-repo push permission. v1 uses `MARKETPLACE_DEPLOY_KEY` secret (operator setup documented in SELF_HOST.md).
- **Single story** — small surface.

## Implementation Units

### Unit 1: Marketplace publish job

**File**: `.github/workflows/release.yml` (edit)

```yaml
marketplace:
  name: publish plugin to marketplace repo
  runs-on: ubuntu-latest
  needs: build
  if: github.event_name == 'push' && startsWith(github.ref, 'refs/tags/v')
  steps:
    - name: checkout jamsesh
      uses: actions/checkout@v4
      with:
        path: jamsesh
    - name: checkout marketplace repo
      uses: actions/checkout@v4
      with:
        repository: ${{ vars.MARKETPLACE_OWNER || github.repository_owner }}/jamsesh-cc-plugin
        path: marketplace
        ssh-key: ${{ secrets.MARKETPLACE_DEPLOY_KEY }}
    - name: download artifacts
      uses: actions/download-artifact@v4
      with:
        path: artifacts
        merge-multiple: true
    - name: assemble plugin tree
      shell: bash
      run: |
        set -euo pipefail
        # Copy plugin source from main repo
        rm -rf marketplace/.claude-plugin marketplace/hooks marketplace/.mcp.json marketplace/skills marketplace/bin
        cp -r jamsesh/.claude-plugin marketplace/
        cp -r jamsesh/hooks marketplace/
        cp jamsesh/.mcp.json marketplace/
        cp -r jamsesh/skills marketplace/

        # Place per-arch binaries with marketplace-expected names
        mkdir -p marketplace/bin
        cp artifacts/jamsesh-darwin-amd64 marketplace/bin/jamsesh-darwin-amd64
        cp artifacts/jamsesh-darwin-arm64 marketplace/bin/jamsesh-darwin-arm64
        cp artifacts/jamsesh-linux-amd64 marketplace/bin/jamsesh-linux-amd64
        cp artifacts/jamsesh-linux-arm64 marketplace/bin/jamsesh-linux-arm64
        cp artifacts/jamsesh-windows-amd64.exe marketplace/bin/jamsesh-windows-amd64.exe

        # Update plugin.json version
        VERSION="${GITHUB_REF_NAME#v}"
        cd marketplace
        jq --arg v "$VERSION" '.version = $v' .claude-plugin/plugin.json > .claude-plugin/plugin.json.tmp
        mv .claude-plugin/plugin.json.tmp .claude-plugin/plugin.json

        # Append CHANGELOG entry
        printf "## v${VERSION}\n\n" >> CHANGELOG.md.tmp
        printf "Released $(date -u +%Y-%m-%d).\n\n" >> CHANGELOG.md.tmp
        [ -f CHANGELOG.md ] && cat CHANGELOG.md >> CHANGELOG.md.tmp
        mv CHANGELOG.md.tmp CHANGELOG.md
    - name: commit + tag + push
      working-directory: marketplace
      run: |
        set -euo pipefail
        VERSION="${GITHUB_REF_NAME}"
        git config user.email "release-bot@jamsesh.io"
        git config user.name "jamsesh release bot"
        git add .
        git commit -m "Release ${VERSION}"
        git tag -a "${VERSION}" -m "Release ${VERSION}"
        git push origin HEAD
        git push origin "${VERSION}"
```

### Unit 2: README scaffold for marketplace repo

(Out of scope for code — operator initializes the marketplace repo manually with a README. Documented in SELF_HOST.md.)

## Testing

- workflow lint via `actionlint`
- Dry-run with workflow_dispatch on a test marketplace repo

## Risks

- **Marketplace repo init**: requires manual one-time setup (create repo, add deploy key). Documented as operator setup step in SELF_HOST.md.
- **Plugin source/dist split drift**: source lives in `jamsesh` repo (.claude-plugin/, skills/, hooks/, .mcp.json); marketplace mirrors. The workflow's `rm -rf` + `cp -r` keeps them in sync mechanically.

## Single Story

`epic-distribution-marketplace-publish-workflow`

## Implementation summary

Single story done.

## Review

**Verdict**: Approve. Capability complete.
