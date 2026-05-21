---
id: story-portal-session-attach-onboarding-sessionlist-integration
kind: story
stage: implementing
tags: [ui]
parent: feature-portal-session-attach-onboarding
depends_on:
  - story-portal-session-attach-onboarding-walkthrough-component
  - story-portal-session-attach-onboarding-help-link
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
---

# SessionList integration (create-success + chrome affordance)

Two changes to `frontend/src/lib/screens/SessionList.svelte`:

1. **Create-success walkthrough** — extend `handleSessionCreated` to set a
   `walkthroughSessionId` state in addition to the existing
   prepend-and-close behavior; render `<SessionAttachWalkthrough>` keyed on
   that state with onclose / onopenSession handlers.
2. **Chrome affordance** — add `<AttachHelpLink sessionId={null} />` to the
   right side of the existing `.topbar`.

The full design is in the parent feature body under
`## Implementation Units → Unit 3`. Read that for the exact state shape,
`onopenSession` navigation target, and acceptance criteria.

## Why one story not two

Both changes touch the same file (`SessionList.svelte`). Splitting would
serialize them anyway because of file-overlap conflicts, and they're
cohesive — both make the surface attach-aware.

## Test file

`frontend/src/lib/screens/SessionList.test.ts` (modify existing).

New tests:
- Creating a session opens the walkthrough with the new session's id
- "Open session view →" inside the walkthrough navigates to `/orgs/{orgId}/sessions/{id}`
- Chrome "Setup help" link renders in the topbar
- Chrome link click opens walkthrough with `sessionId={null}` (chrome-help mode)

Existing tests (load, filter, ws-subscribe, drawer-open) must still pass.

## Negative-case discipline

Same pattern. Verify the new tests catch regressions.
