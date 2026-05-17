---
id: epic-finalize-flow
kind: epic
stage: done
tags: [portal, plugin, ui]
parent: null
depends_on: [epic-cc-plugin, epic-portal-ui, epic-auto-merger]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
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

Locked at epic-design time (initial pass):

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

Locked at the `--only-questions` design-prep pass (2026-05-17):

- **Conflict-resume model**: re-invokable command detects mid-pick.
  When `jamsesh finalize-run` hits a cherry-pick conflict and halts,
  the user resolves the conflict and runs `git cherry-pick --continue`
  themselves (their own Claude Code instance mediates with full project
  context). Re-invoking `jamsesh finalize-run <plan-id>` detects
  mid-cherry-pick state via `git status` and reports the offending
  commit plus what remains in the sequence. No separate
  `finalize-resume` subcommand, no `.jamsesh-finalize-state.json`
  scratch file — the binary rides on git's own state machine. Keeps
  the plugin surface small and avoids state-file drift bugs.

- **Commit-source strategy**: local-first with HTTPS fallback.
  The binary first tries to fetch the session commits from the user's
  local session checkout (filesystem path tracked in plugin state since
  the join flow): `git fetch <local-checkout-path>`. No auth, no
  network, fast. Falls back to fetching from the portal's git
  smart-HTTP endpoint with an ephemeral fetch-only token embedded in
  the remote URL (`https://x-access-token:<token>@portal/git/...`)
  when the local checkout is missing — covers the case where the user
  is finalizing from a different machine than where they joined. The
  ephemeral token has a short TTL (~5 minutes) so even if cleanup
  fails the credential expires on its own. The `jamsesh` remote is
  removed at end of run either way.

- **Merge commits in draft**: linearize via first-parent walk.
  Plan-generation walks `draft` first-parent, follows each merge into
  its second-parent branch, and emits the underlying agent commits in
  chronological order. The auto-merger's merge commits themselves are
  NOT in the cherry-pick list — only the leaf agent commits they
  integrated. Produces a clean linear history on the finalized branch
  and matches what reviewers expect to see in a PR.

- **Finalization mode: squash by default, preserve-all opt-in.** The
  default cherry-pick plan produces ONE commit on the target branch
  containing all curated changes squashed together — matches the
  conventional PR-shipping workflow and is what most teams want
  90% of the time. The curation view has a "Finalization mode"
  segmented control near the top of the cart; squash is pre-selected.
  Users can flip to "Preserve N commits" when they explicitly want
  multi-author history on the target branch (e.g., shipping a
  research jam where per-author attribution at the git level
  matters).

  **Authorship in squash mode**: the squash commit's `author` is the
  user running `jamsesh finalize-run` (the human kicking off the
  ship). The commit message body ends with `Co-authored-by: Name
  <email>` trailers for every distinct author whose commits are in
  the squash. Renders as multi-author in GitHub/GitLab/Forgejo PR
  UIs. Preserves jamsesh's identity model without diverging from
  upstream conventions.

  **Commit-message default**: session goal as subject (truncated to
  72 chars; user-editable). Body = bulleted list of original commit
  subjects in chronological order, followed by the Co-authored-by
  trailers. Editable in the curation view before "Run locally"
  copy-out.

  **Script-body shape (squash mode)**:
  ```
  git fetch <local-path-or-portal>
  git checkout -b <target> <base-sha>
  git cherry-pick --no-commit <c1> <c2> ... <cN>
  git commit --author="<runner>" -m "$(cat <<'EOF' ... EOF)"
  ```
  Cherry-pick `--no-commit` stages each curated commit's changes
  into the index without recording per-commit history. The final
  `git commit` records the single squashed commit with the
  composed message.

  **Conflicts in squash mode** surface as one resolution session
  rather than per-commit. Mid-pick state detection still applies:
  re-invoking `jamsesh finalize-run <plan-id>` after a conflict
  detects the in-progress squash via `git status` (CHERRY_PICK_HEAD
  present + staged changes) and reports remaining commits.

- **Pre-flight checks in `finalize-run`** (run in this order before
  any state-mutating git command):
  1. **Target branch name collision** — `git rev-parse --verify
     refs/heads/<target>` locally and `git ls-remote --heads origin
     <target>` against the source remote. Abort with a clear message
     if either exists; suggest a `--force-recreate` flag or a renamed
     target.
  2. **Dirty working tree** — prompt `Working tree is dirty. Stash
     first? [Y/n]`. Bail on `n`. On `Y`, `git stash push -u` with a
     jamsesh-tagged message; restore via `git stash pop` on clean
     exit.
  3. **Current branch matters** — record the current branch ref;
     warn if it has unpushed commits relative to `origin/<branch>`;
     offer `git checkout -` (or equivalent) on clean exit so the user
     ends where they started.
  4. **Source remote reachable** — `git ls-remote origin` to surface
     auth/network issues up-front, not after a successful cherry-pick
     when the user tries to push.

## Mockups

UI surface for the cross-component slice — the curation view is net-new for
v1. Four layout directions explored at `--only-mocks` design-prep time:

- `.mockups/screens/epic-finalize-flow-portal-ui-curation-view/index.html`
  — navigator (open in browser to compare side-by-side)
  - Option 1 — **Spreadsheet**: dense table, drag-by-order column, side-rail
    summary + CTA. Strongest for scanning a long commit list quickly.
  - Option 2 — **Two-column rivers**: draft on the left, isolated refs in
    the middle, ordered cart on the right. Everything in view at once.
  - Option 3 — **Cart pattern**: source pool of all commits on the left,
    "your final branch" cart on the right with explicit add/remove/reorder.
    Familiar mental model.
  - Option 4 — **Tree-rooted**: continues the session-view DAG aesthetic.
    Click commits in the tree to toggle into the selection; side panel
    shows the ordered list. Lowest context-switch cost from the session
    view.

The feature-design pass on `epic-finalize-flow-portal-ui-curation-view`
will pick a direction (or describe a hybrid) and iterate. Lock-conflict
overlay and post-execution "ready to mark as shipped" state will be mocked
as state variants of the chosen layout in that pass.

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

## Children complete (2026-05-17)

All 3 child features done:
- epic-finalize-flow-plan-generation (3 stories: locks-schema, plan-fetch, fetch-token+mark-shipped)
- epic-finalize-flow-portal-ui-curation-view (2 stories: screen-and-route, squash-editor+chips)
- epic-finalize-flow-plugin-finalize-command (2 stories: flow, fetch-source+cleanup)

## Review (2026-05-17)

**Verdict**: Approve

Capability complete end-to-end. A user can: click Finalize from the session view → curate commits in the cart pattern (squash or preserve mode) → copy `jamsesh finalize-run <plan-id>` → run locally → resolve any conflicts → push to source remote → return to portal → click Mark as Shipped → session transitions to ended/shipped. Lock semantics protect against concurrent finalize; override flow handles the contested case; HTTPS-fallback handles users without a local checkout; cleanup-stack handles SIGINT.

All 7 stories landed with substantial test coverage:
- Backend: 33 finalize tests + 17 mark-shipped tests + golden file checks for script + message composition
- Frontend: 256 tests including 21 for the curation flow + 15 for squash editor/chips
- Plugin: 63 finalizecmd tests with real-git integration coverage

Advancing to done.
