---
id: feature-epic-ephemeral-playground-portal-ui
kind: feature
stage: drafting
tags: [ui, portal, playground]
parent: epic-ephemeral-playground
depends_on: [feature-epic-ephemeral-playground-session-lifecycle]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Portal UI — playground surfaces + CLI-first creation rework

## Brief

Adds every portal-frontend surface the playground epic needs and reworks
the existing "New session" flow to align with the CLI-first creation
pattern. New surfaces: unauthenticated portal landing
(`/playground/...`), joiner nickname picker, anonymous-mode chip and
countdown badge added to `SessionViewShell` chrome (rendered only when
the session's org is the reserved playground org), idle / hard-cap
warning banners, post-destruction confirmation page. Routes are added
to `router.svelte.ts`; the auth gate in `App.svelte` is extended to
exempt the playground route group.

The `NewSessionDrawer.svelte` rework is the design decision this feature
resolves. Two options under consideration (per the parent epic's
strategic-decisions section, which folded in the CLI-first unification):
(A) keep the drawer as an alternative path that collects inputs and then
prints a `jamsesh new ...` CLI command for the user to run locally; or
(B) keep the drawer for users with `JAMSESH_CLI_FIRST_OPTIONAL=true` and
otherwise hide it. The choice is made in this feature's design pass
based on what the CLI-first creation feature actually ships.

Auth state in `auth.svelte.ts` gains a `_playgroundContext` rune field
that tracks whether the current view is anonymous-mode and which
anonymous bearer is in use; the auth gate consults this when deciding
which redirect a 401 triggers.

## Epic context
- Parent epic: `epic-ephemeral-playground`
- Position in epic: **wave 3** — depends on `session-lifecycle` for the
  REST routes the UI calls. Parallelizable with `plugin-skills`.

## Foundation references
- `docs/UX.md` § Flow: creating a session — this feature's design pass
  rolls UX.md forward to describe the unified CLI-first creation flow
  (alongside any retained portal-form path) AND adds the playground
  onboarding flow as a first-class section
- `docs/ARCHITECTURE.md` § Portal frontend — minor: any new top-level
  route group or auth-state shape change is noted

## Mockups
- **Inherits parent epic flow**:
  `.mockups/flows/playground-onboarding/index.html`
- Every user-visible surface this feature ships is covered there:
  step 01 (prospect landing), step 03 (creator session with chip +
  countdown), step 05 (joiner nickname picker), step 06 (joiner
  session), step 07 (warning banners + post-destruction page)
- **Do NOT re-mock at the feature tier.** The flow is the source of
  truth for visual decisions on this feature
- If the `NewSessionDrawer` rework chooses option A (CLI-prompt
  output), that's a new screen state not covered in the parent flow
  — invoke `/ux-ui-design:screens` for that specific surface during
  this feature's design pass
