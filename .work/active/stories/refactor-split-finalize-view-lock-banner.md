---
id: refactor-split-finalize-view-lock-banner
kind: story
stage: implementing
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
