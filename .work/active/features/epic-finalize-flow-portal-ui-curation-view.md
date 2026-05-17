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
updated: 2026-05-17
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
2. Default selection: the leaf agent commits reachable from `draft`
   via first-parent walk (the portal API does the linearization —
   the UI just renders the list it gets back; merge commits never
   appear). Plus a panel of isolated refs and their commits that
   the user can toggle into the selection.
3. **Finalization-mode bar at the top** — segmented control with
   `Squash into one commit` (default) and `Preserve all commits`.
   The chosen mode is sent to the portal via the `mode` field of
   PATCH and persisted on the lock.
4. Curation controls (both modes):
   - Toggle individual commits on/off (granularity locked at
     epic-design — individual commits, not whole refs)
   - Reorder via drag (or up/down buttons for accessibility). In
     squash mode, order drives the bulleted list in the commit
     message body; in preserve mode, order drives the cherry-pick
     sequence.
   - Target branch name input — required field; suggested default
     `jamsesh/<session-name-slug>`.
5. **Squash-mode-only fields**:
   - Commit message editor (textarea, monospaced). Pre-populated
     from the plan response's `commit_message` — subject = session
     goal (truncated to 72 chars), body = bulleted commit subjects
     in order, footer = `Co-authored-by` trailers. Edits round-trip
     through PATCH and are reflected back in `commit_message` on
     subsequent plan fetches.
   - Co-author chip row — read-only, shows every distinct
     contributor across the current selection (colored author dot
     + name). Updates as commits are added/removed.
6. The curation state is sent to the portal via PATCH on every
   change (debounced); this resets the 30-min lock timer.
7. The plain-English summary panel re-fetches `GET /finalize-plan`
   on every curation change. Squash-mode rendering:
   > This will create a branch `<target-name>` from base commit
   > `<sha>` in your local source-repo checkout, then squash
   > <N> commits from <M> authors into one commit titled
   > "<subject>" with a `Co-authored-by` trailer for each
   > contributor. Conflicts during the squash will be left in your
   > working tree for you to resolve. Nothing will be pushed.

   Preserve-mode rendering uses the previous N-commit cherry-pick
   wording.
8. "Run locally" button — large, prominent. Copies
   `jamsesh finalize-run <plan-id>` to clipboard with a confirmation
   toast.
9. After the user has run the squash/cherry-pick locally and pushed
   to their source remote, they return to this view (or the session
   view) and click "Mark as shipped." That fires `POST
   /mark-shipped` and transitions the session to `status: ended`
   with `end_reason: shipped`. Until clicked, the page shows "Ready
   to mark as shipped" prominently.

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

Four layout directions explored at epic-design's `--only-mocks` prep pass
(2026-05-17); **Option 3 (cart pattern) is the chosen direction**.

- `.mockups/screens/epic-finalize-flow-portal-ui-curation-view/index.html`
  — navigator
- **`option-3.html` — Cart pattern (CHOSEN)**: source pool left grouped
  by ref (draft + isolated refs as expandable cards), "your final branch"
  cart right with explicit add/remove/reorder. Top mode bar with
  Squash/Preserve segmented control (squash pre-selected). Squash mode
  shows the commit-message editor + co-author chip row inside the cart
  config block.
- Reference (not chosen): `option-1.html` (spreadsheet), `option-2.html`
  (two-column rivers), `option-4.html` (tree-rooted). Kept for comparison
  if the chosen direction needs re-evaluation.

Feature-design should mock these state variants of the cart layout:
- Lock-conflict ("Alice is finalizing — wait or override") banner state
- Preserve-mode variant (mode bar flipped; cart shows "Cherry-pick order"
  semantics; commit-message editor + co-author chips hidden)
- Post-execution "Ready to mark as shipped" state
- Empty selection / single-commit edge cases

All mocks use the locked `.mockups/design-system/tokens.css` palette and
match the chrome treatment of `epic-portal-ui-session-view-shell`.

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
- **Linearized merge handling** (from `--only-questions` pass): the
  plan response from `GET /finalize-plan` returns leaf agent commits
  only (auto-merger merge commits are walked through, not displayed).
  The curation view never has to render or reason about merge commits
  — it just shows the linearized commit list the plan endpoint
  produces.
- **Copy-to-clipboard CTA** echoes the locked one-line command
  `jamsesh finalize-run <plan-id>`. The plain-English summary panel
  is the transparency layer; this command is the execution layer
  (the user's local `finalize-run` will echo the same summary +
  prompt before doing anything).
- **Squash is the default finalization mode**; preserve-all is opt-in.
  The mode bar sits at the top of the page above the source pool /
  cart split. PATCH carries the chosen mode + the squash commit
  message body when applicable.
- **Squash authorship**: every distinct contributor across the
  selection appears as a chip; the server constructs the
  `Co-authored-by` trailers in the composed commit message. The UI
  surfaces the chips for transparency but doesn't let the user
  remove individual contributors (presence in the trailer list
  follows commit selection).
- **Layout direction**: Option 3 (cart pattern) — see Mockups
  section.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
