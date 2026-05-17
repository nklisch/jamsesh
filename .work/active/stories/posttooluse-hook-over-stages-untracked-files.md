---
id: posttooluse-hook-over-stages-untracked-files
kind: story
stage: review
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

- [x] Root cause identified — no hook or script does a broad `git add`.
      The PostToolUse hook in the agile-workflow plugin
      (`~/.claude/plugins/cache/nklisch-skills/agile-workflow/0.5.0/hooks/scripts/post-tool-use-bump.sh`)
      only runs `sed -i` to bump the `updated:` frontmatter field; it does not
      stage anything. The actual cause is sub-agent discretion: the
      implement-orchestrator's Phase 5 prompt says only "commit with message
      `implement: <story-id>`. Do NOT push." — no explicit `git add <paths>`
      instruction. Without staging guidance, agents default to broad staging
      (likely `git add .` or `git commit -a`) which sweeps any untracked files
      in the working tree. The `scope: org-session-invite-policy` commit left
      the `.mockups/screens/org-session-invite-policy-*/` files untracked, and
      the parallel e2e-tests-infrastructure sub-agents swept them up.
- [x] Documented and accepted as intentional behavior — a `## Commit discipline`
      section was added to `CLAUDE.md` (project root) requiring explicit
      `git add <path>` for every file, forbidding `git add .`, `git add -A`,
      and `git commit -a`. Chosen over patching the plugin because: (a) no hook
      to fix, (b) patching the skill prompt requires an upstream plugin change
      outside this repo, (c) project CLAUDE.md is read by every agent session
      and is the right place for project-level conventions.
- [x] The "test" for the documentation-only remedy is the convention itself:
      any agent (including sub-agents from implement-orchestrator) that reads
      CLAUDE.md before committing will see the explicit prohibition and use
      targeted paths. There is no automated hook-level enforcement; if that is
      wanted in the future, a PreToolUse hook that rejects `git add .` would be
      the right mechanism — scope that as a separate story.

## Autopilot note (2026-05-17)

Advanced from `drafting → implementing` without a design pass. The body
already lays out a bounded investigation path (read `.claude/settings.json`,
look for broad `git add`) and concrete acceptance criteria; the design pass
would just repeat what's here. Investigation + fix happen in one
implementation pass.

## Implementation notes

### What was found

**No PostToolUse hook stages files broadly.** Exhaustive search confirmed:

1. No `hooks` key in `/home/nathan/.claude/settings.json` (user-global) or
   any `.claude/settings.json` / `settings.local.json` / `hooks.json` in this
   project — those files don't exist at the project level.
2. The one PostToolUse hook that IS active comes from the agile-workflow plugin
   (`~/.claude/plugins/cache/nklisch-skills/agile-workflow/0.5.0/hooks/hooks.json`).
   Its script (`post-tool-use-bump.sh`) fires on `Write|Edit` tool calls for
   `.work/active/**.md` or `.work/backlog/**.md` files, and does exactly one
   thing: `sed -i "s/^updated:.*/updated: $today/" "$file_path"`. No git
   operations whatsoever.
3. No `git add .` or `git add -A` anywhere in any plugin version (agile-workflow
   0.1.0 through 0.5.0, ux-ui-design 0.2.1, frontend-design, skill-authoring).
   Some skill instructions use directory globs like `git add .work/active/stories/`
   but that's intentional and scoped.
4. `.work/bin/` contains only `work-view` (no staging scripts). No `scripts/`
   directory exists in the project.

**The real cause: agent staging discretion with no explicit guidance.**

The implement-orchestrator (phase 10 of its SKILL.md) tells sub-agents:
> "Commit with message `implement: <story-id>`. Do NOT push."

This is the only commit instruction. There is no `git add <explicit-paths>`
requirement in the prompt template. When sub-agents lack explicit staging
guidance, they fall back to training-data habits — likely `git add .` or
`git add -A` — which stages everything untracked in the working tree.

The over-staging was a two-part accident:
- The `scope: org-session-invite-policy` commit (b7be016, 09:27:45) included
  only the `.work/` file; the `.mockups/screens/org-session-invite-policy-*/`
  HTML files were created by the screens skill but left untracked (the screens
  skill commit didn't happen before the e2e wave started).
- The parallel e2e-tests-infrastructure sub-agents (09:30–09:57) ran `git add`
  broadly, sweeping those untracked mockup files into unrelated commits.

### Files touched

- `/home/nathan/dev/jamsesh/CLAUDE.md` — added `## Commit discipline` section
- `/home/nathan/dev/jamsesh/.work/active/stories/posttooluse-hook-over-stages-untracked-files.md` — this file

### Reasoning for chosen remedy

Documentation over hook patching:
- Nothing to patch in this repo — the gap is in the agile-workflow plugin's
  SKILL.md prompt template, which lives upstream in the skills repo.
- CLAUDE.md is read by every agent session (the agile-workflow rules even list
  it as item 5 in the ground-yourself checklist). It's the highest-leverage
  place to set a project-wide convention.
- A PreToolUse hook that rejects `git add .` would be a stronger enforcement
  layer but requires adding it to the user's global settings — that's a
  separate concern and a different story's scope.

If the broad-staging recurs after this documentation is in place, the right
next step is to file a PR upstream in the agile-workflow plugin to add an
explicit `git add <changed-files>` instruction to the implement-orchestrator's
Phase 5 commit prompt template.
