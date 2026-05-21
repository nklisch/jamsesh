---
id: story-portal-session-attach-onboarding-sessionlist-integration
kind: story
stage: review
tags: [ui]
parent: feature-portal-session-attach-onboarding
depends_on:
  - story-portal-session-attach-onboarding-walkthrough-component
  - story-portal-session-attach-onboarding-help-link
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-21
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

## Implementation notes

### Approach

`SessionList.svelte` received two cohesive additions:

1. **Post-create walkthrough trigger** — added `let walkthroughSessionId = $state<string | null>(null)` alongside the existing `drawerOpen` state. `handleSessionCreated` now sets `walkthroughSessionId = newSession.id` after closing the drawer. A `<SessionAttachWalkthrough>` rendered unconditionally (keyed on `open={walkthroughSessionId !== null}`) replaces the drawer with the walkthrough modal after creation. The `onopenSession` handler captures the id before clearing state, then calls `navigate` — the same import already used for session-row navigation.

2. **Chrome affordance** — `<AttachHelpLink sessionId={null} />` was placed inside `.page-actions` alongside the "New session" button. The `Chrome.svelte` component exposes no topbar slot, so the page-actions area (top of the content body, flex-end) is the natural and visually equivalent location.

### Test changes

`SessionList.test.ts` was updated:

- `mockPOST` extracted from the `vi.mock('$lib/api/client', ...)` factory so POST responses can be configured per-test (previously `POST: vi.fn()` was unreachable from tests).
- `mockNavigate` extracted from `vi.mock('$lib/router.svelte', ...)` for assertion.
- `localStorage.clear()` and a clipboard mock added to `beforeEach`.
- `createSessionViaDrawer()` helper opens the drawer, fills the name, submits, and awaits the POST.
- 4 new tests added (13 → 17 total in the SessionList suite; overall suite 514 → 518 tests).

### Negative-case result

Temporarily commenting out `walkthroughSessionId = newSession.id` in `handleSessionCreated` caused tests #1 ("successful session creation opens the walkthrough") and #2 ("Open session view → navigates to session") to fail as expected. Restored before commit.
