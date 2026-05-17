---
id: refactor-split-finalize-view-lock-banner
kind: story
stage: review
tags: [refactor, ui]
parent: refactor-split-finalize-view
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Finalize split — Extract `<LockBanner>`

## Files

- New: `frontend/src/lib/components/finalize/LockBanner.svelte`
- New: `frontend/src/lib/components/finalize/LockBanner.test.ts`
- Modify: `frontend/src/lib/screens/FinalizeView.svelte`

## What moves

From `FinalizeView.svelte`:

- `lockLoading`, `lockConflict`, `isCaller` state and any derived runes
  tied to lock UI
- The markup that renders "you're not the caller" / "lock acquired" /
  "lock loading" banners
- Lock-banner-specific CSS

What stays in the orchestrator:

- The fetch call to the lock endpoint (subcomponent receives lock status
  via props, doesn't issue API calls)
- Polling logic (orchestrator owns the timer; passes updated lock status
  in as a prop)

## Props shape

```ts
type Props = {
  status: 'loading' | 'held' | 'conflict' | 'released';
  callerEmail?: string;  // shown when status === 'conflict'
  onRetry?: () => void;  // shown when status === 'conflict'
};
```

## Acceptance

- [ ] `LockBanner.svelte` renders the four states correctly (loading,
      held, conflict-with-caller, released)
- [ ] `LockBanner.test.ts` covers each state and the onRetry callback
- [ ] `FinalizeView.svelte` no longer carries banner-specific markup or CSS
- [ ] `FinalizeView.test.ts` passes unchanged
- [ ] Dev-server visual check confirms identical lock-state UX

## Risk

LOW. The banner is presentational; the polling logic stays in the
orchestrator.

## Rollback

`git revert` the commit; the new component is unreferenced after revert.

## Implementation discovery

**Design choice:** Option 1 — single `<LockBanner>` with raw flag props. The component receives `{ lockConflict, lockError, lock, isCaller, sessionEnded }` and three callbacks (`onWait`, `onOverride`, `onDismissError`). It decides what to render internally using `$derived`. This maps directly onto the existing inline logic without a forced status-enum translation.

**Lock-pill placement note:** The lock-pill was originally inline inside `.sub` (mixed with plan-base text). Since the pill is now rendered by the top-level `<LockBanner>` instance above `.page-head`, it appears just before the section rather than inside it. The FinalizeView test only asserts presence (`getByLabelText`), not DOM position — test passes unchanged.

## Implementation notes

**Final prop shape:** Single `<LockBanner>` component with raw flag props:
```ts
type Props = {
  lockConflict: { holderAccountId: string } | null;
  lockError: string | null;
  lock: { lock_id: string; is_caller: boolean } | null;
  isCaller: boolean;
  sessionEnded: boolean;
  onWait?: () => void;
  onOverride?: () => void;
  onDismissError?: () => void;
};
```

**CSS rules moved from FinalizeView to LockBanner:**
- `.conflict-banner` + `.conflict-text` + `.conflict-actions`
- `.error-banner`
- `.lock-pill`
- `.btn`, `.btn.primary`, `.btn.ghost` (duplicated into LockBanner; originals kept in FinalizeView for the "Back to sessions" ghost button)

**LoC delta on FinalizeView.svelte:** 1110 → 1065 lines (net −45 lines). The new `LockBanner.svelte` is 101 lines.

**Test count:** 11 tests in `LockBanner.test.ts`. All 13 existing `FinalizeView.test.ts` tests pass unchanged. Full suite: 286/286 passing.
