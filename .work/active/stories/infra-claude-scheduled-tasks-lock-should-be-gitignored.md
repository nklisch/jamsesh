---
id: infra-claude-scheduled-tasks-lock-should-be-gitignored
kind: story
stage: implementing
tags: [infra, tooling]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-19
updated: 2026-05-20
---

# `.claude/scheduled_tasks.lock` is tracked but should be gitignored

## Brief

The file `.claude/scheduled_tasks.lock` is committed in the repo and gets
modified by the Claude Code harness whenever scheduled tasks (cron/loop)
mutate. During active autopilot or `/loop` sessions this means the
working tree is never clean — which collides with any tooling that
asserts a clean tree.

Concrete collision surfaced during the autopilot review of
`release-bump-script` (now archived): the script's first pre-flight check
is `git diff --quiet`, which fails as long as the harness is touching
the lock file. The maintainer running the script outside an active
autopilot session won't hit this, but any pre-flight smoke during a
session does.

Fix scope (small):

1. `git rm --cached .claude/scheduled_tasks.lock` (untrack without
   deleting the local file).
2. Add `.claude/scheduled_tasks.lock` to `.gitignore` (or add the
   broader `.claude/*.lock` pattern if other lock files exist or are
   anticipated).
3. Verify other tracked items under `.claude/` are intentional — a
   one-line audit of `git ls-files .claude/` should show what stays.

References:
- Surfaced by `/agile-workflow:autopilot --all` review pass on
  2026-05-19 (commit `8f5e5cf` reviewed at that time).
- The collision is with `scripts/release-bump.sh`'s clean-tree check
  (`scripts/release-bump.sh:114-120`).
