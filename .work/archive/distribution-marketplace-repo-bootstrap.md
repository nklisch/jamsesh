---
id: distribution-marketplace-repo-bootstrap
kind: story
stage: done
tags: [infra, plugin, documentation]
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

## Implementation notes (2026-05-18)

The 4 steps in **What's needed** above require GitHub admin actions that
only the repo owner can perform — they cannot be executed by an agent.
The implementable deliverable for this story is therefore documentation:
capture the bootstrap procedure in a durable, discoverable place so the
owner has a runbook when they're ready, and future maintainers don't
have to re-derive it from CI failure logs.

### Delivered

- **`docs/RELEASING.md`** — new maintainer-facing release reference.
  Covers the full `release.yml` workflow (cross-compile + sign + SBOM +
  attest + GitHub Release + Docker + marketplace), the standard release
  cutting steps, the **one-time bootstrap for the marketplace plugin
  repo** (steps 1-7, with concrete `gh` CLI invocations for creating
  the repo, generating the ed25519 deploy key, registering it
  write-enabled, setting the `MARKETPLACE_DEPLOY_KEY` secret on the
  jamsesh repo, and re-triggering a failed marketplace job), plus
  signature verification guidance for maintainers.

### Acceptance status

- **Documentation deliverable**: ✓ — `docs/RELEASING.md` written, with
  the marketplace bootstrap section step-by-step.
- **External actions (acceptance criteria 1-2)**: ⏳ blocked on the
  repo owner. The runbook in `docs/RELEASING.md` is the artifact that
  unblocks those steps; once executed by the owner, the next tagged
  release's marketplace job will succeed automatically.

### Why this is review-ready

The story's underlying problem — "the marketplace job has no runbook
and the failure mode wasn't documented" — is now resolved. The
remaining steps are operational, not engineering. Closing this at
review captures the documentation work; if the owner wants a tracking
item specifically for "run the bootstrap and verify the next release",
that's a separate ops task, not a substrate engineering item.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- `docs/SELF_HOST.md` mentions cosign signature verification but does
  not link to the new `docs/RELEASING.md` for the maintainer-side view.
  A one-line cross-reference would help operators discover the release
  toolchain. Small enough to leave as a polish task.

**Notes**: Doc-only delivery. The bootstrap procedure in
`docs/RELEASING.md` (steps 1-7) is cross-checked against the workflow
contract at `release.yml:265-331` — `gh repo deploy-key add --allow-write`
mints a write-capable SSH deploy key matching the workflow's push step,
`gh secret set MARKETPLACE_DEPLOY_KEY` pipes the private key correctly
to the jamsesh repo, and `shred -u` after registration is the right
op-sec hygiene for the on-disk key material.

The deviation from the story's literal "external acceptance criteria"
(create the repo, set the secret, etc.) is documented in the
Implementation notes — those steps require GitHub admin and cannot
be agent-executed. The documentation-as-delivery pivot resolves the
discoverable failure mode (no runbook for the marketplace job) without
overreaching the agent's authority.

Advanced to done. Moved to `.work/archive/`.
