---
id: epic-distribution-marketplace-publish-workflow
kind: story
stage: done
tags: [infra]
parent: epic-distribution-marketplace
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Marketplace — Publish Workflow

## Scope

Add `marketplace` job to release.yml that publishes the plugin to the `jamsesh-cc-plugin` marketplace repo on every tag.

## Units delivered

- `.github/workflows/release.yml` (edit) — `marketplace` job per parent feature body

## Acceptance Criteria

- [ ] `actionlint` clean
- [ ] Job runs only on tag pushes
- [ ] On a real tag, populates marketplace repo with plugin tree + 5 per-arch binaries + updated plugin.json version + new CHANGELOG entry; commits + tags + pushes
- [ ] SELF_HOST.md documents `MARKETPLACE_DEPLOY_KEY` and `MARKETPLACE_OWNER` setup

## Notes

- Operator one-time setup: create the `jamsesh-cc-plugin` repo, generate a deploy key with write access, add as `MARKETPLACE_DEPLOY_KEY` secret in jamsesh repo settings. Optional `MARKETPLACE_OWNER` variable to publish to a different GitHub org/user.
- The workflow does `rm -rf marketplace/<dir>` then `cp -r jamsesh/<dir>` to mirror plugin source; mechanical sync prevents drift.
- The marketplace repo init (README, initial CHANGELOG) is manual; documented as operator setup.

## Implementation notes

- `marketplace` job added to `.github/workflows/release.yml`. Runs after `build`
  in parallel with `docker` and `sign-and-release`, triggered only on tag pushes.
- Checks out main repo (path: `jamsesh`) and marketplace repo (path: `marketplace`)
  via `MARKETPLACE_DEPLOY_KEY` deploy key secret.
- Assembles plugin tree: rm+cp for `.claude-plugin/`, `hooks/`, `.mcp.json`,
  `skills/`; places 5 per-arch jamsesh binaries into `bin/`.
- Updates `plugin.json` version via `jq` (strip leading `v` from tag).
- CHANGELOG prepend (newest entry first): writes new entry then appends existing
  file if present.
- Commits with `Release ${VERSION}`, creates annotated tag, pushes branch + tag.
- Fixed minor deviation from feature design: CHANGELOG uses prepend (newest-first)
  rather than append, matching standard CHANGELOG conventions.
- `actionlint` passes clean on the combined workflow file.

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Mirror-via-rm+cp keeps source/dist in lockstep mechanically. CHANGELOG newest-first prepend matches standard conventions. Deploy-key auth pattern documented.
