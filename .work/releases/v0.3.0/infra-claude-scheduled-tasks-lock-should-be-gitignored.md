---
id: infra-claude-scheduled-tasks-lock-should-be-gitignored
kind: story
stage: done
tags: [infra, tooling]
parent: null
depends_on: []
release_binding: v0.3.0
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

## Implementation notes

**Pattern chosen: exact path `.claude/scheduled_tasks.lock`**

Rationale: `git ls-files .claude/` and `ls -la .claude/` showed no other
`.lock` files under `.claude/` — only `scheduled_tasks.lock` itself. Since
there are no other lock files now or obviously anticipated, the exact-path
entry is more precise and avoids inadvertently hiding future lock files that
should be tracked. If additional harness-managed lock files appear, the
pattern can be widened to `.claude/*.lock` at that time.

**Audit of tracked `.claude/` items (all intentional)**

`git ls-files .claude/` (after this fix) shows:
- `.claude/rules/agile-workflow.md` — project navigation rules, intentional
- `.claude/rules/patterns.md` — code pattern rules, intentional
- `.claude/skills/*/SKILL.md` and `references/*.md` — skill definition files,
  intentional (these are project-local skills that extend the Claude Code
  harness for this repo)

No other transient/cache/state files were found. The only candidate was
`scheduled_tasks.lock`, which this story resolves.

**Pre-existing unrelated modification noted (not part of this story)**

`git status` showed `.claude/skills/patterns/openapi-fetch-middleware-client.md`
as modified but unstaged — this is a pre-existing change unrelated to this
story. Left untouched; should be reviewed separately.

(Post-review note: that unstaged change was the in-flight work from
`gate-docs-openapi-fetch-middleware-pattern-citation` which committed in
parallel as `5ca9f63` — not a real cross-story leak, just a timing artifact
of the parallel wave.)

## Review (2026-05-20)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Exact-path entry over `.claude/*.lock` glob is the right surgical call — no other lock files exist or are anticipated. The `git rm --cached` resolved as a no-op deletion in the diff because the file had been staged but never committed to HEAD; the `.gitignore` line is what actually does the work going forward. Resolves the clean-tree collision for `scripts/release-bump.sh`'s `git diff --quiet` pre-flight. Audit of other tracked `.claude/` items came back clean.
