---
id: org-session-invite-policy-invite-accept-ui
kind: story
stage: implementing
tags: [ui]
parent: org-session-invite-policy
depends_on: [org-session-invite-policy-invite-accept-enforce, org-session-invite-policy-get-invite-details]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# InviteAccept.svelte — Onboarding hero

New screen at `/orgs/:orgID/sessions/:sessionID/invites/:inviteID/accept`
(with `?token=<token>`). Implements the chosen mockup
**Option 3 — Onboarding hero** (see
`.mockups/screens/org-session-invite-policy-accept/option-3.html`).
Renders the inviter pill, session-name headline, lead explainer, and the
"What happens when you accept" card on the happy path; warning state for
`members_only` rejection; danger state for invalid/expired tokens.

## Files

- New: `frontend/src/lib/screens/InviteAccept.svelte`
- New: `frontend/src/lib/screens/InviteAccept.test.ts`
- Modify: `frontend/src/lib/router.svelte.ts` — new route
- Modify: `frontend/src/App.svelte` — new screen render branch

## Reference mockup

`.mockups/screens/org-session-invite-policy-accept/option-3.html`

Honors the hero layout: invited-by pill, big headline with the session
name as the accent-colored phrase, lead paragraph, primary Accept CTA,
the "What happens when you accept" explainer card. Rejection and error
states reuse the hero layout but swap the headline and replace the
explainer with a warning- or danger-tinted alert.

## Routing

```ts
// frontend/src/lib/router.svelte.ts
{
  pattern: /^\/orgs\/([^/]+)\/sessions\/([^/]+)\/invites\/([^/]+)\/accept$/,
  name: 'invite-accept',
  params: ['orgId', 'sessionId', 'inviteId'],
},
```

The token comes from `window.location.search`'s `?token=<token>` query
param, NOT from the path. Read it inside the component via
`new URLSearchParams(window.location.search).get('token')`.

In `App.svelte`:

```svelte
{:else if current.name === 'invite-accept'}
  <InviteAccept
    orgId={current.params.orgId}
    sessionId={current.params.sessionId}
    inviteId={current.params.inviteId}
  />
```

**Auth gate exception**: the existing `App.svelte` auth-gate `$effect`
redirects unauthenticated users to `/login` for all non-login routes.
Invite-accept SHOULD trigger this redirect (the API requires bearer auth).
After login, the user should land back on the invite URL. Verify the
existing login flow's post-auth redirect — if it always sends users to
`/orgs/:orgID/sessions`, add a `?return_to=<original-url>` query param
preserved across the login round-trip. Implement that small enhancement
as part of this story if not already present.

## Component contract

```ts
type Props = {
  orgId: string;
  sessionId: string;
  inviteId: string;
};
```

On mount:
1. Extract `token` from query string. If missing → render error state.
2. Call `GET /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}?token=<token>`.
   - 200 → ready state (hero with inviter, session name, expires, explainer)
   - 401 → error state ("invite no longer valid")
   - 409 → error state ("already accepted")
   - other → error state (network)

On Accept click:
- Call `POST /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept`
  with body `{ token }`.
  - 200 → navigate to `/orgs/:orgID/sessions/:sessionID`
  - 403 + `Error: 'auth.org_membership_required'` → rejection state
    (warning hero with members-only explanation)
  - other 4xx/5xx → error state

On Decline click:
- Navigate to `/orgs/:orgID/sessions` if user is in any org, else to `/login`
  fallback.

## States to render

- **Loading** — minimal hero skeleton ("Checking invite…") while GET resolves
- **Happy / ready** — full hero with all metadata + Accept/Decline
- **Rejection** — hero with `Acme Corp is members only` headline + warning
  alert explaining what to do next ("ask an admin to add you to the org")
- **Error** — hero with `This invite is no longer valid` headline + danger
  alert with the server's error code

The mockup's interactive state toggle was for review; production renders one
state at a time based on actual API responses.

## Acceptance criteria

- [ ] Route renders for each state correctly
- [ ] On mount, GET is called with org/session/invite IDs and the token
      from the query string
- [ ] Accept click POSTs with the same token in the body, then navigates
      to the session on success
- [ ] Decline navigates back to the user's session list
- [ ] Rejection state surfaces when the POST returns 403 with
      `auth.org_membership_required`
- [ ] Error state surfaces on 401, 409, and network failures
- [ ] `npm test -- --run InviteAccept` passes
- [ ] `npm run check` clean
- [ ] Full suite passes (no regressions)

## Risk

MEDIUM. New screen + new route + GET-then-POST flow + login-return-to
handling. The login-return-to flow specifically can be subtle if the
existing login screen wasn't built for it. Mitigations:
- The screen handles its own state machine; no shared global state
  beyond `auth`
- The login-return-to enhancement is isolated to one screen + one router
  helper
- Tests pin each state

## Rollback

`git revert` the commit. The route disappears; the email link starts
failing with a 404 on the frontend (users see NotFound.svelte). The
backend endpoints stay functional.
