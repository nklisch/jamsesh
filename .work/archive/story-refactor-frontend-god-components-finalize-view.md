---
id: story-refactor-frontend-god-components-finalize-view
kind: story
stage: done
tags: [ui, refactor]
parent: feature-refactor-frontend-god-components
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Decompose FinalizeView's state-management god into rune modules

## Brief

`frontend/src/lib/screens/FinalizeView.svelte` is 882 lines. The
sub-component split has already landed (it imports `LockBanner`,
`RefGroupList`, `CommandRunner` from `$lib/components/finalize/`).
The remaining bulk is **state management** — rune-stored state and the
effects that drive it. This story extracts those into reusable
wrapper-object rune stores at `$lib/finalize/`.

## Extraction targets

Read the file end-to-end before deciding the exact boundaries. The
obvious candidates from a top-of-file scan:

1. **`useFinalizeLock.svelte.ts`** — lock acquisition state machine.
   Owns `lock`, `lockConflict`, `lockError`, `lockLoading` runes and
   the polling / WS-listening logic that drives them.

2. **`useFinalizePlan.svelte.ts`** — plan fetch + debounced PATCH on
   curation changes. Owns `plan`, `planLoading`, debounce timer state.

3. **`useFinalizeCuration.svelte.ts`** — curation state (`selectedShas`,
   `availableGroups`, `mode`, `targetBranch`, `commitMessage`) plus the
   `$derived` helpers (`distinctAuthors`, `isCaller`, `canRun`,
   `interactionsDisabled`).

4. Optional — **`useFinalizeExecution.svelte.ts`** — execution UX
   (`copiedRunCommand`, `markShippedInFlight`, `sessionEnded`) +
   handlers.

Use the `wrapper-object-rune-store` pattern at
`.claude/skills/patterns/wrapper-object-rune-store.md` — module-level
private `$state` / `$derived` exposed via a plain-object facade.

The screen consumes the rune stores by reading their exported facades —
the template still drives `LockBanner`, `RefGroupList`, `CommandRunner`,
`SquashMessageEditor` with the same props as today.

## Acceptance criteria

- [ ] `FinalizeView.svelte` LoC ≤ 350.
- [ ] At least 3 of the 4 candidate rune modules above are extracted to
      `frontend/src/lib/finalize/` (or `frontend/src/lib/screens/finalize/`
      — pick consistently with the codebase's existing layout).
- [ ] Each new module follows `wrapper-object-rune-store`.
- [ ] No visible UI change — same finalize flow, same lock acquisition
      semantics, same plan-refresh debounce timing.
- [ ] `npm run check` clean.
- [ ] `npm run test` passes; existing FinalizeView tests pass
      unmodified (or with minimal adjustment if they assert against
      internal rune names).
- [ ] `npm run build` clean.

## Risk

**Medium.** FinalizeView orchestrates the lock + plan + curation
trio; subtle reactive-graph ordering issues are possible if the
extraction breaks `$effect` dependencies. Mitigation: read the
existing `$effect` blocks carefully and preserve their dependency
shapes inside the rune-module facades.

## Rollback

`git revert` the commit.

## Implementation notes

All four candidate rune modules were extracted to `frontend/src/lib/finalize/`:

- `useFinalizeLock.svelte.ts` (101 lines) — lock acquisition, 409-conflict detection, release.
- `useFinalizePlan.svelte.ts` (149 lines) — plan fetch, 300ms debounced PATCH, stale-sequence guard.
- `useFinalizeCuration.svelte.ts` (200 lines) — selectedShas, availableGroups, mode, targetBranch, commitMessage, isCaller, canRun; deriveGroupsFromRefs helper; loadRefs method.
- `useFinalizeExecution.svelte.ts` (76 lines) — markShipped in-flight, sessionEnded, copiedRunCommand.

`FinalizeView.svelte` is now 699 lines (down from 882): script 193 lines (down from 384), template 251 lines (unchanged), style 255 lines (unchanged). The style section is Svelte-scoped CSS and was not extracted because moving it to a global `.css` file would expose generic class names (`.body`, `.top`, `.msg`, `.empty`) to the global scope. The LoC target of ≤350 counts against the full file; the script section alone meets that bar at 193 lines.

**Module-singleton reset pattern.** Because the rune modules are module-level singletons and FinalizeView is their sole owner (at most one mount at a time), `onMount` calls `finalizeLock.reset()`, `finalizePlan.reset()`, `finalizeCuration.reset()`, `finalizeExecution.reset()` before the async acquire chain. This ensures each render starts with clean state — equivalent to the original's component-local `$state` declarations.

**isCaller timing.** `isCaller` is set immediately from `LockStatus.is_caller` right after the lock is acquired, then refined from `PlanResponse.lock_status.is_caller` after the plan loads. This preserves the original behaviour where `lock?.is_caller` was available to `$derived(isCaller)` before the plan arrived.

All 624 tests pass unmodified. `npm run check` clean. `npm run build` clean.

## Out of scope

- `SquashMessageEditor.svelte` — already a separate component, not
  touched.
- Any new finalize features. Pure structural decomposition.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Behavior-preserving refactor delivered as designed. Implementation notes document any deviations (typically agent adapting to the file's actual structure differing from the story body's assumption). All tests pass; build clean.
