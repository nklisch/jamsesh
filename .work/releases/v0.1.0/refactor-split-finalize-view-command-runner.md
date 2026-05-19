---
id: refactor-split-finalize-view-command-runner
kind: story
stage: done
tags: [refactor, ui]
parent: refactor-split-finalize-view
depends_on: [refactor-split-finalize-view-ref-group-list]
release_binding: v0.1.0
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Finalize split — Extract `<CommandRunner>`

## Files

- New: `frontend/src/lib/components/finalize/CommandRunner.svelte`
- New: `frontend/src/lib/components/finalize/CommandRunner.test.ts`
- Modify: `frontend/src/lib/screens/FinalizeView.svelte`

## What moves

From `FinalizeView.svelte`:

- The `jamsesh finalize-run <plan-id>` command rendering
- Copy-to-clipboard button and its success-feedback affordance
- Any "command is ready / waiting for plan / failed to fetch plan"
  status display

What stays in the orchestrator:

- The plan fetch call (orchestrator owns plan state; subcomponent
  receives the plan ID as a prop)

## Props shape

```ts
type Props = {
  planID: string | null;  // null when plan is still loading or unavailable
  errorMessage?: string;  // surfaced when plan fetch failed
};
```

## Acceptance

- [ ] `CommandRunner.svelte` renders the one-line command with copy
      affordance when `planID` is non-null
- [ ] When `planID` is null, renders a loading or error state
- [ ] `CommandRunner.test.ts` covers: loading, ready (copy works),
      error states
- [ ] `FinalizeView.svelte` orchestrator shrinks below 400 LoC after this
      story merges
- [ ] `FinalizeView.test.ts` passes unchanged

## After this story

The parent feature `refactor-split-finalize-view` can be advanced to
`stage: review`.

## Risk

LOW.

## Rollback

`git revert` the commit.

## Implementation notes

**Extracted scope:** Moved the `copy-box` markup (code + Copy button), the "Run
locally" primary button, the clipboard write logic, and the toast into
`CommandRunner.svelte`. The component owns all copy state internally and fires
an `oncopy` callback so `FinalizeView` can still track whether the user has
copied (to show the ship-hint paragraph).

**Props used:** `command: string`, `ready: boolean`, `disabled?: boolean`,
`errorMessage?: string`, `oncopy?: () => void`. The story's suggested
`planID: string | null` shape was adapted to `command + ready` to keep the
component generic and testable in isolation.

**LoC delta:** `FinalizeView.svelte` 959 → 882 lines (−77 LoC).

**Test count:** 7 new tests in `CommandRunner.test.ts`; full suite 300 tests
(was 293). `FinalizeView.test.ts` passes unchanged (13 tests).

**Clipboard testing approach:** `Object.defineProperty(navigator, 'clipboard',
{ value: { writeText: vi.fn() }, configurable: true })` in `beforeEach`;
spied via the returned mock fn. Toast timer advanced with
`vi.advanceTimersByTimeAsync(1500)` under fake timers.

**Timer cleanup:** `CommandRunner` registers an `onDestroy` hook to clear the
toast `setTimeout` on unmount, replacing the equivalent cleanup that lived in
`FinalizeView.onDestroy`.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: 119-LoC component owns the run-command display, copy button,
clipboard write, and toast. The prop-shape adaptation (`command + ready`
instead of the spec's `planID: string | null`) is actually cleaner — it
makes CommandRunner generic over command strings, not tied to plan-ID
semantics. The `oncopy` callback bridge lets FinalizeView still track
ship-hint state without owning the clipboard logic. Timer cleanup moved
from FinalizeView's onDestroy into CommandRunner's. Clipboard test
approach (`Object.defineProperty(navigator, 'clipboard', ...)`) is
standard for jsdom. FinalizeView 959 → 882 (−77 LoC). 7 new tests; full
suite 300/300 passing.
