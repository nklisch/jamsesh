---
id: feature-epic-ephemeral-playground-skill-consolidation
kind: feature
stage: implementing
tags: [plugin]
parent: epic-ephemeral-playground
depends_on: [feature-epic-ephemeral-playground-plugin-skills]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Skill surface consolidation audit + implementation

## Brief

Audits the CC plugin's existing skill surface for consolidation
opportunities where agent intelligence can replace flag-driven branching
across multiple narrow skills, then implements the consolidation per the
audit findings. Generalizes the `/jam` pattern that
`feature-epic-ephemeral-playground-plugin-skills` establishes —
collapsing `/jamsesh:new`, `/jamsesh:playground:new`, and the playground
extension of `/jamsesh:join` into one intent-driven skill — and applies
the same lens to the rest of the existing skill set.

The guiding principle: keep the **binary subcommand surface** rich and
explicit (arguments, flags, deterministic parameter resolution); keep
the **skill surface** thin and intent-driven, letting the agent
translate natural-language requests to subcommand invocations. The
skills exist to teach the agent the binary's contract, not to multiply
that contract by the cartesian product of user intents.

Candidates to audit (initial enumeration; the design pass refines and
ranks):

- `/jamsesh:status` — could fold into `/jam status` (single read entry)
  or stay standalone if the read vs. write distinction matters at the
  skill tier
- `/jamsesh:fork` — natural request shape: "fork from here";
  agent translates to the right `--from <ref>` flag
- `/jamsesh:mode` — natural request shape: "switch this ref to isolated"
  / "rejoin sync"; pure intent → flag translation
- `/jamsesh:finalize` — multi-step (preview, confirm, run); the
  consolidation question is whether `/jam finalize` covers the flow or
  the multi-step is enough motivation to keep a dedicated skill

Likely outcome (subject to design pass): one or two top-level skills
(`/jam` for create/join/status/fork/mode, `/jam-finalize` or
`/jamsesh:finalize` retained for the multi-step finalize flow),
backward-compatible aliases for every deprecated narrow slash to ease
the transition for users who learned the original surface.

## Epic context
- Parent epic: `epic-ephemeral-playground`
- Position in epic: **wave 4** — depends on
  `feature-epic-ephemeral-playground-plugin-skills` for the `/jam`
  pattern to be established before generalizing it. Lands last in
  the epic's implementation sequence.

### Hand-off contract with `plugin-skills` (wave 3)

`plugin-skills` creates `plugins/jamsesh/skills/jam/SKILL.md` with the
create / join intent-vocabulary. This feature **extends** that body
**additively** — appending the status / fork / mode vocabulary and the
auto-loaded SKILL.md updates — and never rewrites or replaces the
wave-3-authored content. When this feature's design pass runs, the
first action against `plugins/jamsesh/skills/jam/SKILL.md` is a Read,
not a Write; the body is amended via `Edit`, not overwritten via
`Write`. This guards against a wave-4 agent silently clobbering
wave-3 output.

## Foundation references
- `docs/ARCHITECTURE.md` § Claude Code plugin package — current
  skill directory layout this feature consolidates
- `docs/UX.md` § Status awareness in CC — references the existing
  `/jamsesh:status` slash command; UX.md roll-forward owned by this
  feature's design pass to describe the consolidated surface
- Parent epic's `plugin-skills` feature body — the `/jam` pattern's
  established conventions (intent-driven skill body, agent translates
  to subcommand invocation, binary subcommand surface stays rich)
  this feature generalizes

## Mockups
No UI surface — CLI + skill body work. CC's slash-command UI is the
"surface," which is text/agent-driven; no visual mockup needed.

## Scoping notes (from this feature's promotion from backlog)

This was originally parked as `idea-skill-surface-consolidation-audit`
during the `plugin-skills` `--only-questions` round, when the user
redirected from the planned three-new-skills approach to a single
`/jam` skill and explicitly called out the broader audit pattern. At
the user's direction, the audit was folded into this epic as a 7th
child feature rather than scoped as a standalone follow-up — the work
is interrelated (the `/jam` pattern is already being established by
`plugin-skills`; this feature generalizes it across the existing skill
set) and the deprecation story is cleanest if consolidated.

The folded-in scope shifts the epic's wave shape: wave 4 is now this
single feature, landing after `plugin-skills` (wave 3). Total epic
size grows from 6 features to 7. The CLI binary's subcommand surface
remains the responsibility of the existing wave-1/wave-2 features
(`cli-first-creation` and `session-lifecycle`); this feature touches
only the **skill** layer at `plugins/jamsesh/skills/`.

## Design decisions

Locked at `--only-questions` time. Feature-design Phase 5 inherits these
as fixed input.

### Pre-launch reality (overarching)

Jamsesh has not yet shipped a public release with the existing skill
surface in use by end users — every decision below is made on the
basis that **there are no installed-base users to migrate**. Backward-
compatible aliases, deprecation windows, picker-visibility hygiene, and
gradual cutover patterns are all moot. The consolidation is a
"rip-the-bandaid" rework that lands atomically alongside the playground
epic; the canonical slash surface emerges in the same release that
ships this epic.

If this assumption ever flips (post-launch with real users), this
feature's decisions are explicitly **not durable for that world** — a
revisit would need to introduce aliases, deprecation hints, and a
deprecation window. Documented here so a future maintainer understands
why this feature looks the way it does.

### Decisions

- **Consolidation shape**: most existing skills fold into a single
  `jam` skill; `finalize` stays standalone. The `jam` skill body
  becomes the single intent-driven entry for status, fork, mode, plus
  the create / join paths inherited from `plugin-skills` — agent
  reads the user's natural-language request (e.g., "switch this ref
  to isolated", "fork from amber-otter's tip", "what's the session
  state?") and invokes the right binary subcommand (`jamsesh status`,
  `jamsesh fork --from <ref>`, `jamsesh mode isolated`). `finalize`
  keeps its own skill because the multi-step gravity (preview →
  confirm → run-locally) and the local-vs-portal split make it a
  genuinely different shape that doesn't compress cleanly into a
  single intent-driven entry.

  Note on naming: the skill is `jam` (directory:
  `plugins/jamsesh/skills/jam/`, frontmatter `name: jam`). CC
  automatically presents and invokes it as `/jamsesh:jam` per the
  plugin-namespace convention (plugin name + `:` + skill name). Both
  forms refer to the same skill — "`/jam`" (informal shorthand) and
  "`/jamsesh:jam`" (CC's displayed full form) are not competing
  names. The same goes for `finalize` (`/jamsesh:finalize`).

- **Old slashes deleted outright, no aliases**: `/jamsesh:status`,
  `/jamsesh:fork`, `/jamsesh:mode`, and `/jamsesh:join` (the last
  inherited from `plugin-skills`' original decision that planned an
  alias) are removed entirely in the same release that ships
  `/jamsesh:jam`. No SKILL.md left behind, no deprecated-alias
  forwarding, no deprecation hint text. Pre-launch reality —
  there's no installed-base muscle memory to preserve.

- **No deprecation window**: the cut is atomic with the release. v0.4.0
  (or whichever release ships this epic) ships `/jamsesh:jam` +
  `/jamsesh:finalize` as the only top-level slashes for the plugin.

- **Discoverability**: trivially clean post-cut — the `/` slash-picker
  shows `/jamsesh:jam` and `/jamsesh:finalize` from this plugin, and
  nothing else. No picker-visibility flag work needed (no aliases to
  hide).

### Scope of work this resolves

In implementation terms, this feature deletes the following files:

- `plugins/jamsesh/skills/status/SKILL.md`
- `plugins/jamsesh/skills/fork/SKILL.md`
- `plugins/jamsesh/skills/mode/SKILL.md`
- `plugins/jamsesh/skills/join/SKILL.md` (inherited deletion from
  `plugin-skills`' updated decision — see that feature's body)

And creates / updates:

- `plugins/jamsesh/skills/jam/SKILL.md` (new — single intent-driven
  skill covering status / fork / mode / create / join surfaces; this
  feature owns the body, `plugin-skills` owns its creation as part of
  wave 3 and this feature extends the body in wave 4)
- `plugins/jamsesh/skills/finalize/SKILL.md` (existing — body may need
  light updates to align with the new mental model, but the skill
  stays)
- `plugins/jamsesh/skills/jamsesh/SKILL.md` (auto-loaded — updated to
  teach the agent the consolidated pattern: `/jamsesh:jam` for almost
  everything, `/jamsesh:finalize` for the finalize multi-step)

The exact body for `plugins/jamsesh/skills/jam/SKILL.md` — the
subcommand vocabulary the agent is taught, the intent-mapping
examples, the ambiguity-handling rules — is content for this feature's
full design pass (Phase 5).

## Architectural choice

**Single tight-cohesion implementation — no child stories.** The work
is file operations: delete 3 SKILL.md files; extend 1 SKILL.md (jam)
additively per the hand-off contract; lightly update 2 SKILL.md files
(finalize, auto-loaded jamsesh). One implementing agent walks through.

Why no stories: zero parallelism (all touched files are tiny and
sequential by their nature — the agent reads the jam SKILL.md
contents that wave-3 plugin-skills authored, then appends; reads
the auto-loaded SKILL.md and amends). Story-spawning would add
ceremony without delivery benefit.

## Implementation units

### Unit 1: Delete obsolete narrow-slash SKILL.md files
**Operation**: `git rm`

```bash
git rm plugins/jamsesh/skills/status/SKILL.md
git rm plugins/jamsesh/skills/fork/SKILL.md
git rm plugins/jamsesh/skills/mode/SKILL.md
```

Verify in the same commit that the binary's subcommands (`jamsesh status`,
`jamsesh fork`, `jamsesh mode`) still exist — they're called by the
new `/jamsesh:jam` skill body. Only the SKILL.md files (slash command
entry points) are deleted, not the underlying functionality.

**Acceptance criteria**:
- [ ] All 3 SKILL.md files removed
- [ ] `jamsesh status`, `jamsesh fork`, `jamsesh mode` binary
      subcommands continue to work (verified by `jamsesh --help`
      output)
- [ ] Running `/` in CC's slash-picker no longer shows
      `/jamsesh:status`, `/jamsesh:fork`, `/jamsesh:mode`

### Unit 2: Extend `/jamsesh:jam` SKILL.md additively
**File**: `plugins/jamsesh/skills/jam/SKILL.md` (modify, NOT replace)

**HARD CONTRACT** (from the hand-off note pinned in this feature's
Epic context section): the first operation on this file is a Read.
Then use Edit (not Write) to append the new sections. The
wave-3 plugin-skills feature authored the initial body covering
create/join intent vocabulary; this feature adds status/fork/mode
intent vocabulary.

Sections to append (after the existing create/join sections):

```markdown
## Status

When the user wants to inspect a jam session ("what's the state",
"show me the session", "who's online"), invoke
`jamsesh status [--json]`. Output groups durable and playground
sessions separately.

If the user has only playground sessions (no account-wide OAuth),
status still works — it enumerates per-session tokens. No
"sign in first" friction.

## Fork

When the user wants to fork from a peer's ref or commit
("fork from amber-otter's tip", "branch off f02ac41"), invoke
`jamsesh fork <commit-sha> [--as <branch>] [--mode sync|isolated]`.

Default mode is `sync` (auto-merger will weave the new ref into draft);
isolated mode keeps the fork private until promoted.

## Mode

When the user wants to flip the current ref's mode ("switch to
isolated", "rejoin sync"), invoke `jamsesh mode sync|isolated`. The
flip takes effect on the next push.

Mode-flip semantics:
- `isolated → sync`: subsequent pushes are auto-merger candidates;
  expect conflicts proportional to drift while isolated
- `sync → isolated`: subsequent pushes don't auto-merge; existing
  merged commits remain in draft
```

**Acceptance criteria**:
- [ ] File body still contains the wave-3-authored create/join
      sections (verified by reading the file post-edit)
- [ ] The three new sections (Status, Fork, Mode) are appended
- [ ] Body reads cleanly as one continuous skill body (not as
      jarringly-bolted-on sections)
- [ ] Operation used Edit, not Write (no rewrite of the file)

### Unit 3: Light update to `/jamsesh:finalize` SKILL.md
**File**: `plugins/jamsesh/skills/finalize/SKILL.md` (modify)

The finalize skill stays standalone (multi-step gravity per the
locked design decision). Light update to align mental model:

- Add a short note at the top: "Finalize is the one operation that
  stays as its own skill (separate from `/jamsesh:jam`). It's a
  multi-step flow with local-vs-portal coordination, so it warrants
  its own surface."
- No other changes — the existing skill body is correct for the
  finalize flow

**Acceptance criteria**:
- [ ] Short framing note added at the top
- [ ] No other content changes
- [ ] Operation used Edit, not Write

### Unit 4: Update auto-loaded `skills/jamsesh/SKILL.md`
**File**: `plugins/jamsesh/skills/jamsesh/SKILL.md` (modify)

Add a section near the top (after the project's intro / fundamentals)
that teaches the consolidated skill pattern:

```markdown
## Skill surface

This plugin exposes two top-level skills:

- `/jamsesh:jam` — intent-driven entry for creating, joining, and
  operating on jam sessions. The agent reads the user's natural-language
  request and invokes the right underlying subcommand. Covers: new
  durable sessions, new playground sessions, joining via URL or ID,
  status queries, forking, mode flips. See `/jamsesh:jam`'s own body
  for the full vocabulary.
- `/jamsesh:finalize` — multi-step finalize flow with local cherry-pick
  coordination. Standalone because the multi-step shape doesn't
  compress into intent-driven dispatch cleanly.

The binary's subcommand surface (`jamsesh new`, `jamsesh join`,
`jamsesh status`, `jamsesh fork`, `jamsesh mode`, `jamsesh finalize`)
remains rich and explicit — the agent invokes them directly via the
skill bodies above. Skills are thin intent translators, not parameter
multipliers.
```

The wave-3 plugin-skills feature added a "Playground sessions" section
to this file. This story's edit goes elsewhere in the file (a different
section) — they don't conflict. Read the file before editing to confirm
location.

**Acceptance criteria**:
- [ ] "Skill surface" section added; agent can read it and understand
      the consolidation
- [ ] Wave-3's "Playground sessions" section unchanged
- [ ] Operation used Edit, not Write

## Implementation order

Sequential, all in one PR:
1. Unit 1 (delete obsolete SKILL.md files)
2. Unit 2 (extend /jamsesh:jam additively)
3. Unit 3 (light finalize update)
4. Unit 4 (auto-loaded SKILL.md update)

## Testing

This feature is content-only (markdown edits + file deletions). No
unit tests in the Go sense. Verification:

- `jamsesh --help` shows all expected binary subcommands intact
- `ls plugins/jamsesh/skills/` shows the post-consolidation skill
  directory layout (`jam`, `finalize`, `jamsesh` — and nothing else
  from the plugin's slash commands)
- Manual review of the modified SKILL.md files for coherence
- Existing `cmd/jamsesh/*_test.go` tests for the underlying
  subcommands continue to pass (sanity regression — the binary surface
  is unchanged by this feature)

## Risks

- **Wave-3 plugin-skills' Story 1 must land first**: Story 1
  (jam-consolidation) creates the `/jamsesh:jam` SKILL.md. This
  feature appends to it. The feature's `depends_on` correctly declares
  the dependency on plugin-skills as a whole; the hand-off contract
  in the body further specifies "read before write, append never
  replace." Implementer attention to this contract prevents wave-4
  silently clobbering wave-3 output.

- **Skill picker UX regression**: deleting the three SKILL.md files
  removes them from the CC slash-picker. If a soft-launch user
  learned `/jamsesh:status` during the v0.3.x window, they hit a
  "skill not found" wall in v0.4.0. Mitigation: prominent release
  notes per the parent epic body's revised risk section. The
  pre-launch reality assumption (locked in --only-questions) accepts
  this as the cost of clean consolidation.

- **Skill body content drift across the two SKILL.md updates**: the
  jam body and the auto-loaded jamsesh body both describe the
  consolidation. If they describe it differently, the agent gets
  conflicting signals. Mitigation: keep the jam body focused on
  "how to invoke" (vocabulary + flags); keep the auto-loaded body
  focused on "why this shape" (architectural framing). They
  complement, don't repeat.

## Implementation notes

Implemented as 4 sequential units with no design-flaw discoveries.

**Unit 1** — Deleted 3 obsolete SKILL.md files via `git rm`:
`plugins/jamsesh/skills/status/SKILL.md`,
`plugins/jamsesh/skills/fork/SKILL.md`,
`plugins/jamsesh/skills/mode/SKILL.md`.
The binary subcommands (`jamsesh status`, `jamsesh fork`, `jamsesh mode`)
remain intact — verified via `jamsesh --help`.

**Unit 2** — Extended `plugins/jamsesh/skills/jam/SKILL.md` additively.
Read the file first (wave-3 wave-3-authored content confirmed intact),
then used Edit to append three new sections: Status, Fork, Mode. The
wave-3 create/join vocabulary is preserved unchanged.

**Unit 3** — Added the framing note at the top of
`plugins/jamsesh/skills/finalize/SKILL.md` (before the `# Finalize the
session` heading). No other content changed. Used Edit, not Write.

**Unit 4** — Inserted a new `## Skill surface` section in
`plugins/jamsesh/skills/jamsesh/SKILL.md` after the opening callout
blocks and before `## 1. What you're working in`. The wave-3 "Playground
sessions" section at the bottom is unchanged.

**Verification**:
- `ls plugins/jamsesh/skills/` → `finalize`, `jam`, `jamsesh` only
- `jamsesh --help` → all subcommands present (auth, mcp-headers, hook, new,
  invite, join, status, fork, mode, finalize, finalize-run, jam)
- `go test ./cmd/jamsesh/...` → all pass
- `go vet ./...` → clean

## Review (2026-05-23)

**Verdict**: Request changes

**Blockers**:
- `story-skill-consolidation-rollforward-foundation-docs` — `docs/UX.md`
  and `docs/ARCHITECTURE.md` still reference the deleted `/jamsesh:status`,
  `/jamsesh:fork`, `/jamsesh:mode`, `/jamsesh:join` slashes. Rolling-foundation
  is a hard-rule blocker, and this feature's own "Foundation references"
  section explicitly assigned the UX.md roll-forward to this work.
- `story-skill-consolidation-primer-stale-slash-refs` — the auto-loaded
  primer `plugins/jamsesh/skills/jamsesh/SKILL.md` instructs agents 6+
  times to run `/jamsesh:status` / `/jamsesh:mode ...` which no longer
  exist. The primer is canonical context for every agent; following its
  current instructions hits "skill not found." This is a correctness bug
  in the consolidated documentation surface.

**Important**:
- `idea-skill-consolidation-references-stale-slash-refs` (backlog) —
  reference files `plugins/jamsesh/skills/jamsesh/references/mcp-tools.md`
  (lines 8, 68) and `references/conflicts.md` (lines 42, 102, 117) also
  reference deleted slashes. Lower blast radius than the primer (loaded
  on-demand, not auto-loaded), but should be cleaned up in the same pass.

**Nits**:
- The `jam` SKILL.md `argument-hint: "[new|join] [flags]"` is now slightly
  misleading because the body documents status/fork/mode flows that bypass
  `jamsesh jam` and invoke the top-level binary subcommands directly. Not
  worth filing — the intent-driven design means the agent translates intent
  rather than passing `$ARGUMENTS` literally for those flows, and the hint
  is still accurate for the literal `jam new|join` paths. Worth a future
  pass when the consolidated mental model settles.

**Notes**:
- The SKILL.md surface changes themselves (delete status/fork/mode, append
  to jam, framing note on finalize, Skill surface section in primer) are
  exactly what the design specified — implementation is faithful to the
  acceptance criteria.
- Binary verification clean: `jamsesh --help` shows all subcommands;
  `go test ./cmd/jamsesh/...` passes.
- The two blocker stories can be batched into a single follow-up PR — they
  share a common cleanup theme. Re-review after they land.
