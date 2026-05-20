---
id: gate-docs-ux-md-home-landing-surface
kind: story
stage: review
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: docs
created: 2026-05-20
updated: 2026-05-20
---

# `docs/UX.md` Portal UI surfaces list omits the new home/org-landing surface

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/UX.md:208-229`
- Code: `frontend/src/lib/screens/Home.svelte:1-67`, `frontend/src/lib/router.svelte.ts:7`

## Current doc text
> ## Portal UI surfaces
>
> The portal UI has these primary surfaces. (Concrete designs land in
> `.mockups/screens/` as built.)
>
> - **Session list** — sessions visible to the user, grouped by status
>   (active, finalizing, ended).
> - **Session view** — the main work surface for a single session: …
> - **Comment composer** — overlay on the artifact pane …
> - **Finalize view** — appears when finalize is initiated. …
> - **Settings** — per-session: invitees, scope (widen), default mode, …
> - **Admin** — org-level: members, invitations, billing (hosted), configuration (self-host).

## Reality
The SPA now ships a `/` Home surface as the post-auth landing — it
renders three sub-states (loading from `/api/me`; empty: welcome
heading + inline create-org form; picker: list of `auth.orgs` with role
badges + inline create-another form) and auto-routes to
`/orgs/{id}/sessions` when the user has exactly one org. Org creation
is a primary user-facing flow on this surface (calls `POST /api/orgs`
and navigates into the new org). This is a non-org-scoped primary
surface that the UX surfaces inventory does not list.

## Required edit
Add a bullet to `## Portal UI surfaces` for the home/landing surface
and document the org-bootstrap flow it ships. Insert BEFORE "Session
list" since it is the post-auth root. Suggested wording:

> - **Home (post-auth landing at `/`)** — the SPA's first non-org-scoped
>   surface after sign-in. Renders one of three states from
>   `GET /api/me`'s `orgs[]`: empty (no memberships) shows an inline
>   "name your org" form that calls `POST /api/orgs` and lands the new
>   creator in `/orgs/{id}/sessions`; single-org auto-routes straight
>   into `/orgs/{only-id}/sessions`; multi-org shows a workspace picker
>   with role badges and an inline "create another org" form.

Then extend the existing `## Flow: creating a session` section's
preamble or add a new `## Flow: creating your first org` section to
capture the in-SPA bootstrap path (today the doc only describes session
creation, not org creation, and assumes the user is already in an org).

## Implementation notes

### What was added to `docs/UX.md`

**Bullet added to `## Portal UI surfaces`** (inserted before "Session list",
now the first bullet in that section):

- **Home (post-auth landing at `/`)** bullet describing all four auth.orgs
  states: null (loading), empty array (inline create-org form), single entry
  (auto-route), two-or-more (workspace picker + create-another form). Includes
  concrete file references: `frontend/src/lib/router.svelte.ts` (router name
  `home`) and `frontend/src/lib/screens/Home.svelte`.

**New flow section added** (`## Flow: creating your first org`) inserted
immediately before `## Portal UI surfaces`. Covers: empty-state render →
user submits name → `POST /api/orgs` → `auth.addOrg(...)` + navigate to
`/orgs/{new-id}/sessions` → lands in session list. Includes inline error
handling note and cross-reference to "Flow: creating a session".

### Drift noticed

The story body describes the loading state as "loading from `/api/me`". In
`Home.svelte` the loading state is driven by `auth.orgs === null` — the
component never calls `/api/me` directly; the auth store handles that. The
bullet was written to match the implementation: "populated from
`GET /api/me`'s `orgs[]`" preserves the accurate API reference while
making clear the component reads `auth.orgs` rather than making a direct
fetch call.

The story's suggested wording references only three states (empty, single,
multi); the actual component has four: `null` (loading), empty array,
single entry, and two-or-more. The bullet documents all four.

The single-org condition in the code is `auth.orgs.length === 1` (exact
equality); the multi-org picker branch is `auth.orgs.length >= 2`. Both
are documented accurately.
