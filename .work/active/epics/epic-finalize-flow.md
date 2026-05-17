---
id: epic-finalize-flow
kind: epic
stage: implementing
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

## Design decisions

- **Concurrent finalize handling**: first finalize locks the session into
  `status: finalizing`. Subsequent finalize attempts see "Alice is
  finalizing — wait for her or override (which restarts curation from
  her draft)?". The lock is held by an account id; only that account
  (or any member with override) can modify the plan or release the lock.
  Lock auto-releases after 30 minutes of inactivity (in case someone
  starts and walks away). Prevents two divergent plans being shipped;
  surfaces the coordination need explicitly.

- **Cherry-pick plan delivery**: `jamsesh finalize-run <plan-id>`
  command backed by a plain-English summary in the portal. The flow:
  1. Human completes curation in the portal UI.
  2. Portal shows a plain-English summary: "This will create a branch
     `<target-name>` from base commit `<sha>` in your local source-repo
     checkout, then cherry-pick these N commits in order: [list with
     author, message, sha]. Conflicts during cherry-pick will be left in
     your working tree for you to resolve. Nothing will be pushed."
  3. Human reviews and clicks "Run locally" — copies a one-line
     command like `jamsesh finalize-run <plan-id>` to their terminal.
  4. The binary fetches the plan via the portal API, prints what it's
     about to do (echoes the summary), prompts "proceed? [Y/n]",
     executes with verbose logging of each git operation.
  5. Conflicts halt the script; the user's local Claude Code helps
     resolve them with full project context.
  6. After clean cherry-pick, the binary prints next-step instructions:
     "Branch `<name>` is ready. Push when you're ready:
     `git push origin <name>`. Then mark the session shipped in the
     portal."

  The plain-English summary is the transparency layer (human knows what
  will happen before execution); the run command is the execution layer
  (one-click). Power users who want to see the raw shell can still get
  it via `jamsesh finalize-run <plan-id> --print-script` for inspection
  before running.

- **Post-finalize session state transition**: manual "Mark as shipped"
  button in the portal. After the human pushes the finalized branch to
  source on their own, they (or any session member) clicks the button
  in the portal session view. Session moves `status: finalizing →
  status: ended` with `ended_at` and `ended_reason: shipped` recorded.
  Until clicked, the session sits in `finalizing` (and the
  finalize-flow UI shows "ready to mark as shipped" prominently). Honest
  about what the portal does and doesn't know: the portal never reaches
  out to the source remote, so it can't detect the push automatically.
  Explicit click is the user's signal of completion. The "ended" reason
  distinguishes `shipped` (cleanly finalized) from `abandoned` (closed
  without finalize) in archived-session stubs.

Locked at epic-design time (this pass):

- **Plan persistence model**: plans are computed on-demand from the
  curation state stored in a `finalize_locks` table; `plan_id` is just
  `<session_id>:<lock_id>`. Each plan-fetch returns a fresh script
  pinning to the curated SHAs. The lock is the durable artifact; the
  plan is a deterministic view. Rationale: avoids stale-plan-id
  problems if draft advances between curation and execution.
- **Lock recovery after browser close**: 30-min auto-release per the
  prior epic decision. Other members see "Alice is finalizing — wait
  or override." Override creates a new lock that supersedes; the old
  lock becomes a ghost.
- **CLI curation in v1**: no. Curation is a multi-commit-selection UX
  that's hard in a terminal; the portal UI is the right home.
  `--local` mode for the plugin is for headless users who curated via
  web from another device and want the script locally.
- **Curation granularity**: individual commits selected from
  isolated refs, NOT whole-ref picks. More flexibility; matches the
  "curation" framing.
- **End-to-end validation as a feature?**: no. Each component
  feature owns its own integration tests; the gate-tests skill at
  release-deploy time coordinates if gaps surface.

## Decomposition

Three child features along component boundaries. The cross-component
nature is intentional — finalize is the value-capture moment, and
keeping its curation UI + plan generation + local execution evolving
together under one epic prevents drift across the three components.

- **plan-generation** (portal-API) owns the `finalize_locks` table,
  lock-acquire/release endpoints, the plan computation endpoint, and
  the `mark-shipped` status transition.
- **portal-ui-curation-view** (portal-UI) is the dedicated full-page
  curation surface (separate from the always-on session view).
- **plugin-finalize-command** (CC plugin) wires `jamsesh finalize`
  (browser-open + `--local`) and `jamsesh finalize-run <plan-id>`
  (the one-click local execution).

Critical path: `plan-generation → {portal-ui-curation-view ||
plugin-finalize-command}`. Two deep with the two consumers parallel.

### Child features

- `epic-finalize-flow-plan-generation` — `finalize_locks` table,
  lock acquire/patch/release/override endpoints, `GET
  /finalize-plan`, `POST /mark-shipped` — depends on:
  `[epic-portal-api-events-log, epic-portal-api-sessions-rest,
  epic-portal-foundation-http-skeleton, epic-portal-git-storage]`
- `epic-finalize-flow-portal-ui-curation-view` — `/sessions/<id>/
  finalize` route with commit-list curation, target-branch input,
  plain-English summary panel, "Run locally" copy button,
  "Mark as shipped" transition — depends on:
  `[epic-finalize-flow-plan-generation, epic-portal-ui-foundation,
  epic-portal-ui-session-view-shell]`
- `epic-finalize-flow-plugin-finalize-command` — `jamsesh finalize`
  (browser-open + `--local`) and `jamsesh finalize-run <plan-id>`
  (interactive with Y/n prompt + verbose logging + clean conflict
  halt) — depends on: `[epic-finalize-flow-plan-generation,
  epic-cc-plugin-binary-foundation]`

### Decomposition risks

- **Concurrent finalize lock under network partition.** Alice locks,
  her connection drops, she returns after 30 minutes (lock released),
  finds someone else mid-finalize. Mitigation: the override UI is the
  explicit conflict-resolution path; lock-status is visible in every
  plan-fetch response.
- **Plan determinism vs draft drift.** Between curation and
  `jamsesh finalize-run` execution, draft might advance with new
  commits. The plan must pin to curated SHAs at generation time, not
  "draft tip at execution." Locked: plan reads SHAs from the lock
  state, not live refs.
- **`jamsesh finalize-run` safety on the user's local checkout.** One
  command runs a long git sequence in the user's source repo. Mid-way
  failure leaves the user in a half-cherry-picked state. Mitigation:
  the plugin command halts cleanly on conflicts (user resolves with
  their own CC); aborts on non-conflict errors with clear recovery
  instructions; the `jamsesh` remote is cleaned up on exit so
  remotes don't accumulate.
- **"Mark as shipped" honesty.** The portal can't detect the user's
  source-remote push. The button is an explicit user signal. The UI
  surfaces "Ready to mark as shipped" prominently until clicked —
  this is a UX truth, not a bug.
