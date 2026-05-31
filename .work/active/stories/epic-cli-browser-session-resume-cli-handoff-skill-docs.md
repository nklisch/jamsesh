---
id: epic-cli-browser-session-resume-cli-handoff-skill-docs
kind: story
stage: implementing
tags: [plugin, documentation]
parent: epic-cli-browser-session-resume-cli-handoff
depends_on: [epic-cli-browser-session-resume-cli-handoff-resume-command]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# CLI resume: skill + UX docs

Implements **Unit 3** of `epic-cli-browser-session-resume-cli-handoff`. See the
feature body.

## Scope

- `plugins/jamsesh/skills/jam/SKILL.md`: document that `--open` now ADOPTS the
  CLI identity (opens the session as you, not a fresh participant), and add the
  `jamsesh resume [session-id]` command (reopen your session in the browser;
  bare = current session, explicit id for multi-session, `jamsesh status` lists).
- `docs/UX.md`: reflect the resume handoff in the create/join CLI flows and a
  short "resume later" note (present-tense; describes shipped behavior).

## Acceptance criteria

- [ ] `SKILL.md` documents `--open` identity adoption + `jamsesh resume`.
- [ ] `docs/UX.md` covers the resume handoff in create/join + reopen-later.
- [ ] Copy matches the shipped behavior from Units 1-2.
