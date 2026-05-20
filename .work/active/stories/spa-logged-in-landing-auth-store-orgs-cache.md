---
id: spa-logged-in-landing-auth-store-orgs-cache
kind: story
stage: implementing
tags: [frontend, ui]
parent: spa-logged-in-landing-and-org-bootstrap
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
---

# Auth store extension + bootstrap effect

## Scope

Extend `auth.svelte.ts` to cache the full `MeResponse` shape (currently only
id/email/displayName captured). Add a reactive `orgs` getter, an `addOrg`
mutator for the create-flow to update without a refetch, idempotency
guarding on `loadCurrentUser`, and an `App.svelte` `$effect` that triggers
the load on cold-start.

Today nothing in production code calls `auth.loadCurrentUser()` — the
function exists but no `auth.currentUser` consumer ever fires. This
story closes that gap as a side effect of wiring the bootstrap.

See parent feature `## Implementation Units > Unit 1` for the full
specification (file paths, store shape, bootstrap-effect body, edge cases).

## Files

- `frontend/src/lib/auth.svelte.ts` (edit)
- `frontend/src/lib/auth.test.ts` (edit — extend coverage; do not delete
  existing cases)
- `frontend/src/App.svelte` (edit — add a second `$effect` for bootstrap;
  the existing auth-gate effect stays untouched until Unit 3)

## Acceptance Criteria

- [ ] `auth.orgs` returns `null` before `loadCurrentUser()` resolves and an
      `MeOrgMembership[]` after (possibly empty array, never `null` once
      resolved).
- [ ] `auth.loadCurrentUser()` is idempotent — two concurrent calls fire
      exactly one fetch; calls after resolution are no-ops.
- [ ] `auth.signOut()` clears `_orgs` to `null` and resets `_loadingMe`.
- [ ] `auth.addOrg(org)` appends `org` to `_orgs`, creating the array when
      `_orgs` was `null`, via reassignment (not in-place push).
- [ ] App.svelte's new effect calls `loadCurrentUser()` once on cold-load
      when `auth.isAuthenticated && auth.orgs === null` (verify via
      fetch-mock call count = 1).
- [ ] App.svelte's new effect does NOT call `loadCurrentUser()` when
      `auth.isAuthenticated && auth.orgs !== null`.
- [ ] All existing `auth.test.ts` cases still pass after the shape change.
- [ ] `npm run check` (or project equivalent) passes — type-check covers
      the new `MeOrgMembership` import path.
- [ ] `npm run test` passes.

## Notes

- The `MeOrgMembership` type comes from the generated
  `frontend/src/lib/api/types.gen.ts` — use `components['schemas']['MeOrgMembership']`.
- This story does NOT touch any screen — chrome consumers (`SessionList`,
  `SessionViewShell`, `InviteAccept`) start rendering `auth.currentUser`
  properly as a side effect, which is the intended outcome.
- Do NOT add a UI for re-triggering `loadCurrentUser` on failure. The
  retry path is "App.svelte effect fires whenever auth flips" plus
  whatever screen-level retry the next story adds.
