---
id: feature-refactor-frontend-god-components
kind: feature
stage: drafting
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
