---
id: story-refactor-frontend-god-components-org-settings
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

# Decompose OrgSettings into member list + invite form + info editor

## Brief

`frontend/src/lib/screens/OrgSettings.svelte` is 555 lines spanning
org info edit, member list, invitations, and deletion — four
distinct concerns in one screen.

## Extraction targets

Read the file first. Strong candidates:

1. **`OrgMemberList.svelte`** — member list rendering + removal
   handlers. Self-contained.

2. **`OrgInviteForm.svelte`** — invitation send form (email input,
   submit, error states).

3. **`OrgInfoEditor.svelte`** — name / slug / session_invite_policy
   editor.

4. **`OrgDangerZone.svelte` (optional)** — deletion flow if it's
   substantial enough to warrant separation.

Each extracted component takes props for what it needs and emits
events for actions; the screen orchestrates them.

## Acceptance criteria

- [ ] `OrgSettings.svelte` LoC ≤ 300.
- [ ] At least 2 of the 4 candidate components extracted (preferably
      MemberList + InviteForm, as those are the most state-heavy).
- [ ] No visible UI change — same edit flow, same member operations,
      same invitation send.
- [ ] `npm run check` clean.
- [ ] `npm run test` passes; existing OrgSettings tests pass with
      minimal adjustment.
- [ ] `npm run build` clean.

## Risk

**Low.** Distinct, well-bounded concerns.

## Rollback

`git revert` the commit.
