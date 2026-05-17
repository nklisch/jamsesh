---
id: epic-finalize-flow-plugin-finalize-command
kind: feature
stage: drafting
tags: [plugin]
parent: epic-finalize-flow
depends_on: [epic-finalize-flow-plan-generation, epic-cc-plugin-binary-foundation]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
---

# Finalize Flow — Plugin Finalize Command

## Brief

The two `jamsesh` binary subcommands that wire the local side of
finalize:

- **`jamsesh finalize`** — the entry point. Default: opens the
  portal's finalize view in the user's browser
  (`https://<portal>/sessions/<session>/finalize`). With `--local`:
  fetches the plan and prints it to stdout (for headless users who
  curated via web from another device and just want the cherry-pick
  script locally).

- **`jamsesh finalize-run <plan-id>`** — the one-click execution
  command the portal UI hands the user via copy-to-clipboard. The
  flow:
  1. Resolve plan-id (format `<session_id>:<lock_id>`).
  2. **Mid-pick detection** — before anything else, check `git
     status` in the cwd. If `CHERRY_PICK_HEAD` is present (the user
     resolved a previous run's conflict and re-invoked us), report
     the offending commit and what remains in the sequence based on
     the plan's commit list, then point them at `git cherry-pick
     --continue` (or `--abort`) and exit. The binary does NOT
     try to drive `--continue` itself.
  3. `GET /api/sessions/<session_id>/finalize-plan?lock_id=<lock_id>`
     for the fresh plan. Response includes `mode`, `summary`,
     `script`, `commit_message` (squash mode), `co_authors` (squash
     mode), `fetch_source` (HTTPS-fallback info).
  4. Print the plain-English summary as it appeared in the portal,
     so the user confirms intent locally. Squash variant:
     > This will create a branch `<target>` from base commit
     > `<sha>` in your local source-repo checkout, then squash <N>
     > commits from <M> authors into one commit titled
     > "<subject>" with a Co-authored-by trailer for each
     > contributor. Conflicts during the squash will be left in
     > your working tree for you to resolve. Nothing will be
     > pushed.
     >
     > Commit message:
     > <heredoc-style preview of the composed message>

     Preserve variant uses the existing N-commit cherry-pick wording.
  5. Run pre-flight checks (in order; bail with a clear message on
     any failure):
     - `git rev-parse --verify refs/heads/<target>` — local branch
       collision (suggest `--force-recreate` or new name)
     - `git ls-remote --heads origin <target>` — remote branch
       collision
     - `git status --porcelain` — dirty working tree → prompt
       `Stash first? [Y/n]`; on `Y`, `git stash push -u` with a
       jamsesh-tagged message and remember to pop on clean exit
     - Record current branch (`git symbolic-ref --short HEAD`);
       warn if it has unpushed commits vs `origin/<branch>`; offer
       to `git checkout -` on successful exit
     - `git ls-remote origin` — source remote reachable
  6. Prompt `Proceed? [Y/n]` and read stdin. Bail on `n`.
  7. **Choose commit source** — local-first:
     - If the plugin's local session-checkout path exists on disk
       (from join-time state), use it: `git fetch <local-path>`.
       No auth, no portal touched.
     - Else fall back to HTTPS: mint an ephemeral fetch-only token
       via `POST /api/sessions/<id>/finalize/fetch-token`, build
       `https://x-access-token:<tok>@<portal>/git/<org>/<sess>.git`,
       `git remote add jamsesh <url>`, `git fetch jamsesh`, then
       `git remote remove jamsesh` on exit.
  8. Execute the mode-appropriate script with verbose per-step
     logging (each git operation printed before running):
     - **Squash mode**:
       - `git checkout -b <target> <base-sha>`
       - `git cherry-pick --no-commit <c1> <c2> ... <cN>`
       - `git commit --author="<runner>" -F <heredoc-of-composed-message>`
     - **Preserve mode**:
       - `git checkout -b <target> <base-sha>`
       - `git cherry-pick <c1> <c2> ... <cN>`
  9. On a conflict during cherry-pick: halt the script with a clear
     message showing the offending commit + remaining commits in
     the sequence + the exact resolution command (`git cherry-pick
     --continue` after the user fixes conflicts, or `--abort`).
     The user's own Claude Code mediates resolution with full
     project context. Re-invoking `jamsesh finalize-run <plan-id>`
     re-enters at step 2 and reports current state — it never
     drives `--continue` itself.
 10. On clean completion: print next-step instructions:
     > Branch `<name>` is ready. Push when you're ready:
     >   `git push origin <name>`
     > Then mark the session shipped in the portal.
 11. Optional: `--print-script` flag on `finalize-run` prints the
     raw shell script (mode-appropriate) the binary is about to
     execute for power-user inspection.

**Safety semantics**:

- The command runs in the user's CURRENT working directory (assumed
  to be a source-repo checkout). If the directory isn't a git repo,
  fail fast with a clear message.
- All four pre-flight checks (above) run before any state-mutating
  git command. Failures bail without touching the working tree.
- The temporary `jamsesh` remote (HTTPS-fallback path only) is
  removed on exit — clean success, clean failure, or signal — so
  the user's `git remote -v` doesn't accumulate entries.
- The ephemeral fetch token has a ~5 min TTL on the portal side; even
  if cleanup misses the remote, the credential expires on its own.

**Headless considerations**: `finalize-run` is interactive (Y/n
prompt) by default; add `--yes` for non-interactive use.

Does NOT cover the curation view (`portal-ui-curation-view`). Does NOT
cover the plan-generation backend (`plan-generation`). Does NOT cover
the "Mark as shipped" transition — that's a button in the portal UI;
this command prints next-step instructions pointing the user back.

## Epic context

- Parent epic: `epic-finalize-flow`
- Position in epic: the local-execution layer of the cross-component
  slice. Depends on plan-generation for the plan API; on
  cc-plugin-binary-foundation for the subcommand router, portal
  client, and JSON IO scaffold.

## Foundation references

- `docs/ARCHITECTURE.md` — Reconciliation (local) section (the
  canonical cherry-pick shell flow)
- `docs/UX.md` — Flow: finalizing (steps 4-6 are this command)
- `docs/SPEC.md` — Local client (slash command subcommands)

## Inherited epic design decisions

- **`jamsesh finalize-run` is interactive by default**: plain-English
  summary echo + Y/n prompt + verbose per-step logging.
- **Conflicts halt the script cleanly**: user resolves with their own
  Claude Code, then resumes; no auto-continue.
- **`--local` mode for headless users**: fetches and prints, no
  browser.
- **`--print-script` for power users**: dumps the raw shell for
  inspection before running.
- **Mark-shipped is left to the portal UI**: this command prints
  next-step instructions; doesn't transition status itself.
- **Conflict-resume model**: re-invokable command detects mid-pick
  via `git status`. No `finalize-resume` subcommand, no state file
  — `jamsesh finalize-run <plan-id>` checks for `CHERRY_PICK_HEAD`
  on every invocation and reports state if present. User drives
  `git cherry-pick --continue` / `--abort` themselves with their own
  Claude Code mediating.
- **Commit-source strategy**: local-first (filesystem path to user's
  session checkout, tracked in plugin state from join time) with
  HTTPS fallback (ephemeral fetch-only token embedded in remote URL,
  ~5 min TTL, vended by `POST /finalize/fetch-token`).
- **Linearized merge handling**: the plan's commit list is already
  linearized server-side — the binary just consumes it. The
  cherry-pick / cherry-pick `--no-commit` list never contains merge
  commits.
- **Finalization mode**: the binary handles both. Squash mode runs
  `cherry-pick --no-commit <c1>...<cN>` + `git commit --author=...
  -F <heredoc>` with the composed message from the plan. Preserve
  mode runs `cherry-pick <c1>...<cN>`. Mode comes from the plan
  response; the binary doesn't carry mode state of its own.
- **Pre-flight checks** (in order): target-branch collision (local
  + source remote), dirty working tree (with stash prompt), current
  branch awareness (with restoration), source remote reachable.
  All run before any state-mutating git command.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
