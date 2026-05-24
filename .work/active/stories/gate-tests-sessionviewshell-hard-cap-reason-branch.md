---
id: gate-tests-sessionviewshell-hard-cap-reason-branch
kind: story
stage: review
tags: [testing, ui, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# `SessionViewShell.test.ts` playground branch lacks `reason=hard_cap` coverage

## Priority
Medium

## Spec reference
Item: `story-epic-ephemeral-playground-portal-ui-router-refactor`

Acceptance criterion: Bug story AC (`story-playground-ws-protocol-mismatch-session-view-extensions`): "DestructionWarningBanner renders for both `idle_timeout` and `hard_cap` reasons, with hard-cap priority preserved."

## Gap type
missing test for valid partition

## Suggested test
```ts
it('updates hard-cap timer when playground.destruction_warning fires with reason=hard_cap', async () => {
    // Fire warning event with reason='hard_cap' and ends_at set.
    // Assert: hardCapAt updates; idleTimeoutAt unchanged.
});
```

## Test location (suggested)
`frontend/src/lib/screens/SessionViewShell.test.ts`

## Implementation notes

Added test `'updates hard-cap timer when playground.destruction_warning fires with reason=hard_cap'` to the existing `playground branch (orgId === org_playground)` describe block in `frontend/src/lib/screens/SessionViewShell.test.ts`.

The test fires a `playground.destruction_warning` WS event with `reason=hard_cap` and `ends_at` set 3 minutes in the future, then asserts the countdown badge acquires the `urgent` CSS class. This confirms the `hard_cap` branch in `usePlaygroundCountdown.mountSubscriptions()` updates `_hardCapAt`, which CountdownBadge converts to `hardCapRemainingMs < WARN_THRESHOLD_MS` → `isUrgent`.

The idle fixture timer stays at 24h (from `makePlaygroundSession`), so only the hard-cap branch drives urgency — the test is partition-exclusive for the `hard_cap` reason. SessionViewShell now has 23 tests (was 22).
