---
id: story-portal-session-attach-onboarding-inviteaccept-integration
kind: story
stage: implementing
tags: [ui]
parent: feature-portal-session-attach-onboarding
depends_on: [story-portal-session-attach-onboarding-walkthrough-component]
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
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
