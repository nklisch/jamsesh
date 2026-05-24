---
id: story-refactor-frontend-god-components-session-attach-walkthrough
kind: story
stage: review
tags: [ui, refactor]
parent: feature-refactor-frontend-god-components
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Decompose SessionAttachWalkthrough multi-step wizard

## Brief

`frontend/src/lib/components/SessionAttachWalkthrough.svelte` is 747
lines — a multi-step wizard. Steps are tightly bundled into a single
file, making each step hard to test in isolation and adding nesting
that obscures the linear flow.

## Extraction targets

Read the file first to identify steps. A multi-step wizard typically
splits along:

1. **Step components** — one Svelte component per step
   (`InstallCommandStep.svelte`, `AttachCommandStep.svelte`,
   `VerifyStep.svelte`, etc. — actual step names per the file). Each
   step takes props for shared state and calls into a `step` handler
   to advance.

2. **`useAttachWalkthrough.svelte.ts`** — wizard state machine
   (current step, validation, can-advance logic) using the
   `view-state-union-machine` pattern for step identity:
   `type WalkthroughStep = 'install' | 'attach' | 'verify' | 'done'`.

3. **Form-state helpers** — if any step manages form-input state,
   apply the form-state pattern noted in the per-feature design
   discovery (could land in a sibling refactor; not blocking here).

Use `wrapper-object-rune-store` for the rune module and
`view-state-union-machine` for the step state.

## Acceptance criteria

- [ ] `SessionAttachWalkthrough.svelte` LoC ≤ 300.
- [ ] Each wizard step is its own component in
      `frontend/src/lib/components/walkthrough/` (or sibling
      directory).
- [ ] Wizard state machine extracted to a rune module.
- [ ] No visible UI change — same step transitions, same validation,
      same final state.
- [ ] `npm run check` clean.
- [ ] `npm run test` passes; new per-step tests are NOT required by
      this story but the existing walkthrough test must not regress.
- [ ] `npm run build` clean.

## Risk

**Low.** Wizard steps tend to be naturally cohesive — extraction
boundaries are obvious once the file is read.

## Rollback

`git revert` the commit.

## Implementation notes

The file was a two-mode modal (`full` / `compact`), not a multi-step
wizard as the story template assumed. The actual extraction split was:

- `walkthrough/CcPane.svelte` (199 LoC) — shared Claude Code prompt
  pane used in both modes
- `walkthrough/FullCard.svelte` (368 LoC) — first-time ceremonial card
  (terminal + CC pane + dismiss checkbox)
- `walkthrough/CompactCard.svelte` (161 LoC) — returning-user compact
  card (CC pane only + reopen link)

Parent `SessionAttachWalkthrough.svelte` reduced from 747 → 163 LoC.

`view-state-union-machine` pattern applied: `type Mode = 'full' | 'compact'`
drives template branching in parent. `$bindable()` used on `closeBtnRef`
prop in both card components so parent's focus-management `$effect` can
still call `.focus()` on the close button after mode switch.

All 624 tests pass; `npm run check` 0 errors; `npm run build` clean.
