---
id: epic-distribution-marketplace-publish-workflow
kind: story
stage: implementing
tags: [infra]
parent: epic-distribution-marketplace
depends_on: []
release_binding: null
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
