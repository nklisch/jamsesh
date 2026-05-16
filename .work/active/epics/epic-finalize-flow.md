---
id: epic-finalize-flow
kind: epic
stage: drafting
tags: [portal, plugin, ui]
parent: null
depends_on: [epic-cc-plugin, epic-portal-ui, epic-auto-merger]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Finalize Flow

## Brief

End-to-end value capture: the moment jam work becomes a shippable branch
on the source repo.

When humans hit finalize, they get a curation interface (default selection
= `draft` tip since the auto-merger has continuously integrated most work;
optional additions from isolated refs they want to include; commit ordering;
target branch naming). The portal generates a copy-pasteable shell script
— the **cherry-pick plan** — that the human runs in their local source-repo
checkout. The script fetches the session refs as a temporary remote, creates
the target branch from the session base, cherry-picks the chosen commits.
Conflicts during cherry-pick surface in the human's local environment where
their normal Claude Code instance (with full project context) helps resolve.
Finally the human pushes the branch to their source remote.

This epic is intentionally cross-component because finalize is the slice
where the value of the entire system materializes. Scoping it as one epic
ensures the curation UI, plan generation, and plugin subcommand evolve
together with end-to-end validation rather than diverging across three
parent epics.

Component sub-scope:
- Portal UI: a dedicated finalize curation view (separate from the
  always-on session view).
- Portal API: cherry-pick plan generation endpoint (computes the script
  body from the curated commit list).
- Plugin: `jamsesh finalize` subcommand (opens the portal finalize view in
  browser, or with `--local` fetches the plan and prints/copies it).

This epic does NOT cover the cherry-pick execution itself (it runs in the
user's local environment with their own CC); it does NOT cover PR opening
(out of scope per VISION.md — the human's choice on their own forge).

## Foundation references

- `docs/UX.md` — Flow: finalizing
- `docs/ARCHITECTURE.md` — Reconciliation (local) section
- `docs/VISION.md` — What it isn't (PR opening); What you get (a finalized
  branch you push to your source repo on your own terms)
- `docs/PROTOCOL.md` — REST endpoint `POST /api/sessions/<id>/finalize`,
  `GET /api/sessions/<id>/finalize-plan`

## Anticipated child features

Provisional — actual decomposition lands when this epic is designed.

- Portal UI: finalize curation view (commit picker over draft + isolated
  refs, ordering, target branch name input)
- Portal API: cherry-pick plan generation (builds the script body)
- Plugin: `jamsesh finalize` subcommand (browser-open + `--local` modes)
- End-to-end finalize flow validation (joining → contributing → finalizing
  → pushing succeeds end-to-end)

<!-- Design pass on each child feature will fill in specifics. -->
