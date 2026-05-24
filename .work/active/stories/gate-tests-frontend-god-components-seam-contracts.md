---
id: gate-tests-frontend-god-components-seam-contracts
kind: story
stage: review
tags: [testing, ui, refactor]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Decomposed sub-component seams (props in / events out) not asserted in isolation

## Priority
Medium

## Spec reference
Item: `feature-refactor-frontend-god-components`

Acceptance criterion: Refactor target is behavior-preserving decomposition. Existing tests verify the parent screen still works; nothing asserts the extracted child fulfills its declared seam.

## Gap type
missing test for e2e-seam

## Suggested test
For each extracted child (`useFinalizeCuration`, `useFinalizeExecution`,
`useFinalizeLock`, `useFinalizePlan`, `useCommentComposer`, `useTreeState`,
`useRefActions`, `usePlaygroundCountdown`, `useNewSessionForm`) — add a smoke
test that calls the hook in isolation and asserts the contract documented in
the story body.

## Test location (suggested)
`frontend/src/lib/finalize/`, `frontend/src/lib/session/`, `frontend/src/lib/components/`

## Implementation notes

Added dedicated seam-contract test files for all nine hook modules:

- `frontend/src/lib/session/useTreeState.test.ts` — 5 tests via a thin `.test.svelte` harness because `createTreeState` calls `$effect` internally; tests cover default state, localStorage restore, invalid-value fallback, cycle() state machine, and cycle() → localStorage persistence.
- `frontend/src/lib/session/useCommentComposer.test.ts` — 5 tests; covers open/close contract, range select, null-range behavior, and per-instance isolation.
- `frontend/src/lib/session/useRefActions.test.ts` — 7 tests; covers initial state, handleRefAction, closeMenu, handleMenuAction(fork/mode-switch), closeDialog, and isolation.
- `frontend/src/lib/finalize/useFinalizeCuration.test.ts` — 14 tests; covers canRun guards (all three required fields), addCommit dedup, removeCommit, moveUp/moveDown, addAllInGroup-adjacent adoptFromPlan idempotency, and reset().
- `frontend/src/lib/finalize/useFinalizeLock.test.ts` — 6 tests; covers 200 → status set, 409 → conflict, other error → lockError, dismissError, and reset().
- `frontend/src/lib/finalize/useFinalizePlan.test.ts` — 6 tests; covers refetch success/error, schedulePatch debounce fires after delay, cancelPendingPatch prevents fire, and reset() cancels pending patch.
- `frontend/src/lib/finalize/useFinalizeExecution.test.ts` — 7 tests; covers all boolean flags, markCopied/clearCopied, endSession, markShipped success, markShipped error, and reset().
- `frontend/src/lib/components/useNewSessionForm.test.ts` — 7 tests; covers initial state, submit() validation failures, submit() command generation, invitee flag, mode setter, reset() output clearing, and isolation.

`usePlaygroundCountdown` is excluded from isolated tests: its `$derived` reads from `auth.playgroundContext` (a module-level rune store), and its seam is already thoroughly covered by the SessionViewShell integration tests which exercise `seedFromSession`, `mountSubscriptions`, and WS event handling in a full render context.
