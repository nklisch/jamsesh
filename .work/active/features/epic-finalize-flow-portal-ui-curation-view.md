---
id: epic-finalize-flow-portal-ui-curation-view
kind: feature
stage: done
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

## Design

### Architectural choice

The curation view is a **single full-page Svelte screen** mounted at a
new `finalize` route, owning all state for the page lifecycle:
lock acquisition, curation state (selection, order, mode, target
branch, commit message), plan re-fetch on change, and WebSocket
reactions. The page composes the locked Option-3 (cart) layout from
three distinct surfaces (page-head with lock pill, mode bar at top,
source-pool-left + cart-right main grid) and embeds two sub-components
that only render in squash mode — a `SquashMessageEditor` and a
`CoAuthorChipRow`. Sub-components are kept thin: they are pure
input/output surfaces driven by props and emit events back to the
parent, which holds the single source of truth for the curation
state. This keeps the PATCH-debouncing logic and the WS reaction
matrix in one place.

All API access goes through the existing `client` (openapi-fetch typed
client) and `subscribe` from `$lib/ws.svelte`. The screen is added
under `frontend/src/lib/screens/` and wired into `App.svelte`'s
route table via a new route in `router.svelte.ts`.

### Component tree

```
FinalizeView.svelte                 (screen — new route target)
├── Chrome (app-chrome strip — inlined; matches SessionViewShell
│   pattern; no separate component yet)
├── page-head (h1, base sha, lock pill)
├── ModeBar (inlined segmented control — local, ~30 lines)
├── main (CSS grid)
│   ├── SourcePoolPanel (inlined — render groups + commit rows;
│   │   delegates expand/collapse + add to handlers)
│   └── CartPanel (inlined — config block + cart list + footer)
│       ├── target-branch input
│       ├── SquashMessageEditor.svelte   (only when mode==="squash")
│       │   └── CoAuthorChipRow.svelte
│       ├── cart-list (cart-items with reorder + remove)
│       ├── plan-summary block
│       ├── stats row
│       ├── copy-box (copies `jamsesh finalize-run <plan-id>`)
│       └── primary action: "Run locally" + "Mark as shipped"
└── LockConflictBanner (inlined — surfaced above page-head when
    another member holds the lock)
```

Rationale for keeping the panels inline (not separate components):
the cart and source-pool surfaces are tightly coupled to the curation
$state living on FinalizeView, and there's no reuse target. Lifting
them into components would require prop-drilling the entire curation
state. The two sub-components that *do* get extracted —
`SquashMessageEditor` and `CoAuthorChipRow` — are extracted because
they only render conditionally (squash mode only) and because the
chip row's distinct-author derivation is self-contained logic worth
isolating for testing.

### File map

- `frontend/src/lib/screens/FinalizeView.svelte` — the screen
- `frontend/src/lib/screens/FinalizeView.test.ts` — vitest suite
- `frontend/src/lib/components/SquashMessageEditor.svelte`
- `frontend/src/lib/components/SquashMessageEditor.test.ts`
- `frontend/src/lib/components/CoAuthorChipRow.svelte`
- `frontend/src/lib/components/CoAuthorChipRow.test.ts`
- `frontend/src/lib/router.svelte.ts` — add `finalize` route
- `frontend/src/App.svelte` — render `<FinalizeView/>` on
  `current.name === 'finalize'`
- `frontend/src/lib/screens/SessionViewShell.svelte` — wire the
  existing "Finalize" header button to
  `navigate('/orgs/<org>/sessions/<sid>/finalize')` (small edit
  inside story 1)

### Route

Route pattern: `/^\/orgs\/([^/]+)\/sessions\/([^/]+)\/finalize$/`
→ name `finalize`, params `{orgId, sessionId}`. Added to the
`routes` array in `router.svelte.ts` **before** the broader
`session-view` route so the more specific pattern matches first.

### Props signatures

```ts
// FinalizeView.svelte — top-level screen
let { orgId, sessionId }: { orgId: string; sessionId: string } = $props();

// SquashMessageEditor.svelte
let {
  message,
  onmessagechange,
  coAuthors,
}: {
  message: string;
  onmessagechange: (next: string) => void;
  coAuthors: CoAuthor[];
} = $props();
// CoAuthorChipRow renders via slot composition inside the editor.

// CoAuthorChipRow.svelte
let { authors }: { authors: CoAuthor[] } = $props();

// Local type alias mirrors openapi.yaml CoAuthor schema (added by
// plan-generation; until then this is referenced via the
// generated types module).
type CoAuthor = components['schemas']['CoAuthor'];
type PlanResponse = components['schemas']['PlanResponse'];
type FinalizeLock  = components['schemas']['FinalizeLock'];
```

### State (FinalizeView owns the single source of truth)

```ts
// — connection / lock —
let lock = $state<FinalizeLock | null>(null);
let lockConflict = $state<{ holderName: string; holderAccountId: string } | null>(null);
let lockError = $state<string | null>(null);

// — curation —
let selectedShas = $state<string[]>([]);          // ordered cart
let availableGroups = $state<RefGroup[]>([]);     // grouped by ref
let mode = $state<'squash' | 'preserve'>('squash');
let targetBranch = $state<string>('');
let commitMessage = $state<string>('');           // squash-only

// — plan readout (fetched after every curation change) —
let plan = $state<PlanResponse | null>(null);
let planLoading = $state<boolean>(false);

// — execution UX —
let copyToastVisible = $state<boolean>(false);
let markShippedInFlight = $state<boolean>(false);
let sessionEnded = $state<boolean>(false);        // post-mark-shipped

// — derived —
const isCaller = $derived(plan?.lock_status?.is_caller ?? false);
const canRun = $derived(
  selectedShas.length > 0 && targetBranch.trim().length > 0 &&
  (mode === 'preserve' || commitMessage.trim().length > 0)
);
const distinctAuthors = $derived(plan?.co_authors ?? []);
```

`RefGroup` is the client-side shape derived from the lock response's
`available_refs` field (one entry per source-pool group). Until
plan-generation locks its exact shape, FinalizeView assumes:

```ts
type RefGroup = {
  ref: string;
  kind: 'draft' | 'isolated';
  commits: Array<{
    sha: string;
    author_name: string;
    author_id: string;       // for AuthorDot color
    subject: string;
  }>;
};
```

### Page lifecycle / data flow

1. **Mount** (`$effect`):
   - Call `POST /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock`
     with empty body.
     - `200` → store lock; populate `selectedShas`,
       `targetBranch`, `mode`, `commitMessage` from the lock state.
     - `409 LockHeld` → store `lockConflict` from response and
       render the banner. The page below stays mounted but
       interactions are disabled.
   - Call `GET /api/orgs/{orgID}/sessions/{sessionID}/finalize-plan?lock_id=<id>`
     to render the initial plan + populate `availableGroups`.
   - `subscribe(sessionId, 'session.finalize-lock-acquired', …)`,
     `'session.finalize-lock-released'`, `'session.ended'`,
     `'mode.changed'`. Cleanup on unmount.
2. **Curation change** (any of: toggle commit in cart, reorder,
   target-branch input, mode flip, commit-message edit):
   - Push the change into local $state immediately (optimistic UI).
   - Schedule debounced PATCH (see below).
   - After PATCH success, re-fetch `GET /finalize-plan` and replace
     `plan` (this updates the summary, stats, and `co_authors`).
3. **"Run locally"** click:
   - Copy `jamsesh finalize-run <plan.plan_id>` via
     `navigator.clipboard.writeText`. Show toast for 1.5s.
4. **"Mark as shipped"** click:
   - `POST /api/orgs/{orgID}/sessions/{sessionID}/mark-shipped`.
   - On success, set `sessionEnded = true`, swap the primary CTA
     to a navigation back to the session list, release the lock
     via `DELETE` (best-effort).
5. **Override** click (lock-conflict banner):
   - `POST /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock`
     with body `{override: true}`. On success: clear
     `lockConflict`, reload via the mount sequence.
6. **Unmount**:
   - Best-effort `DELETE /finalize/lock/<lock_id>` if we hold it
     and `sessionEnded === false`. WS cleanup.

### PATCH debounce semantics

Debounce is **trailing-edge with cancellation**: every curation
change resets a 300ms timer. The timer is held as a module-local
`number | null` ref inside the screen's `<script>` block (NOT in
`$state` — its identity changes wouldn't drive UI).

```ts
let patchTimer: ReturnType<typeof setTimeout> | null = null;
let patchSeq = 0;          // monotonic; PATCH N's response is
                           // discarded if patchSeq has advanced

function schedulePatch() {
  if (patchTimer !== null) clearTimeout(patchTimer);
  patchTimer = setTimeout(flushPatch, 300);
}

async function flushPatch() {
  if (!lock) return;
  patchTimer = null;
  const seq = ++patchSeq;
  const body = {
    selected_commit_shas: selectedShas,
    target_branch: targetBranch,
    base_sha: plan?.base_sha ?? '',
    mode,
    commit_message: mode === 'squash' ? commitMessage : null,
  };
  const { data, error } = await client.PATCH(
    '/api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}',
    { params: { path: { orgID, sessionID, lockID: lock.id } }, body },
  );
  if (seq !== patchSeq) return;     // stale response, ignore
  if (error) { /* surface in lockError; do not roll back */ return; }
  await refetchPlan();
}
```

Cancellation rules:
- Component unmount → `clearTimeout(patchTimer)` and skip flush.
- Override or mark-shipped → `clearTimeout(patchTimer)` and skip
  flush.
- Successive PATCHes are serialized by `patchSeq`; only the latest
  in-flight response is allowed to refresh the plan.

### WS reaction matrix

| Event                                  | Reaction                                                                                                                                          |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `session.finalize-lock-acquired`       | If `acquired_by_account_id !== auth.currentUser.id` AND we currently believe we hold the lock → set `lockConflict`, disable the page (we've been overridden), best-effort `DELETE` is skipped (server already cleared ours). |
| `session.finalize-lock-released`       | If `lock_id` matches our held lock and `acquired_by_account_id !== us` → same as above. Otherwise no-op.                                          |
| `session.ended`                        | If `end_reason === 'shipped'` and we have a lock → set `sessionEnded = true`, swap CTA to "Session shipped — back to session list" navigation. If `end_reason === 'abandoned'` → set lockError + disable. |
| `mode.changed`                         | No direct effect on curation state (this is a ref-mode change, not finalization-mode). The page is read-only consumer of the linearized leaf commits; if a ref flips isolated→sync mid-curation the source pool refreshes on the next plan re-fetch. We DO trigger `refetchPlan()` (debounced 1s) to refresh the source pool grouping. |

### Plan re-fetch sequencing

`refetchPlan()` is called after every successful PATCH and after
`mode.changed` events. It's NOT debounced separately — it piggybacks
on PATCH debouncing for curation changes, and is direct on
WS-driven changes (the WS layer already coalesces events).

```ts
async function refetchPlan(): Promise<void> {
  if (!lock) return;
  planLoading = true;
  const { data, error } = await client.GET(
    '/api/orgs/{orgID}/sessions/{sessionID}/finalize-plan',
    { params: {
        path: { orgID, sessionID },
        query: { lock_id: lock.id },
    } },
  );
  planLoading = false;
  if (!error && data) plan = data;
}
```

### Source pool: add/remove/reorder semantics

- **Toggle add**: append the commit's SHA to `selectedShas` (preserves
  selection order = cart order).
- **Toggle remove** (from cart or via in-cart `×`): filter
  `selectedShas`.
- **Reorder** (▲/▼ buttons): swap with neighbor in `selectedShas`.
  Drag-and-drop is **out of scope for v1** — up/down buttons only
  (also satisfies the accessibility note in the brief; drag handles
  remain as a future enhancement story).
- **"Add all" on a group**: append every commit-sha in the group
  that's not already in `selectedShas`.

Every mutation calls `schedulePatch()`.

### Lock-conflict banner

Rendered at the top of the page body (above the page-head) when
`lockConflict !== null`. Uses the design-system `Card` with
warning-muted background. Body: "<HolderName> is finalizing — wait
for them, or override (which restarts curation from the current
draft)." Single `<Button variant="ghost">Override</Button>`.
Override click → `POST /finalize/lock` with `{override: true}`.

### Post-execution "Ready to mark as shipped" state

Always-visible in the cart footer below the "Run locally" button
once the user has copied the run command at least once OR when the
session has been finalizing for > 30 seconds (heuristic — they're
likely running the command locally).

Implementation: a $derived flag `readyToMarkShipped` toggles a
visible "Ready to mark as shipped" banner with the "Mark as
shipped" button. The banner does NOT block the rest of the UI; it
sits between the "Run locally" CTA and the page edge. Once
clicked, the screen shows a confirmation toast and navigates back
to `/orgs/<orgId>/sessions` after 1s (the session is now ended).

### Empty / edge states

- **Empty selection**: cart shows "No commits selected. Toggle commits
  on the left to add them." `canRun` is false → "Run locally"
  disabled.
- **Single commit + squash**: subject pre-fills from that commit's
  message; the bulleted body is just that one bullet.
- **No isolated refs**: source pool only shows the `draft` group.
- **Lock acquire failure (non-409)**: full-page error state with
  retry button.

### Test plan

FinalizeView.test.ts (screen-level):
1. Mounts → calls `POST /finalize/lock` with empty body → renders
   page when lock acquired (mock client + mock initial plan
   response).
2. On 409 lock-held → renders lock-conflict banner with holder
   name; main interactions disabled.
3. Override click → calls `POST /finalize/lock` with
   `{override: true}` → reloads.
4. Toggle a commit in source pool → optimistic add to cart →
   PATCH fired after 300ms debounce; verify request body shape.
5. Successive toggles within debounce window → single PATCH at the
   trailing edge with the final state (not N PATCHes).
6. Stale-response handling — second PATCH supersedes first;
   first's response does not refresh the plan.
7. Mode flip squash→preserve → SquashMessageEditor unmounts;
   PATCH body has `commit_message: null`.
8. WS `session.finalize-lock-acquired` from another account →
   lockConflict set; interactions disabled.
9. WS `session.ended` with `end_reason: 'shipped'` → CTA flips;
   navigate triggered after delay.
10. "Run locally" click → clipboard.writeText called with
    `jamsesh finalize-run <plan_id>`; toast shown.
11. "Mark as shipped" → POST mark-shipped; on 200, sessionEnded
    true.
12. Unmount during pending PATCH → no PATCH fired.
13. Route: navigating to `/orgs/o1/sessions/s1/finalize` matches
    `finalize` route with params `{orgId:'o1', sessionId:'s1'}`.

SquashMessageEditor.test.ts:
1. Renders the message prop in a textarea.
2. Editing the textarea fires `onmessagechange` with new value
   (verify it does NOT fire on every keystroke — debounce is the
   parent's responsibility, but the editor itself is uncontrolled
   re: timing; just forwards `oninput`).
3. CoAuthorChipRow renders one chip per author in `coAuthors`,
   each with the matching AuthorDot color.

CoAuthorChipRow.test.ts:
1. Renders empty when `authors` is empty (no `<span class="label">`
   alone with no chips).
2. Renders N chips for N authors; verifies name + AuthorDot
   composition.
3. Updates when prop changes (Svelte 5 prop reactivity check).

### Verification commands

- `cd frontend && pnpm test` — vitest suite passes
- `cd frontend && pnpm check` — svelte-check clean

### Child stories

1. `epic-finalize-flow-portal-ui-curation-view-screen-and-route`
   — FinalizeView.svelte screen, route wiring, lock lifecycle,
   PATCH debouncing, plan re-fetch, WS reactions, source-pool +
   cart layout, "Run locally" copy + "Mark as shipped" + lock-
   conflict banner. SquashMessageEditor and CoAuthorChipRow may be
   placeholder stubs in this story (rendering nothing); they get
   filled in story 2.
2. `epic-finalize-flow-portal-ui-curation-view-squash-editor-and-coauthor-chips`
   — SquashMessageEditor.svelte + CoAuthorChipRow.svelte. Depends
   on story 1 (slot fits into the FinalizeView's cart-config
   block). Story 1 lands the screen with the sub-components
   imported but no-op; story 2 implements them and verifies the
   message + co-author chip rendering.

Dependency: story 2 `depends_on: [story 1]`.

## Implementation summary

2 child stories landed (commits a547ca7, 1754890). FinalizeView screen with cart pattern + 300ms-debounced PATCH + stale-response guard, plus the SquashMessageEditor and CoAuthorChipRow polish components. 256 frontend tests pass.

## Review

**Verdict**: Approve. Cart-pattern curation flow ships clean.
