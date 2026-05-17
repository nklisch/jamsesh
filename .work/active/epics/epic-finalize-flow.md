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

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->


## Anticipated child features

Provisional — actual decomposition lands when this epic is designed.

- Portal UI: finalize curation view (commit picker over draft + isolated
  refs, ordering, target branch name input)
- Portal API: cherry-pick plan generation (builds the script body)
- Plugin: `jamsesh finalize` subcommand (browser-open + `--local` modes)
- End-to-end finalize flow validation (joining → contributing → finalizing
  → pushing succeeds end-to-end)

<!-- Design pass on each child feature will fill in specifics. -->
