---
id: feature-playground-anon-session-access
kind: feature
stage: done
tags: [playground, auth]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-06-01
updated: 2026-06-01
---

# Anonymous playground participants as first-class session members

## Brief

Anonymous playground participants are added as `session_members` at join time
(`internal/portal/playground/handler.go:188,322` call `AddSessionMember`) but
are **never** `org_members` of the reserved `org_playground` org. As a result
they are second-class on every org-scoped session surface: the endpoints that
populate the session UI gate on org membership and reject them, and the SPA
doesn't reliably carry the anonymous bearer across navigations. The net effect
is a broken first impression for the playground — the exact funnel the
visitor-entry landing drives traffic into.

Three concrete defects, all symptoms of the same gap, observed live on v0.5.0
(session `01KT0M1JPAMMSEXAQQBSTZFD7D`, 2026-06-01):

1. **Empty file tree** — `GET /api/orgs/{orgID}/sessions/{id}/refs` and
   `/files` return `403 "not a member of this org"` because
   `ListSessionRefs` (`internal/portal/sessions/state.go:109`) and
   `ListSessionFiles` (`internal/portal/sessions/files.go:46`) call
   `GetOrgMember` before the session-membership check. The push lands and
   `base_sha` is set, but the file panel renders nothing. (Same gating in
   `refmodes.go:43`, `listing.go:38`, `invites.go:45`, `state.go:236`.)
2. **Refresh → login bounce** — on a fresh page load at the org-scoped URL
   `/orgs/org_playground/sessions/<id>` the `anonymous_session_bearer` isn't
   rehydrated, so `GET /api/playground/sessions/{id}` returns 401 and the SPA
   redirects to login. The bearer IS attached on the canonical
   `/playground/s/<id>/...` URL (that route returns 200).
3. **No live WebSocket updates** — the WS layer guards on `auth.token` and the
   `POST /api/auth/ws-ticket` request isn't playground-scoped, so the anonymous
   bearer never reaches the upgrade and `/ws/sessions/<id>` 403s.

## Strategic decisions

- **Fix direction for the server-side 403s** (org_members-vs-session-membership):
  deferred to feature-design. Two candidates, to be resolved with full code
  context during the design pass:
  - *Add anon as org_members* — at playground join also
    `AddOrgMember(org_playground, role=member)`. One change clears the refs,
    files, and ws-ticket 403s simultaneously, but grants anon broader
    org-scoped reach (listing, invites) that may need separate guarding.
  - *Endpoints accept session membership* — make the org-gated session
    endpoints fall back to session membership when `org_id` is the reserved
    playground org. More surgical but touches every handler and adds a
    playground special-case.
  Whichever is chosen determines how much collapses into a single change — the
  org_members route could resolve stories 1 and 3 server-side together, leaving
  only the frontend rehydration (story 2) and the WS-ticket scoping as separate
  work.

## Decomposition

Three child stories already exist (created at scope time); feature-design should
refine their bodies and sequencing rather than spawn new ones:

- `story-playground-anon-access-file-tree-403` — server-side authz (portal)
- `story-playground-anon-access-refresh-bounce` — SPA bearer rehydration (ui)
- `story-playground-anon-access-ws-live-updates` — playground-scoped WS ticket
  + bearer (ui), migrated from backlog

## Design decisions

- **Should anonymous playground participants become `org_members` of
  `org_playground`?** No. Keep anonymous accounts session-only and make
  session-scoped endpoints authorize against `session_members`. This preserves
  the foundation-doc invariant that anonymous accounts never appear in
  `org_members` and avoids granting org inventory or invite surfaces to throwaway
  identities.
- **How should reload/direct-return work for anonymous browser users?** Store
  the anonymous playground context in browser-scoped `localStorage` keyed by
  `session_id`, with the raw bearer, nickname, and server `expires_at`. This is
  intentionally stronger than the current in-memory-only contract: a visitor
  should be able to come back in the same browser and jump back in while the
  playground session is live.
- **How should stale stored bearers fail?** Never route a playground bearer
  failure through durable account sign-out or `/login`. Clear the stored
  playground context and move the user to the playground join or ended path,
  depending on whether the bearer was absent locally or rejected by the server.
- **Does this feature need new mockups?** No. The work repairs existing
  session-view, tree, artifact, and WebSocket behavior without changing visual
  structure. Existing playground/session UI patterns are reused.

## Architectural choice

Three plausible approaches were considered:

1. **Add anonymous participants as `org_members` of `org_playground`.** This is
   the smallest backend diff, but it violates the explicit auth model and risks
   widening access to org-level routes like session listing and invites.
2. **Special-case `org_playground` in every failing handler.** This repairs the
   observed defects but spreads a reserved-org branch across handlers and makes
   the auth story harder to audit.
3. **Treat org-scoped session routes as session-member routes.** Route paths
   stay org-scoped for tenancy, while authorization for session resources uses
   `session_members`. Durable org-level surfaces remain org-member gated.

Choose option 3. It matches the system model: operations that touch a concrete
session are authorized by session membership; operations that enumerate or
manage an org are authorized by org membership. The implementation should not
create broad `org_members` rows for anonymous accounts.

## Implementation Units

### Unit 1: Session-resource auth gates

**Story**: `story-playground-anon-access-file-tree-403`

**Files**:
- `internal/portal/sessions/state.go`
- `internal/portal/sessions/files.go`
- `internal/portal/sessions/refmodes.go`
- `internal/portal/sessions/handler.go`

```go
// Example handler guard shape. Each operation still maps AuthFail through its
// own strict-server response wrapper.
acc, member, fail, ok := handlerauth.RequireSessionMember(ctx, h.store, orgID, sessionID)
if !ok {
    if fail.Err != nil {
        return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: <op>: session member: %w", fail.Err))
    }
    return <operation>Fail(fail), nil
}
_ = acc
_ = member
```

**Implementation Notes**:
- Replace the leading `GetOrgMember` checks on concrete session-resource
  endpoints with `handlerauth.RequireSessionMember`.
- Cover at least `GetSession`, `ListSessionRefs`, `GetSessionDigest`,
  `GetSessionFile`, and `UpsertRefMode`. Keep `ListSessions` org-gated because
  it enumerates an org. Keep session-invite creation org-gated because
  playground does not use durable invites and org invites should not be exposed
  to anonymous accounts.
- Add operation-local fail mappers where missing, following
  `authfail-three-branch-guard`.
- Preserve session not-found handling where the handler already fetches the
  session before doing expensive work. It is acceptable for a non-member to see
  403 rather than using session existence as an oracle.

**Acceptance Criteria**:
- [ ] An anonymous bearer whose account is a `session_member` but not an
  `org_member` receives 200 from its own session refs/digest routes.
- [ ] That same bearer receives 403 for another playground session.
- [ ] Durable org listing and invite endpoints remain org-member gated.

### Unit 2: Browser-scoped playground context

**Story**: `story-playground-anon-access-refresh-bounce`

**Files**:
- `frontend/src/lib/auth.svelte.ts`
- `frontend/src/lib/api/client.ts`
- `frontend/src/App.svelte`
- `frontend/src/lib/screens/JoinerPicker.svelte`
- `frontend/src/lib/screens/ResumeExchange.svelte`
- `frontend/src/lib/screens/SessionViewShell.svelte`
- `docs/SPEC.md`
- `docs/ARCHITECTURE.md`

```ts
export type PlaygroundContext = {
  sessionId: string;
  bearer: string;
  nickname: string;
  expiresAt: string;
};

type StoredPlaygroundContext = PlaygroundContext & {
  storedAt: string;
};

const PLAYGROUND_CONTEXT_KEY_PREFIX = 'jamsesh.playground.';

function playgroundContextKey(sessionId: string): string;
function loadStoredPlaygroundContext(sessionId: string): PlaygroundContext | null;
function persistPlaygroundContext(ctx: PlaygroundContext): void;
function clearStoredPlaygroundContext(sessionId?: string): void;

export const auth = {
  get playgroundContext(): PlaygroundContext | null;
  setPlaygroundContext(ctx: PlaygroundContext | null): void;
  restorePlaygroundContext(sessionId: string): boolean;
  clearPlaygroundContext(sessionId?: string): void;
};
```

**Implementation Notes**:
- Persist playground context in browser-scoped `localStorage` under a
  per-session key, not under the durable `jamsesh.token` account key. The
  bearer remains separate from OAuth access/refresh tokens.
- Store `expiresAt` from `PlaygroundSessionCreated.expires_at`,
  `PlaygroundJoinResult.expires_at`, and `ExchangeSessionResume.expires_at`.
  `restorePlaygroundContext` must drop records whose `expiresAt` is in the
  past before returning false.
- On `/orgs/org_playground/sessions/:sessionID`, `App.svelte` should restore
  context synchronously before the auth gate decides. If no local context
  exists, navigate to `/playground/s/:sessionID/join` instead of `/login`.
- `SessionViewShell` should treat a 401 from
  `GET /api/playground/sessions/{id}` as playground expiry/revocation: clear
  the stored context and navigate to `/playground/s/:sessionID/ended`, not
  `/login`.
- `client.ts` should attach the playground bearer only when the request is for
  the active playground session, not for every `org_playground` request in the
  browser. It should also skip durable `auth.signOut()` for 401s on requests
  that used a playground bearer.
- Update the foundation docs after code changes: the current in-memory-only
  description in `docs/ARCHITECTURE.md`/`docs/SPEC.md` must become
  browser-scoped, session-keyed persistence with explicit clearing on expiry,
  revocation, or session end.

**Acceptance Criteria**:
- [ ] Refreshing `/orgs/org_playground/sessions/:sessionID` rehydrates the
  anonymous bearer and stays on the session while the bearer is live.
- [ ] Opening the org-scoped playground URL later in the same browser restores
  the context or sends the user to the playground join/ended route, never the
  durable login route.
- [ ] Durable account tokens and playground bearers remain orthogonal and can
  coexist without one shadowing the other.

### Unit 3: Shared client file fetch

**Story**: `story-playground-anon-access-file-tree-403`

**File**: `frontend/src/lib/components/ArtifactPane.svelte`

```ts
const { data, error } = await client.GET(
  '/api/orgs/{orgID}/sessions/{sessionID}/files',
  {
    params: {
      path: { orgID: orgId, sessionID },
      query: { commit: selectedSha, path: selectedPath },
    },
    signal: controller.signal,
  },
);
```

**Implementation Notes**:
- Replace the manual `fetch()` plus `auth.token` header with the shared typed
  `client`. The manual path cannot attach an anonymous playground bearer and
  bypasses the global auth policy.
- Keep abort semantics so rapid commit/file changes do not write stale content
  into the artifact pane.

**Acceptance Criteria**:
- [ ] The file endpoint request carries the playground bearer for the active
  playground session.
- [ ] Existing binary/too-large/file-not-found UI behavior remains unchanged.

### Unit 4: Playground WebSocket tickets

**Story**: `story-playground-anon-access-ws-live-updates`

**Files**:
- `frontend/src/lib/ws.svelte.ts`
- `frontend/src/lib/session/usePlaygroundCountdown.svelte.ts`

```ts
function bearerForSession(sessionId: string): string | null {
  return auth.playgroundContext?.sessionId === sessionId
    ? auth.playgroundContext.bearer
    : auth.token;
}

async function fetchTicket(sessionId: string): Promise<string | null> {
  const bearer = bearerForSession(sessionId);
  if (!bearer) return null;
  const { data } = await client.POST('/api/auth/ws-ticket', {
    headers: { Authorization: `Bearer ${bearer}` },
  });
  return data?.ticket ?? null;
}
```

**Implementation Notes**:
- Remove the `auth.token`-only guards in `open()` and `reopen()`. Gate on
  `bearerForSession(sessionId)` instead.
- Pass the session ID into `fetchTicket` so the anonymous playground bearer is
  used only for its own session. Durable sessions should still use the durable
  account token.
- No server route change is required for WS tickets: `IssueWsTicket` issues a
  ticket for the account attached by bearer middleware, and the WS gateway
  already checks session membership at upgrade time.

**Acceptance Criteria**:
- [ ] A playground-only browser context fetches a ticket and opens
  `/ws/sessions/:sessionID`.
- [ ] The ticket request uses the anonymous bearer for that playground session.
- [ ] A coexisting durable account token does not shadow the playground bearer
  for playground sockets and is still used for durable sockets.

## Implementation Order

1. `story-playground-anon-access-file-tree-403` - server session-resource auth
   plus shared client file fetch.
2. `story-playground-anon-access-refresh-bounce` - browser-scoped rehydration,
   routing, and foundation-doc contract update.
3. `story-playground-anon-access-ws-live-updates` - WS ticket fetch and
   reconnect behavior using the rehydrated context.

## Testing

- Backend: add per-dialect session handler tests that seed an anonymous account
  as a playground `session_member` without an `org_member`, then assert own
  refs/digest access succeeds and cross-session access fails.
- Backend: keep existing durable org membership tests for `ListSessions` and
  invite creation green.
- Frontend unit: extend `auth.test.ts` for persisted playground context,
  expiry pruning, restore, and clear.
- Frontend unit: extend `client.test.ts` for session-specific playground bearer
  selection and no durable sign-out on playground 401.
- Frontend unit: extend `App.test.ts` and `SessionViewShell.test.ts` for
  org-scoped playground reload/direct-entry flows.
- Frontend unit: extend `ws.test.ts` for playground-only ticket fetch, durable
  token coexistence, and reconnect using a fresh playground ticket.

## Risks

- Browser-scoped bearer persistence increases the impact of a local browser
  compromise compared with in-memory-only state. The risk is bounded by
  anonymous, session-scoped bearers, hard-cap expiry, revocation on destruction,
  and per-session storage keys.
- The direct `ArtifactPane` fetch currently bypasses the shared client. If it is
  not converted with the auth work, the tree may load while file preview still
  fails for anonymous participants.
- The 401 middleware currently treats `auth.*` failures as durable sign-out.
  Missing the playground exception would preserve the login bounce even after
  persistence is added.

## Implementation Summary

- Concrete session-resource endpoints now authorize with session membership
  rather than org membership, keeping anonymous playground accounts out of
  `org_members` while allowing their own session refs, digest, files, and ref
  mode writes.
- The SPA persists playground context in browser-scoped, session-keyed storage
  with `expiresAt` validation and restores it before the auth gate on
  org-scoped playground session routes.
- Playground 401s clear playground context and route to join/ended states
  without durable sign-out.
- Artifact file fetches use the shared typed client, and WebSocket ticket
  fetches use explicit session-specific bearer selection.

## Verification

- `go test ./internal/portal/sessions`
- `npm test -- --run src/lib/auth.test.ts src/lib/api/client.test.ts src/lib/components/ArtifactPane.test.ts src/App.test.ts src/lib/screens/SessionViewShell.test.ts src/lib/ws.test.ts`
- `npm run check` (0 errors, 1 pre-existing Svelte warning in `ModeSwitchDialog.svelte`)

## Review (2026-06-01)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Deep substrate feature review. Covered correctness, tests, design
alignment, lightweight auth/security, public contract behavior, foundation-doc
alignment, and browser/WS lifecycle risks. Fresh-context review is deferred to
the autopilot final peer-review loop rather than duplicated here. Child stories
are approved and done.
