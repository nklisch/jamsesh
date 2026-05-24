---
id: gate-tests-sessionviewshell-hard-cap-reason-branch
kind: story
stage: drafting
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
