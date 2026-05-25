---
id: idea-attach-onboarding-localstorage-error-handling
kind: story
stage: done
tags: [ui, bug]
parent: feature-attach-onboarding-a11y-robustness
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-21
updated: 2026-05-25
---

Wrap all `localStorage` access in `SessionAttachWalkthrough.svelte` in
try/catch so `QuotaExceededError` / `SecurityError` never block the modal
from closing.

## Scope

**File**: `frontend/src/lib/components/SessionAttachWalkthrough.svelte`

Three call sites:
1. `$effect` mode-on-mount — `localStorage.getItem(DISMISS_KEY)` (line ~47)
2. `handleClose()` — `localStorage.setItem(DISMISS_KEY, 'true')` (line ~84)
3. `handleOpenSession()` — `localStorage.setItem(DISMISS_KEY, 'true')` (line ~92)

## Implementation

### A. `$effect` mode-on-mount

```svelte
$effect(() => {
  if (!open) return;
  let dismissed = false;
  try {
    dismissed = localStorage.getItem(DISMISS_KEY) === 'true';
  } catch {
    // SecurityError or quota-blocked context — default to full mode
  }
  mode = dismissed ? 'compact' : 'full';
});
```

Remove the `typeof localStorage !== 'undefined'` guard — the try/catch
subsumes it (throws for both undefined and restricted access).

### B. `handleClose()`

```ts
function handleClose() {
  if (dismissChecked) {
    try {
      localStorage.setItem(DISMISS_KEY, 'true');
    } catch (e) {
      console.warn('[jamsesh] Could not persist walkthrough dismiss flag:', e);
    }
  }
  onclose();
}
```

### C. `handleOpenSession()`

```ts
function handleOpenSession() {
  if (dismissChecked) {
    try {
      localStorage.setItem(DISMISS_KEY, 'true');
    } catch (e) {
      console.warn('[jamsesh] Could not persist walkthrough dismiss flag:', e);
    }
  }
  if (onopenSession) {
    onopenSession();
  } else {
    onclose();
  }
}
```

## Acceptance Criteria

- [ ] `localStorage.setItem` throwing does not prevent `onclose()` from firing
- [ ] `localStorage.setItem` throwing does not prevent `onopenSession()` from firing
- [ ] `localStorage.getItem` throwing causes the modal to render in `full` mode (not crash)
- [ ] `console.warn` is called on `setItem` failure
- [ ] The `typeof localStorage !== 'undefined'` guard is removed (try/catch is the guard)
- [ ] Test: spy `Storage.prototype.setItem` to throw → `onclose` still called once
- [ ] Test: spy `Storage.prototype.getItem` to throw → modal renders `.modal-card.first-time`

## Implementation notes

- `$effect` mode-on-mount: wrapped `localStorage.getItem(DISMISS_KEY)` in
  try/catch; on throw, falls back to `dismissed = false` (i.e. `mode = 'full'`).
  The `typeof localStorage !== 'undefined'` guard is removed.
- `handleClose()` + `handleOpenSession()`: wrapped `localStorage.setItem(...)`
  in try/catch with `console.warn` on failure. `onclose()` / `onopenSession()`
  always runs.
- Two new tests pin the behaviour:
  - `still calls onclose when localStorage.setItem throws`
  - `falls back to full mode when localStorage.getItem throws`

Verified: `npm test -- --run SessionAttachWalkthrough.test.ts` → 28 passed.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: All three call sites wrapped per design. Read path silently falls back to `full`; write paths log `console.warn` and proceed. The `typeof localStorage !== 'undefined'` guard is correctly removed (try/catch subsumes). Two negative-path tests pin the behavior.
