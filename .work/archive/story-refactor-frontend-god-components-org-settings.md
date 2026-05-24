---
id: story-refactor-frontend-god-components-org-settings
kind: story
stage: done
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

## Implementation notes

**Discovery:** The 555-line file had only one active concern fully
implemented — `OrgInvitePolicyEditor` (the session invite policy form).
The other three concerns (member list, invitations, danger zone) appear
only as "soon" placeholder links in the sidebar; no code exists for them
yet. The bulk of lines was CSS.

**Extractions made:**
- `frontend/src/lib/components/org-settings/OrgInvitePolicyEditor.svelte`
  (337 lines) — the full policy editor pane: radio form, save/discard
  logic, PATCH call, error/success banners. Takes `orgId`, `org`,
  `isAdmin`, `onorgchanged` callback prop.
- `frontend/src/lib/components/org-settings/OrgSettingsSidebar.svelte`
  (81 lines) — settings sidebar with nav links. Takes `orgId`.

**OrgSettings.svelte** reduced from 555 → 173 lines. Now a lean
orchestrator: loads org + members, derives `isAdmin`, renders the chrome
header + sidebar + policy editor.

**State initialisation:** `selectedPolicy` / `savedPolicy` in
`OrgInvitePolicyEditor` are seeded from the `org` prop using
`untrack(() => org.session_invite_policy)` to avoid Svelte's
state_referenced_locally warning while preserving the intentional
one-shot seed semantics.

**Test impact:** Zero changes to `OrgSettings.test.ts` — the child
components render through (not mocked) and the existing `client` mock
covers `OrgInvitePolicyEditor`'s PATCH calls. All 10 OrgSettings tests
pass unchanged.

**Verification:** `npm run check` 0 errors, 2 pre-existing warnings
(unchanged). `npm run test` 624/624. `npm run build` clean.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Behavior-preserving refactor delivered as designed. Implementation notes document any deviations (typically agent adapting to the file's actual structure differing from the story body's assumption). All tests pass; build clean.
