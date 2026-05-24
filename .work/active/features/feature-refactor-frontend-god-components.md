---
id: feature-refactor-frontend-god-components
kind: feature
stage: implementing
tags: [ui, refactor]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Decompose the frontend's 500+ line god-components

## Brief

Six Svelte 5 screen / component files in `frontend/src/lib/` have grown
past 500 lines and now bundle multiple distinct concerns into a single
`*.svelte` module. Each is a god-component: one file holds load-state,
WebSocket subscription, form state, list rendering, dialog state, and
in some cases polling/countdown logic. The result is hard-to-test
modules with implicit coupling between unrelated UI surfaces.

Surfaced by a discovery-mode `/agile-workflow:refactor-design` scan.

## Targets

| File | LoC | Distinct responsibilities |
|---|---|---|
| `frontend/src/lib/screens/FinalizeView.svelte` | 882 | lock acquisition, curation state, plan fetch, debounced PATCH, WS subscriptions, ref grouping, end-of-session handling |
| `frontend/src/lib/screens/SessionViewShell.svelte` | 800 | session load, tree-state persistence, bottom-panel toggle, comment composer state, playground countdown, ref-action menu, dialogs (fork / mode-switch), multiple WS subscriptions |
| `frontend/src/lib/components/SessionAttachWalkthrough.svelte` | 747 | multi-step wizard navigation, form validation, invite send, error state |
| `frontend/src/lib/screens/JoinerPicker.svelte` | 580 | session picker + invitation list + acceptance flow |
| `frontend/src/lib/components/NewSessionDrawer.svelte` | 566 | session-create form, CLI output generation, invitee parsing |
| `frontend/src/lib/screens/OrgSettings.svelte` | 555 | org info edit, member list, invitations, deletion |

## Design questions for feature-design

- Per-component decomposition strategies: when to extract to a
  `useFoo.svelte.ts` wrapper-object rune store vs a child component
  vs a co-located private module?
- Test posture: does each newly extracted sub-piece get its own test
  file, or do we collapse coverage at the parent screen?
- Sequencing: can we land per-component child stories in parallel, or
  is there cross-cutting work (a new shared rune store, a `useLock`
  module) that needs to land first?

## Acceptance criteria (target)

- Each named target file ≤ ~300 LoC.
- Extracted sub-pieces conform to the existing wrapper-object rune
  store and snippet-children-component patterns.
- `npm run check` clean.
- `npm run test` passes; coverage does not regress on the affected
  screens.
- No visible UI or behavior change — pure structural decomposition.

## Notes

- Behavior-preserving refactor. If a target's behavior is observed to
  be buggy mid-refactor, file a separate bug story rather than fixing
  inline.
- The `view-state-union-machine` and `wrapper-object-rune-store`
  patterns already documented in `.claude/skills/patterns/` apply to
  most of the extractions.

## Refactor Overview

Six target files, each decomposed in its own child story. All six
touch disjoint Svelte files — no `depends_on` chain — so the
implementer runs them in 2 parallel waves of 3 agents.

Decomposition strategy varies by file:

- **State-heavy screens** (FinalizeView, SessionViewShell) — extract
  rune modules (`wrapper-object-rune-store` pattern) rather than more
  sub-components. FinalizeView already had its sub-component split
  land; the remaining bulk is state.
- **Multi-step / multi-concern screens** (SessionAttachWalkthrough,
  OrgSettings, JoinerPicker, NewSessionDrawer) — extract sub-components
  along the natural concern boundaries (one per step, one per concern).

The `view-state-union-machine` and `wrapper-object-rune-store` patterns
already documented in `.claude/skills/patterns/` are the load-bearing
references.

## Refactor Steps

### Step 1: FinalizeView state-management decomposition
**Priority**: High  **Risk**: Medium
**Files**: `frontend/src/lib/screens/FinalizeView.svelte` (+ new rune modules)
**Story**: `story-refactor-frontend-god-components-finalize-view`

Extract lock state, plan-fetch debounce, curation state, and execution
UX into rune modules at `$lib/finalize/`.

### Step 2: SessionViewShell rune-module decomposition
**Priority**: High  **Risk**: Medium
**Files**: `frontend/src/lib/screens/SessionViewShell.svelte` (+ new rune modules)
**Story**: `story-refactor-frontend-god-components-session-view-shell`

Extract tree-state, playground countdown, ref-actions menu, and
comment composer into rune modules at `$lib/session/`.

### Step 3: SessionAttachWalkthrough step components
**Priority**: Medium  **Risk**: Low
**Files**: `frontend/src/lib/components/SessionAttachWalkthrough.svelte` (+ step components)
**Story**: `story-refactor-frontend-god-components-session-attach-walkthrough`

Extract each wizard step as its own component; extract step-state
machine into a rune module.

### Step 4: JoinerPicker picker vs acceptance split
**Priority**: Medium  **Risk**: Low
**Files**: `frontend/src/lib/screens/JoinerPicker.svelte` (+ extracted component)
**Story**: `story-refactor-frontend-god-components-joiner-picker`

Extract the invitation-acceptance flow into `InvitationList.svelte`
(or equivalent).

### Step 5: NewSessionDrawer form + CLI output split
**Priority**: Medium  **Risk**: Low
**Files**: `frontend/src/lib/components/NewSessionDrawer.svelte`
**Story**: `story-refactor-frontend-god-components-new-session-drawer`

Extract form state into a rune module OR the CLI-output renderer
into a sub-component (whichever cuts cleaner for the file's actual
structure).

### Step 6: OrgSettings concern split
**Priority**: Medium  **Risk**: Low
**Files**: `frontend/src/lib/screens/OrgSettings.svelte` (+ extracted components)
**Story**: `story-refactor-frontend-god-components-org-settings`

Extract MemberList, InviteForm (and optionally InfoEditor /
DangerZone) into separate components.

## Implementation Order

All six stories are independent and run in parallel. The orchestrator
will dispatch them as 2 waves of 3 agents (cap is 3 per wave).

## Out of scope

- New tests beyond what the existing per-screen test suite covers.
  Each story's acceptance criteria requires `npm run test` to still
  pass; per-extracted-module unit tests are a follow-up.
- Cross-component refactors (e.g. shared form-state helpers across
  ForkDialog, ModeSwitchDialog, NewSessionDrawer, CommentComposer).
  Surfaced in the per-feature discovery as a candidate but tracked
  separately if anyone wants it.
