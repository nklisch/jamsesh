---
id: posttooluse-hook-over-stages-untracked-files
kind: story
stage: implementing
tags: [infra, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# PostToolUse hook over-stages untracked files into unrelated commits

## Finding

Multiple commits during the `epic-e2e-tests-infrastructure` implementation
bundled unrelated files into their staging:

- `806a413 review: epic-e2e-tests-infrastructure-module-skeleton`
  picked up `.mockups/screens/org-session-invite-policy-accept/{index,option-1,option-2,option-3,option-4}.html` — these are from a separate scope item (`b7be016 scope: org-session-invite-policy`)
- `4215a36 implement: epic-e2e-tests-infrastructure-playwright-bootstrap`
  picked up `.mockups/screens/org-session-invite-policy-settings/{index,option-1,option-2,option-3}.html`
- The portal-image-build agent also bundled the ccdriver story's files into one shared commit during parallel-wave execution

The common cause: some PostToolUse hook (or staging script) does a broader
`git add` than the explicit `git add <specific files>` the agent invokes,
causing untracked files in the working tree to be swept into the next
commit.

## Why it matters

- Commit messages become misleading (the "implement: playwright-bootstrap"
  commit changes 1000+ lines of HTML that have nothing to do with Playwright)
- Code-review diffs are noisy
- A failed `git reset` to revert a story's changes also reverts the
  unrelated files
- Reproducibility — replaying the autopilot run in a clean tree would
  produce different commits

## Suggested investigation

- Read `.claude/settings.json` and any `.claude/hooks.json` for PostToolUse
  hooks that touch git
- Check `.work/bin/` for autostaging logic
- Look for any post-edit script that runs `git add .` or `git add -A` rather
  than `git add <path>`

## Acceptance criteria

- [ ] Root cause identified (which hook or script does the broad add)
- [ ] Either the hook is fixed to only stage the file the agent edited, OR
      it's documented and accepted as intentional behavior
- [ ] A test (manual or scripted) confirms an Edit to file X followed by a
      commit only stages X, not any other untracked file

## Autopilot note (2026-05-17)

Advanced from `drafting → implementing` without a design pass. The body
already lays out a bounded investigation path (read `.claude/settings.json`,
look for broad `git add`) and concrete acceptance criteria; the design pass
would just repeat what's here. Investigation + fix happen in one
implementation pass.
