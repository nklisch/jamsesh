---
id: org-session-invite-policy-org-settings-ui
kind: story
stage: implementing
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
