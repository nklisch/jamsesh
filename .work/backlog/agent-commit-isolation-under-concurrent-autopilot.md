---
id: agent-commit-isolation-under-concurrent-autopilot
kind: story
stage: drafting
tags: [process, agent-tooling]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
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
