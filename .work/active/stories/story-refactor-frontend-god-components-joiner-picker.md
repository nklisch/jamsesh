---
id: story-refactor-frontend-god-components-joiner-picker
kind: story
stage: review
tags: [ui, refactor]
parent: feature-refactor-frontend-god-components
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Decompose JoinerPicker into picker + invitation-list components

## Brief

`frontend/src/lib/screens/JoinerPicker.svelte` is 580 lines combining
two distinct concerns: a session picker and an invitation-acceptance
flow.

## Extraction targets

Read the file first. Likely splits:

1. **`InvitationList.svelte`** — invitation-acceptance UI. Owns the
   list of pending invites, the accept/decline handlers, and the
   error states for that flow.

2. **`useInvitationAccept.svelte.ts`** — rune module for invitation
   state if the InvitationList component grows substantial state.

3. **`SessionPicker.svelte`** — leaves the picker as the screen's
   primary concern, possibly extracting only the picker-list rendering
   if it's a clear sub-region.

The split target is the **two distinct concerns** — picker vs
acceptance. Extracting either one as a sub-component is sufficient.

## Acceptance criteria

- [ ] `JoinerPicker.svelte` LoC ≤ 300.
- [ ] At least one of the two concerns extracted to its own component
      (preferably the invitation-acceptance flow, as it's typically the
      more state-heavy).
- [ ] No visible UI change — same picker behavior, same acceptance
      flow.
- [ ] `npm run check` clean.
- [ ] `npm run test` passes.
- [ ] `npm run build` clean.

## Risk

**Low.** Two concerns with a clear boundary.

## Rollback

`git revert` the commit.

## Implementation notes

**Actual concerns identified:** The 580-line file split cleanly along two lines:

1. **Nickname picker / join form** (`idle` + `joining` states) — extracted to
   `frontend/src/lib/components/JoinerForm.svelte`. Owns: nickname state,
   avatar derivation, wordlist generator, reroll, validation, and submit
   handler. Props: `viewState: 'idle' | 'joining'`, `onjoin(nickname)`,
   `onnavplayground()`.

2. **Join outcome states** (`full` + `error` states) — extracted to
   `frontend/src/lib/components/JoinerOutcome.svelte`. Owns: the session-full
   and generic-error panels. Props: `viewState: 'full' | 'error'`, `errorMsg`,
   `onretry()`, `onnavplayground()`.

**JoinerPicker.svelte** is now a thin screen orchestrator: imports both
components, owns the ViewState machine and the async `handleJoin` function
(API call, auth context write, navigation), delegates all rendering.

**LoC delta:** 580 → 162 lines on JoinerPicker.svelte (−418 lines).
JoinerForm.svelte: 260 lines. JoinerOutcome.svelte: 118 lines.

**No shared rune module needed:** state is not shared between picker and
outcome — they are mutually exclusive branches. Props + callbacks suffice.

**Test result:** All 22 JoinerPicker tests pass unchanged — the test renders
the screen component and the extracted sub-components are transparent to it.
`npm run check` 0 errors. `npm run build` clean.
