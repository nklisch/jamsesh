---
id: story-refactor-frontend-god-components-joiner-picker
kind: story
stage: implementing
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
