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
updated: 2026-05-16
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
  flow per the epic decision:
  1. Resolve plan-id (format `<session_id>:<lock_id>`).
  2. `GET /api/sessions/<session_id>/finalize-plan?lock_id=<lock_id>`
     for the fresh plan (summary + script).
  3. Print the plain-English summary as it appeared in the portal,
     so the user confirms intent locally:
     > This will create a branch `<target>` from base commit
     > `<sha>` in your local source-repo checkout, then cherry-pick
     > these N commits in order: <list>. Conflicts during cherry-pick
     > will be left in your working tree for you to resolve. Nothing
     > will be pushed.
  4. Prompt `Proceed? [Y/n] ` and read stdin. Bail on `n`.
  5. Execute the cherry-pick sequence with verbose logging per step
     (printing each git operation before running). Use the canonical
     shell flow from the brief:
     - `git remote add -f jamsesh https://<portal>/git/<org>/<session>.git`
     - `git fetch jamsesh`
     - `git checkout -b <target-branch> <base-sha>`
     - `git cherry-pick <commit-1> <commit-2> ...`
     - `git remote remove jamsesh`
  6. On a cherry-pick conflict: halt the script with a clear message
     showing the offending commit + how to resolve (start the user's
     own Claude Code with full project context to mediate). Do NOT
     auto-continue.
  7. On clean cherry-pick completion: print next-step instructions:
     > Branch `<name>` is ready. Push when you're ready:
     >   `git push origin <name>`
     > Then mark the session shipped in the portal.
  8. Optional: `--print-script` flag on `finalize-run` prints the raw
     shell script the binary is about to execute (for power users who
     want to inspect or copy-paste).

**Safety semantics**:

- The command runs in the user's CURRENT working directory (assumed
  to be a source-repo checkout). If the directory isn't a git repo,
  fail fast with a clear message.
- If the working tree is dirty, prompt "Working tree is dirty.
  Stash first? [Y/n]" before proceeding.
- The `jamsesh` remote is added with `-f` for the fetch, then
  removed at the end (or on clean failure) so the user's `git remote`
  doesn't accumulate jamsesh entries across multiple finalizes.

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

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
