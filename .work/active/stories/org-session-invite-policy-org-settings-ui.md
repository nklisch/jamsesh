---
id: org-session-invite-policy-org-settings-ui
kind: story
stage: done
tags: [ui]
parent: org-session-invite-policy
depends_on: [org-session-invite-policy-patch-endpoint]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# OrgSettings.svelte — Sidebar nav + content pane

New screen at `/orgs/:orgID/settings`. Implements the chosen mockup
**Option 2 — Sidebar nav + content pane** (see
`.mockups/screens/org-session-invite-policy-settings/option-2.html`).
Admin (org creator) sees an editable Save flow; non-admin sees a read-only
view with disabled controls and an explanatory note.

## Files

- New: `frontend/src/lib/screens/OrgSettings.svelte`
- New: `frontend/src/lib/screens/OrgSettings.test.ts`
- Modify: `frontend/src/lib/router.svelte.ts` — add the new route
- Modify: `frontend/src/App.svelte` — add the screen render branch

## Reference mockup

`.mockups/screens/org-session-invite-policy-settings/option-2.html`

Honors the sidebar/content layout, the `nav.dim` styling for future-soon
sections, the warning-tinted read-only banner, and the per-page Save
in the bottom-right of the pane.

## Routing

Add a route in `frontend/src/lib/router.svelte.ts`:

```ts
{ pattern: /^\/orgs\/([^/]+)\/settings$/, name: 'org-settings', params: ['orgId'] },
```

Order matters because of first-match semantics. Place AFTER more specific
patterns under `/orgs/:orgID/sessions/...` but BEFORE the catch-alls.

In `App.svelte`:

```svelte
{:else if current.name === 'org-settings'}
  <OrgSettings orgId={current.params.orgId} />
```

## Component contract

```ts
type Props = {
  orgId: string;
};
```

Loads the org's current policy via `GET /api/orgs/{orgID}` on mount.
Tracks admin status by reading `auth.currentUser` against the org's
members (or by leaning on a backend field — if `Me.orgs[X].role` is
already exposed in `MeResponse.orgs`, use that; that's simpler than
querying separately).

On Save, calls `PATCH /api/orgs/{orgID}` with the new policy. On success,
updates local state and shows a transient "Saved" affordance. On 403,
surfaces "Only org creators can change this setting" inline. On other
error, surface the message in the warning banner.

## States to handle

- Loading (initial fetch)
- Admin: editable, Save enabled when dirty
- Non-admin: read-only with disabled radios + Save and the
  warning-tinted "Only org creators can change this setting" banner
- Save in-flight (disable controls briefly)
- Save error (banner)
- Save success (transient toast or inline check)

## Acceptance criteria

- [ ] Route `/orgs/:orgID/settings` renders the screen
- [ ] Admin sees the sidebar nav (active "Session invites", dimmed future
      sections), the policy radio, and an enabled Save button
- [ ] Non-admin sees the same layout with disabled radios + disabled Save
      + the warning-tinted explanation banner
- [ ] Save calls `PATCH /api/orgs/{orgID}` and updates local state on 200
- [ ] Save handles 403 (shouldn't happen for admins; defensive)
- [ ] `npm test -- --run OrgSettings` passes
- [ ] `npm run check` clean
- [ ] Full suite passes (no regressions)

## Risk

LOW-MEDIUM. New route + screen + auth-state read. The admin-vs-member
discrimination needs to work consistently with how the rest of the app
checks roles — verify the existing pattern (look at how SessionList or
SessionViewShell gate admin actions; if they don't, this is the first
gate of its kind and the implementer should pick a clean approach).

## Rollback

`git revert` the commit. The route disappears; the patch endpoint stays
(idempotent).

## Implementation notes

### GET /api/orgs/{orgID} — added as part of this story

The design assumed GET existed; it didn't. Added inline:

- `docs/openapi.yaml` — new `get:` operation under `/api/orgs/{orgID}` with
  `operationId: GetOrg`. Requires bearer + org membership; returns `Org`.
- `internal/portal/accounts/orgs.go` — `GetOrg` handler using
  `RequireOrgMember` then `store.GetOrgByID`; mirrors `PatchOrg` shape.
- `internal/portal/accounts/orgs_test.go` — 4 Go tests:
  NoBearer→401, NotMember→403, CreatorSuccess→200, MemberSuccess→200.
- `internal/portal/accounts/handlers_test.go` — `GetOrg` added to
  `accountsOnlyStrict` (delegates to `accounts.Handler`).
- `cmd/portal/main.go` — `GetOrg` route + `combinedHandler` delegation.
- Regenerated `internal/api/openapi/server.gen.go` and
  `frontend/src/lib/api/types.gen.ts`.

### Admin determination

`auth.currentUser` carries no org role. Used `listOrgMembers` on mount
(parallel with GetOrg) and matched `account_id === auth.currentUser.id`
to find `role === 'creator'`. Clean one-round-trip approach.

### OrgSettings.svelte

Svelte 5 runes (`$state`, `$derived`, `onMount`). Layout matches Option 2
mockup exactly: sidebar nav with 4 items (Session invites active; Members,
Billing, API keys dimmed with "soon" badge); content pane with radios,
pane-actions row at bottom-right. Warning-tinted banner for non-admin and
for save errors. Transient "Saved" success span with 2s auto-dismiss.

### Test debt fixed

`frontend/src/lib/components/finalize/RefGroupList.test.ts` had 6
pre-existing `svelte-check` errors from `new Set()` being inferred as
`Set<unknown>` instead of `Set<string>`. Fixed in-session with
`new Set<string>()` at all call sites. All 343 tests still pass.

### Verification

- `go build ./...` — clean
- `go test ./internal/portal/accounts/... -count=1` — pass (all prior +
  4 new GetOrg tests)
- `npm test -- --run OrgSettings` — 10/10 pass
- `npm run check` — 0 errors, 2 pre-existing warnings

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- Adding `GetOrg` to `StrictServerInterface` required panic stubs on 5 more
  `*OnlyStrict` test doubles than the agent caught initially. Orchestrator
  patched the rest in commit `4681f53`. A pattern worth noting: when adding
  a new endpoint to the strict interface, grep for `var _ openapi.StrictServerInterface = (*` and update every match.
- Pre-existing `Set<unknown>` errors in `RefGroupList.test.ts` were fixed
  inline. Reasonable — they were blocking `npm run check` cleanliness.
- The 555-line component is dense but well-organized (parallel
  `Promise.all` for load, `$derived` dirty tracking, transient success
  via timer). No extraction needed yet.

**Notes**: Backend admin check is independently re-validated by `PatchOrg`,
so the client-side `isAdmin` flag is UX-only (which is correct). The
sidebar nav with dimmed future sections sets a clean scaffold for upcoming
Members/Billing/API-keys sections — anticipates growth without
restructuring. Filling the design gap by inlining the GET endpoint
(rather than blocking on a separate story) was the right call.
