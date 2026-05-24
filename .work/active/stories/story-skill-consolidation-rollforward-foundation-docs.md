---
id: story-skill-consolidation-rollforward-foundation-docs
kind: story
stage: review
tags: [bug, documentation]
parent: feature-epic-ephemeral-playground-skill-consolidation
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-24
---

# Roll foundation docs forward to reflect consolidated skill surface

## Brief

The skill-consolidation feature deleted `/jamsesh:status`,
`/jamsesh:fork`, `/jamsesh:mode`, and `/jamsesh:join` slash commands
but left foundation docs unchanged. Per the rolling-foundation
principle (and per the feature's own "Foundation references" section,
which assigned roll-forward responsibility to this feature's design
pass), `docs/UX.md` and `docs/ARCHITECTURE.md` must be updated.

## Drifted assertions found during review

### `docs/ARCHITECTURE.md` (§ Claude Code plugin package, lines 160-162)

The "skill directory layout" tree still lists:

```
│   ├── status/SKILL.md            /jamsesh:status command
│   ├── fork/SKILL.md              /jamsesh:fork command
│   ├── mode/SKILL.md              /jamsesh:mode command
```

These files were deleted. The tree must show only `jam/SKILL.md`,
`finalize/SKILL.md`, and `jamsesh/SKILL.md` (the auto-loaded primer).

### `docs/UX.md` (§ Interaction model, line 11)

> The jamsesh plugin adds slash commands (`/jamsesh:join`,
> `/jamsesh:status`, etc.) ...

Should reference the consolidated `/jamsesh:jam` and
`/jamsesh:finalize` instead.

### `docs/UX.md` (§ Flow: fork, line 176)

> 1. `/jamsesh:fork <commit-sha> [--as <branch>] [--mode sync|isolated]`

The slash is gone. Either replace with the binary subcommand
(`jamsesh fork ...`) invoked via `/jamsesh:jam`, or describe the
agent-driven invocation path.

### `docs/UX.md` (§ Flow: switching mode, lines 187, 195)

> 1. `/jamsesh:mode isolated` in CC.
> 1. `/jamsesh:mode sync`.

Same issue — these slashes no longer exist.

### `docs/UX.md` (§ Status awareness in CC, line 247)

> - `/jamsesh:status` — tree summary, peers, scope, mode...

Same issue.

## Acceptance criteria

- [ ] `docs/ARCHITECTURE.md` skill-directory tree updated to reflect
      `jam/`, `finalize/`, `jamsesh/` (and only those)
- [ ] `docs/UX.md` § Interaction model lists the consolidated slashes
- [ ] `docs/UX.md` § Flow: fork describes the consolidated path
- [ ] `docs/UX.md` § Flow: switching mode describes the consolidated path
- [ ] `docs/UX.md` § Status awareness updated
- [ ] `grep -rn "/jamsesh:status\|/jamsesh:fork\|/jamsesh:mode\|/jamsesh:join" docs/`
      returns nothing (or only release-notes / changelog entries)

## Why this is a blocker

The rolling-foundation principle is a hard rule. The feature body's
"Foundation references" section explicitly stated "UX.md roll-forward
owned by this feature's design pass to describe the consolidated
surface" — that work was not done.

## Notes

Filed as a follow-up because the parent feature's atomic deletion
already shipped; the doc roll-forward is mechanically simple and
self-contained.

## Implementation notes

**Consolidated skill surface documented:**
- `/jamsesh:jam` — intent-driven dispatch for new (durable + playground), join, status, fork, and mode operations
- `/jamsesh:finalize` — multi-step finalize flow
- `jamsesh/SKILL.md` — auto-loaded primer

**`docs/ARCHITECTURE.md`** — three edits:
1. Plugin package skill-directory tree: removed `join/`, `status/`, `fork/`, `mode/` entries; now shows `jamsesh/`, `jam/`, `finalize/` only.
2. Slash command subcommands description updated to name the two skills and list the full underlying binary subcommand surface; intro paragraph rewritten to describe intent-driven dispatch.
3. Multi-agent and Recovery sections: all `/jamsesh:join` references updated to `/jamsesh:jam join`.

**`docs/UX.md`** — six edits:
1. § Interaction model: `(/jamsesh:join`, `/jamsesh:status`, etc.)` → `(/jamsesh:jam`, `/jamsesh:finalize`)`.
2. § Creating a session: `/jamsesh:new` skill reference → `/jamsesh:jam`.
3. § Joining a session: `/jamsesh:join <session-id-or-url>` → `/jamsesh:jam join <session-id-or-url>`.
4. § Flow: fork (CC path): replaced direct slash invocation with natural-language-dispatch description.
5. § Flow: switching mode: replaced both `/jamsesh:mode isolated` and `/jamsesh:mode sync` with `/jamsesh:jam` natural-language-dispatch pattern.
6. § Status awareness: replaced `/jamsesh:status` bullet with `/jamsesh:jam (status)` dispatch pattern.

**SKILL.md files** — no changes needed; `jamsesh/SKILL.md`, `jam/SKILL.md`, and `finalize/SKILL.md` already accurately describe the consolidated surface (updated by earlier sibling story).
