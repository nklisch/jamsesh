---
id: distribution-marketplace-repo-bootstrap
kind: story
stage: implementing
tags: [infra, plugin]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Marketplace repo bootstrap

## Context

`v0.1.0` shipped successfully, but the `publish plugin to marketplace repo`
job in `.github/workflows/release.yml` failed because
`nklisch/jamsesh-cc-plugin` doesn't exist on GitHub yet (the checkout step
got a 404). The primary release (binaries, signing, Docker, GitHub Release)
still completed.

Tracked failure run:
https://github.com/nklisch/jamsesh/actions/runs/26070414266 — job
`publish plugin to marketplace repo`.

## What's needed

1. Create the `nklisch/jamsesh-cc-plugin` repo on GitHub (empty, public).
2. Set the `MARKETPLACE_DEPLOY_KEY` secret in the jamsesh repo with a
   deploy key that has write access to the marketplace repo.
3. Optionally seed the marketplace repo with an initial README.
4. Re-run the failed marketplace job for `v0.1.0` (`gh run rerun
   26070414266 --failed`) OR rely on the next tagged release to populate
   it.

## Acceptance

- `nklisch/jamsesh-cc-plugin` exists and is reachable from the release
  workflow.
- The marketplace job for the next tagged release completes successfully,
  pushing the plugin tree + per-arch binaries + a tag matching the
  release tag.
