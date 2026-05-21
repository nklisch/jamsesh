---
id: story-portal-session-attach-onboarding-inviteaccept-integration
kind: story
stage: done
tags: [ui]
parent: feature-portal-session-attach-onboarding
depends_on: [story-portal-session-attach-onboarding-walkthrough-component]
release_binding: v0.3.1
gate_origin: null
created: 2026-05-20
updated: 2026-05-21
---

# InviteAccept integration — walkthrough on accept success

Modify `frontend/src/lib/screens/InviteAccept.svelte` so that on successful
POST (accepting → accepted), the walkthrough opens instead of immediately
calling `navigate(...)`. Navigation is deferred until the user closes the
walkthrough (via any path) or clicks "Open session view →".

The full design is in the parent feature body under
`## Implementation Units → Unit 4`. Read that for the exact state-machine
change and acceptance criteria.

## Summary of change

- Add `let walkthroughOpen = $state(false);`
- In the POST 200 success branch, set `walkthroughOpen = true` instead of
  calling `navigate(...)` immediately.
- Render `<SessionAttachWalkthrough open={walkthroughOpen} sessionId={sessionId}
  onclose={...} onopenSession={...} />` near the bottom of the markup;
  both handlers navigate to `/orgs/{orgId}/sessions/{sessionId}`.
- Rejection and error flows are UNCHANGED — they still navigate immediately.

## Test file

`frontend/src/lib/screens/InviteAccept.test.ts` (modify existing).

New tests:
- POST 200 success opens the walkthrough; URL stays on invite-accept until close
- `onclose` triggers navigation to the session view
- `onopenSession` triggers navigation to the session view
- Rejection flow still navigates immediately without opening the walkthrough
- Error flow still navigates to /login

## Negative-case discipline

Verify by temporarily restoring the immediate `navigate()` call in the
success branch — the new "walkthrough opens on success" test must fail.
Restore the deferred-navigate behavior; test passes.

## Implementation notes

### Files modified

- `frontend/src/lib/screens/InviteAccept.svelte` — added `walkthroughOpen`
  state, replaced immediate `navigate()` in POST 200 branch with
  `walkthroughOpen = true`, rendered `<SessionAttachWalkthrough>` after
  `</div>` at the bottom of the markup.
- `frontend/src/lib/screens/InviteAccept.test.ts` — added clipboard mock and
  `localStorage.clear()` to `beforeEach`; updated existing happy-path test to
  assert dialog opens and navigate is NOT called; added 3 new walkthrough tests.

### State-machine choice

**Option A** — the `accepting` viewState is kept while the walkthrough modal is
open. The "Joining…" button remains disabled behind the modal's backdrop. No
new `attaching` state was introduced; the simpler path was correct here since
the backdrop dims everything and the background UI is not interactive.

### Tests added (delta: +3, updated: 1)

New tests:
1. `POST 200 opens walkthrough modal and defers navigate` — asserts dialog
   appears, `mockNavigate` not called.
2. `closing the walkthrough (ESC) navigates to the session` — fires ESC,
   asserts `mockNavigate('/orgs/org-1/sessions/sess-1')`.
3. `"Open session view" button navigates to the session` — clicks button,
   asserts same navigate call.

Updated test (renamed + behavior change):
- `POSTs with the token in the body on accept click` — previously asserted
  immediate navigate; now asserts dialog opens and navigate is deferred.

Total: 17 tests (was 15), all passing across 44 test files (514 total).

### Negative-case verification

Temporarily replaced `walkthroughOpen = true` with `navigate(...)` (immediate).
Result: 4 tests failed as expected:
- `POSTs with the token in the body on accept click`
- `POST 200 opens walkthrough modal and defers navigate`
- `closing the walkthrough (ESC) navigates to the session`
- `"Open session view" button navigates to the session`

Deferred-navigate restored; all 17 tests pass.
