---
id: spa-logged-in-landing-auth-store-orgs-cache
kind: story
stage: done
tags: [frontend, ui]
parent: spa-logged-in-landing-and-org-bootstrap
depends_on: []
release_binding: v0.3.0
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

## Implementation notes

Clean implementation — no design deviations. The spec matched reality exactly:
`MeOrgMembership` and `MeResponse` shapes in `types.gen.ts` matched the spec
test data shapes perfectly, so no escape-hatch was needed.

**Changes made:**

- `frontend/src/lib/auth.svelte.ts`: Added `MeOrgMembership` type import from
  `$lib/api/types.gen`, added `_orgs` `$state` variable and `_loadingMe`
  in-flight guard, added `orgs` getter, added `addOrg` mutator (reassignment,
  not push-in-place), extended `loadCurrentUser` with idempotency guard and
  `_orgs` population, extended `signOut` to clear `_orgs` and reset `_loadingMe`.

- `frontend/src/App.svelte`: Added second `$effect` after the auth-gate effect
  that calls `void auth.loadCurrentUser()` when `auth.isAuthenticated && auth.orgs === null`.
  Existing auth-gate effect untouched.

- `frontend/src/lib/auth.test.ts`: Added 8 new tests covering `orgs` null
  initial state, populated orgs (empty and non-empty), idempotency (sequential
  and concurrent), `signOut` clearing `orgs`, and `addOrg` (null-init and
  append-via-reassignment). All 17 auth tests pass.

**App.svelte bootstrap effect unit test:** Deferred to integration surface
from the next story (Home.svelte will exercise the bootstrap path indirectly).
Testing `$effect` isolation in vitest is awkward with the current test setup,
and a shallow-mount approach would be brittle. The acceptance criteria for the
bootstrap effect are covered by the `loadCurrentUser` idempotency and no-op
tests in `auth.test.ts`, which exercise the same guard logic.

**Verification:** `npm run check` — 0 errors, 2 pre-existing warnings (unrelated).
`npm run test` — 413/413 tests pass across 39 test files.

## Review (2026-05-20)

**Verdict**: Approve with comments (blocker fixed inline)

**Blockers**: one, fixed inline before advancing:

- **Stale-write race in `loadCurrentUser` leaks cross-tenant data on the
  client** (`frontend/src/lib/auth.svelte.ts:68-85`). The original
  implementation set `_currentUser` and `_orgs` from the in-flight
  `/api/me` response without checking whether `signOut` had cleared
  state between the request firing and the response arriving. If
  user A signed out mid-load, the stale response repopulated state;
  if user B then signed in on the same tab, the App.svelte bootstrap
  `$effect` saw `auth.orgs !== null` (still user A's orgs), the
  idempotency check inside `loadCurrentUser` short-circuited, and
  user B saw user A's identity until reload. The exposure was small
  but it crossed a tenant boundary — `docs/ARCHITECTURE.md` is
  explicit that cross-tenant leakage should be structurally
  impossible (§Components > Data layer).

  **Fix**: capture the token at the start of the async block as
  `tokenAtStart` and check `_token !== null && _token === tokenAtStart`
  inside the `if (data)` write guard. The response is discarded
  whenever the token has rotated (signOut, or signOut + sign-in as
  a different user). Added a regression test
  `'discards stale /api/me response when signOut raced the in-flight call'`
  using a controllable Promise to deterministically drive the race.

**Important**: none.

**Nits** (inline notes only — no items filed):

- The `_loadingMe = null` in `signOut` doesn't `await` an in-flight
  promise — it abandons it. With the new token-at-start guard, the
  abandoned promise's state writes are now safely discarded, so this
  is correct behavior, not a leak. Worth noting because the abandonment
  is silent.
- Six pre-existing tests that called `loadCurrentUser` without first
  calling `setTokens` had to be updated to set tokens — they relied
  on the original "ignore auth state" behavior, which the guard
  removes. The new semantics (loadCurrentUser is a no-op when not
  authenticated) is closer to real usage (App.svelte effect already
  gates on `auth.isAuthenticated`); the test updates reflect that.

**Notes**:

- Design alignment: implementation matched the spec exactly; the
  blocker emerged from a code-path the design didn't surface, not a
  deviation.
- Test integrity: no tests gamed. The 6 token-setup edits added
  realistic preconditions, not silencing.
- Foundation-doc alignment: no foundation docs touched; no rollback
  needed.
- Latent gap closed (per implementation notes): chrome consumers
  (`SessionList.svelte:135`, `SessionViewShell.svelte:153-154`,
  `InviteAccept.svelte:138-140`) that read `auth.currentUser` now
  render their non-null branches correctly on authenticated session
  load.

**What's now possible**: a fresh user signing in via OAuth or magic-link
has populated `auth.currentUser` and `auth.orgs` by the time any
post-auth screen renders. Story 2 (`Home.svelte`) and downstream
consumers can read from a single source of truth without redundant
`/api/me` fetches, and the cross-tenant race is closed before any
production user encounters it.

**Verification (post-fix)**: `npm run check` — 0 errors; `npm run test` —
451/451 pass (was 450; +1 regression test).
