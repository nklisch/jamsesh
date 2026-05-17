---
id: epic-finalize-flow-portal-ui-curation-view
kind: feature
stage: drafting
tags: [ui]
parent: epic-finalize-flow
depends_on: [epic-finalize-flow-plan-generation, epic-portal-ui-foundation, epic-portal-ui-session-view-shell]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Finalize Flow — Portal UI Curation View

## Brief

A dedicated full-page route in the portal UI for finalize curation —
separate from the always-on session view. Lets a human reviewer curate
which commits go into the final branch, name the target branch, review
the plain-English summary, and copy the one-line `jamsesh finalize-run
<plan-id>` command.

**Route**: `/sessions/<session-id>/finalize` — opens after the user
hits "Finalize" from the session view's header.

**Lifecycle on the page**:

1. Acquire lock (`POST /finalize/lock`). If another member holds the
   lock, show "Alice is finalizing — wait for her, or override
   (which restarts curation from the current draft)." Override is a
   button that calls `POST /finalize/lock` with `override: true`.
2. Default selection: every commit currently reachable from `draft`
   in chronological order. Plus a panel of isolated refs and their
   commits that the user can toggle into the selection.
3. Curation controls:
   - Toggle individual commits on/off (granularity locked at
     epic-design — individual commits, not whole refs)
   - Reorder via drag (or up/down buttons for accessibility)
   - Target branch name input — required field; suggested default
     `jamsesh/<session-name-slug>`.
4. The curation state is sent to the portal via PATCH on every
   change (debounced); this resets the 30-min lock timer.
5. The plain-English summary panel re-fetches `GET /finalize-plan`
   on every curation change and renders:
   > This will create a branch `<target-name>` from base commit
   > `<sha>` in your local source-repo checkout, then cherry-pick
   > these N commits in order: <list with author / message / sha>.
   > Conflicts during cherry-pick will be left in your working tree
   > for you to resolve. Nothing will be pushed.
6. "Run locally" button — large, prominent. Copies
   `jamsesh finalize-run <plan-id>` to clipboard with a confirmation
   toast.
7. After the user has run the cherry-pick locally and pushed to their
   source remote, they return to this view (or the session view) and
   click "Mark as shipped." That fires `POST /mark-shipped` and
   transitions the session to `status: ended` with
   `end_reason: shipped`. Until clicked, the page shows "Ready to
   mark as shipped" prominently.

**Cross-cutting UI elements**: reuses base components from
`epic-portal-ui-foundation` (typed REST client, WebSocket primitive,
chrome, theme toggle) and `epic-portal-ui-design-system` (Button,
Card, Pill, AuthorDot, InlineCode). Navigation between session view
and finalize view follows the patterns established in
`epic-portal-ui-session-view-shell`.

**WebSocket integration**: subscribes to session events while the page
is open. If another member overrides the lock or marks shipped, the
view reacts (banner appears, controls disable).

## Mockups

This is net-new UI surface for v1. The feature-design pass should run
`/ux-ui-design:screens epic-finalize-flow-portal-ui-curation-view` to
explore 3-4 options for the curation layout (commit-list selection +
target-branch input + summary panel) before locking the direction.
References `.mockups/design-system/tokens.css` for palette/typography
and inherits the chrome treatment from the locked onboarding flow.

Does NOT cover the plan generation backend (plan-generation feature).
Does NOT cover the `jamsesh finalize-run` execution (plugin feature).

## Epic context

- Parent epic: `epic-finalize-flow`
- Position in epic: the human-facing curation surface. Depends on
  plan-generation for API, on portal-ui-foundation for primitives, on
  session-view-shell for navigation context.

## Foundation references

- `docs/UX.md` — Flow: finalizing (the canonical user journey)
- `docs/ARCHITECTURE.md` — Reconciliation (local)
- `.mockups/design-system/tokens.css` — locked palette + typography

## Inherited epic design decisions

- **Concurrent-finalize UX**: explicit "Alice is finalizing — wait or
  override" panel; override is a button.
- **Mark-as-shipped is manual**: prominent "Ready to mark as shipped"
  prompt until clicked.
- **Curation granularity**: individual commits, not whole refs.
- **Plan-id format**: opaque from the client's perspective.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
