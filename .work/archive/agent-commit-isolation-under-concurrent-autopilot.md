---
id: agent-commit-isolation-under-concurrent-autopilot
kind: story
stage: done
tags: [process, agent-tooling]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-18
---

# Agent commit isolation under concurrent autopilot activity

## Problem

Surfaced during review of `org-session-invite-policy-invite-accept-ui`:
the Wave 3b agent's implementation files (App.svelte, router.svelte.ts,
InviteAccept.svelte/.test.ts, Login.svelte, story body) ended up co-committed
with the unrelated `e2e-test-design: epic-e2e-cnd-coverage-lease-fencing`
work in commit `550280d`. The commit message reflects only the e2e work;
the invite-accept-ui implementation is invisible in `git log --oneline`.

This violates the CLAUDE.md commit-discipline rule:

> Always use `git add <explicit-path>` for every file. Never use `git add .`,
> `git add -A`, or `git commit -a` — these sweep untracked files into unrelated
> commits and have caused real noise in commit history.

The likely cause: two sub-agents running concurrently both staged files into
the same shared git index (one working tree, one .git/index). When one of
them called `git commit`, it picked up the other's staged changes too —
even though each agent's `git add` only named its own paths.

## Why it matters

- Commit history becomes misleading: the `implement: <story-id>` audit
  trail is missing for the invite-accept-ui story.
- Reverting the InviteAccept work would also revert the e2e story files,
  forcing surgery rather than `git revert`.
- The PostToolUse hook's `updated:` bump can land on the wrong story body.
- Cross-cutting reviews (e.g. epic review) can't reliably grep
  `git log --grep "<id>"` to find a story's diff.

## Possible directions (to scope at design)

1. **Worktree isolation for concurrent waves** — implement-orchestrator
   spawns each agent with `isolation: "worktree"` when multiple are
   running in parallel. Each agent commits to its own branch, then the
   orchestrator merges. Expensive; possibly excessive for single-file
   stories.
2. **Index-locking discipline** — guarantee at most one agent per
   conversation holds the staging index at any time. Requires changes
   in how implement-orchestrator and review/scope skills interleave.
3. **Pre-commit verification** — agents diff `git diff --staged` against
   their declared file list before committing; abort if surprise files
   are staged. Cheap and easy retrofit.
4. **Post-commit detection** — orchestrator scans new commit's file list
   against expected; if mismatch, surface a warning so the human can
   manually fix history. Doesn't prevent the issue but at least makes
   it visible.

## Verification path

- Audit other recent autopilot runs for similar bundled commits.
- Reproduce by spawning two skill invocations that both stage + commit
  simultaneously.

## Tags rationale

- `process` — affects multi-agent workflow, not product code.
- `agent-tooling` — fix lives in agent prompts / orchestrator / skill
  templates, not in jamsesh itself.

## Notes

This isn't blocking; the work is correct and tests pass. The fix is
worth doing before the next big multi-feature autopilot run.

## Autopilot routing note (2026-05-17)

`/agile-workflow:autopilot --all` skipped this item — `kind: story` at
`stage: drafting` falls outside the design-family routing table (which
covers `kind: epic` → epic-design and `kind: feature` → feature-design /
refactor-design / perf-design / e2e-test-design). Stories normally skip
drafting per `.work/CONVENTIONS.md`.

Two paths to unblock:
1. Upgrade `kind` to `feature` and re-run autopilot — `feature-design`
   picks it up and formally evaluates the 4 design options.
2. Treat the 4 options in the body as the design pass already done,
   pick the simplest in conversation (likely option 3, "pre-commit
   verification"), advance to `implementing`, and let autopilot drain it.

Additionally: the fix lives **outside the jamsesh codebase** (in the
upstream `nklisch-skills` agile-workflow skill templates, not in
jamsesh product code). The "implementation" would be a PR against
that skill package, not a jamsesh commit. Worth weighing whether to
track this in jamsesh's substrate at all vs. open an upstream issue.

## Closed (2026-05-18, no action)

Reviewed during `/agile-workflow:feature-design`. Decision: drop without
action. Rationale:

- The bug is non-blocking — the affected work was correct and tests
  passed; the only damage was a misleading commit message in
  `550280d` (a single commit, recoverable by `git log --grep` against
  the actual file paths).
- The fix lives in upstream `nklisch-skills` skill templates, not
  jamsesh product code. The upstream rule at
  `implement-orchestrator/SKILL.md:248` already gates worktree
  isolation on file overlap; this case (non-overlapping files sharing
  the git index) is a missed-case in that rule, but designing and
  delivering an upstream PR isn't worth a jamsesh substrate stride.
- Recurrence is infrequent — multi-agent waves with simultaneous
  commits are rare enough that occasional commit-message rewrites are
  cheaper than the prevention machinery.
- v0.1.0 shipped without recurrence and post-release backlog is empty
  except for this and the small clustered-Postgres race; no signal
  that this is biting active work.

Audit trail preserved: the original problem analysis, four design
options, and verification path remain in the body above for anyone who
hits this again.
