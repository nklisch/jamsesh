---
id: feature-portal-session-attach-onboarding
kind: feature
stage: done
tags: [ui]
parent: null
depends_on: []
release_binding: v0.3.1
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
---

# Portal session-attach onboarding

## Brief

The portal UI today gives users no guidance on how to actually attach a
local Claude Code instance to a freshly-created (or freshly-joined)
session. `NewSessionDrawer` just closes after the POST succeeds, and
`SessionViewShell` drops the user into the awareness surface as if they
were already participating. The designed flow in `docs/UX.md` calls for
the portal to surface a join command after session creation and expects
users to run `/jamsesh:join <session-id>` from a checkout of the source
repo, but the SPA never tells anyone that — they have to already know.

This feature closes that gap by introducing a tiered-disclosure attach
walkthrough that appears at every point a user encounters a fresh session
they haven't joined yet. First-time users get a ceremonial three-step
walkthrough; experienced users get a compact one-liner with the join
command; an always-reachable "Setup help" affordance in the portal chrome
re-opens the full walkthrough at any time.

Visual direction is locked: the modal embeds a Claude-Code-styled pane
(real `claudecode-color.svg` icon, CC's slate-navy chrome, `#D97757`
accent, `❯` prompt indicator) that distinguishes the join slash command
from the two preceding shell-command install steps. See `## Mockups`
below.

## Strategic decisions

Resolved at scope time:

- **Persistence of "don't show again"**: per-browser via `localStorage`
  (no backend). Multi-device users may see the walkthrough once per
  browser; acceptable as a "remember-me" niceness. Avoids growing scope
  into a backend account-preferences slice.
- **Invite-accept inclusion**: in-scope. The walkthrough also opens after
  a successful invite-accept — same install steps apply for collaborators
  arriving via a join link.
- **Affordance behavior**: dumb (always available, same UI for everyone).
  The chrome affordance does not detect whether the current user has
  already attached. Lower complexity; avoids needing a backend attach-state
  endpoint or WS-listening to first-push.

## Anchor surfaces

Four touchpoints in the SPA, listed in expected child-story order:

1. **`SessionAttachWalkthrough` shared component** — the modal itself,
   per the locked mock. State machine for first-time / compact / re-opened.
   localStorage flag `jamsesh.attach-walkthrough-dismissed`. Click-to-copy
   for both shell commands and the slash command. Real CC icon embedded.
2. **`NewSessionDrawer` integration** — on POST success, open the
   walkthrough (passing the new session id) instead of just closing.
3. **`InviteAccept` integration** — on accept success, open the
   walkthrough (passing the session id from the invite).
4. **Always-reachable chrome affordance** — a "Setup help" link in
   `SessionList` and `SessionViewShell` chrome that opens the walkthrough
   on demand. Always available; not gated on attach-state.

## Open scope-time questions (deferred to feature-design)

These are design-internal calls — feature-design Phase 4 / Phase 4.5
resolves them when it drafts the child stories:

- **Compact-view trigger**: dismissal-flag-only (always show full on first
  load until dismissed) or also auto-compact when the user re-opens the
  modal mid-session (within the same browser tab)? Subtle UX call.
- **CC pane "running" feedback**: should the click-to-copy interaction
  also pretend to "run" the command (e.g., a fake "session joined" line
  appears), or is just-copy enough? Mock shows just-copy; revisit if user
  testing surfaces confusion.
- **SessionViewShell empty-state vs always-on**: should the in-session
  affordance render differently when there's clearly no commits yet
  (suggesting the user hasn't attached) vs when commits are flowing
  (clearly attached)? The "dumb" strategic decision says no — but
  presentation could still vary cosmetically.

## Mockups

- Compare: `.mockups/screens/portal-session-attach-onboarding/index.html`
- **Selected: option-6** — Terminal-first ceremonial, both states sharing
  the same modal shell. Signed off 2026-05-20.
- Rationale: Two shell commands in a "your terminal" pane on top, then a
  Claude-Code-styled pane below for the slash command. The CC pane embeds
  the real `claudecode-color.svg` brand icon, uses CC's `#D97757` clay
  accent, a white `❯` prompt indicator, and matches CC's slate-navy
  chrome. The compact (returning) state is the same shell at a smaller
  size — single CC pane, single command. Surface distinction
  (shell vs Claude Code) is named explicitly in the lede prose and
  reinforced by the visual chrome of each pane.
- Iteration notes (for feature-design's reference):
  1. First pass (Option 2) made first-time and returning views look like
     two unrelated screens — confused the user. Refined into Option 5
     where both states are the same modal-card shape (just bigger /
     smaller). Option 6 carried that journey shape but adopted the
     terminal-styled CC pane.
  2. Three layout bugs caught during iteration: eyebrow stretching to
     full width (column-flex `align-items: stretch`); third step-card
     overflowing past modal edge (`min-width: auto` on flex/grid
     children); rust-tinted status bar reading wrong (matched real CC's
     slate bg).
  3. Surface labeling matters: original mocks rendered all three
     commands with a shell `$ ` prompt. The join command runs *inside*
     Claude Code, not in a shell. Final design splits the panes
     visually (shell pane top, CC pane bottom) and uses `❯` (CC's
     prompt char) for the slash command.
  4. CC pane brand fidelity: hand-drawn mascot wasn't close enough; the
     real `claudecode-color.svg` is now embedded. Use CC's own colors
     (`#D97757` for accent, white for the prompt chevron) inside the
     pane, but keep the rest of the portal modal in Quiet Slate.
  5. Long-command overflow: `.cc-input .cc-cmd` uses
     `text-overflow: ellipsis` rather than `overflow-x: auto` so the
     compact view doesn't grow a horizontal scrollbar; the copy still
     fires from `data-cmd` (full string, never truncated).

## Notes for feature-design

- No foundation-doc roll-forward — UX.md already describes the flows
  this feature surfaces; the implementation is a faithful rendering, not
  a directional shift.
- No backend / `docs/openapi.yaml` changes — the join command is composed
  client-side from the session id the SPA already has.
- Tests live alongside each child story per the project's
  `spa-test-module-mock-barrel` pattern; no separate testing story.
- The README "Install the Claude Code plugin" section (shipped in v0.3.0)
  carries the same install commands — should stay aligned with whatever
  the in-app walkthrough says.

## Design decisions

Resolved at feature-design time (2026-05-20):

- **InviteAccept integration timing**: walkthrough opens on the InviteAccept
  page; navigation to the session is deferred until the user clicks
  "Open session view →" inside the modal. Same shape as the NewSessionDrawer
  create-success flow.
- **SessionViewShell touchpoint**: chrome affordance only — no top banner,
  no inline empty-state in the artifact pane. Aligns with the "dumb
  affordance" strategic decision (no attach-state detection).
- **Chrome affordance scope**: BOTH SessionList and SessionViewShell
  headers. One shared `<AttachHelpLink>` component reused at both sites.

## Architectural choice

**Approach A — local component state per host.** Each parent that triggers
the walkthrough holds its own `walkthroughSessionId` (or `walkthroughOpen`)
state and conditionally renders `<SessionAttachWalkthrough/>`. The chrome
affordance is wrapped in a small `<AttachHelpLink/>` component that
internally owns both the link rendering and its own walkthrough state.

Considered and rejected:
- Approach B (global rune store + single mount in `App.svelte`): over-engineered
  for four trigger points; doesn't earn its overhead until 5+ surfaces.
- Approach C (hybrid local + store): two patterns to maintain — worst of both.

Approach A matches existing modal patterns in the SPA (`ForkDialog`,
`ModeSwitchDialog`, `NewSessionDrawer` all use local-state-conditional-render).
Maintains consistency with the project's rune-store boundary — rune stores
(`auth.svelte.ts`, `router.svelte.ts`, `ws.svelte.ts`) are for cross-component
shared state, which this is not.

## Implementation Units

### Unit 1: SessionAttachWalkthrough component

**File**: `frontend/src/lib/components/SessionAttachWalkthrough.svelte`
**Story**: `story-portal-session-attach-onboarding-walkthrough-component`

```ts
// Props
type Props = {
  open: boolean;                  // parent toggles visibility
  sessionId: string | null;       // null = chrome-help mode (no specific session)
  onclose: () => void;            // backdrop click / ESC / Close button
  onopenSession?: () => void;     // "Open session view →" — falls back to onclose
};

// Internal state
let mode = $state<'full' | 'compact'>('full');
let dismissChecked = $state(false);          // full-mode checkbox value
let copiedCmd = $state<string | null>(null); // currently-shown "Copied" feedback

// Constants
const DISMISS_KEY = 'jamsesh.attach-walkthrough-dismissed';
const COMMANDS = {
  marketplace: 'claude plugin marketplace add nklisch/jamsesh',
  install: 'claude plugins install jamsesh',
  // join: composed at render time from sessionId
};
```

**Implementation Notes**:

- **DOM/CSS port**: the entire visual layout (modal-backdrop, modal-card,
  eyebrow, shell pane, CC pane, footer) is a direct port of
  `.mockups/screens/portal-session-attach-onboarding/option-6.html`. Use the
  same class names so the CSS is faithful. The mock's `.mock-meta` state
  toggle is dev-only and is NOT carried into the component.
- **Mode detection on mount**: `mode = (typeof localStorage !== 'undefined'
  && localStorage.getItem(DISMISS_KEY) === 'true') ? 'compact' : 'full'`.
  Guard pattern matches `auth.svelte.ts:16-21`.
- **Full mode** renders the `.modal-card.first-time` shell with the
  three-step layout (two shell commands + the CC pane with the slash
  command) plus the "Don't show again" checkbox + Close + "Open session
  view" buttons.
- **Compact mode** renders the `.modal-card.compact` shell — eyebrow +
  heading + CC pane only + the `First-time setup? →` link + footer
  buttons.
- **Chrome-help mode** (`sessionId === null`): the CC pane still renders
  (visual continuity), but the copyable command line is replaced by a
  muted line: "Open a session view to copy its join command." The two
  shell-install commands still render normally — those are the bits
  users actually forget.
- **"Don't show again" persistence**: on close, IF `dismissChecked` is
  true, write `localStorage.setItem(DISMISS_KEY, 'true')`. If the
  checkbox is left unchecked, no write. There is no "un-dismiss" action
  inside the modal; that pref is sticky once set.
- **"First-time setup?" link in compact**: sets internal `mode = 'full'`
  for the rest of the modal's lifetime. Doesn't clear localStorage —
  user's "don't show again" preference persists; this is a one-shot
  expansion.
- **Click-to-copy**: `await navigator.clipboard.writeText(cmd); copiedCmd =
  cmd; setTimeout(() => { if (copiedCmd === cmd) copiedCmd = null; }, 1200);`.
  Render `.copied` class on the line whose command equals `copiedCmd`.
  Use `text-overflow: ellipsis` on the displayed command but read from
  the source string for the clipboard write (see iteration note #5 in
  ## Mockups).
- **ESC + backdrop close**: `$effect` registers a `keydown` listener
  while `open === true`; calls `onclose()` on Escape. Backdrop click
  uses `onclick={(e) => { if (e.target === e.currentTarget) onclose(); }}`.
  Match Modal.svelte's pattern at `frontend/src/lib/components/Modal.svelte:22-32`.
- **"Open session view →"**: calls `onopenSession?.() ?? onclose()`. Don't
  re-implement navigation here — the parent owns destination logic.
- **Accessibility**: `role="dialog"`, `aria-modal="true"`, `aria-label="Attach
  Claude Code to this jam"`. Move focus to the Close button on open
  (rAF deferred). Restore prior focus on close.
- **Real Claude Code icon**: embed `claudecode-color.svg` inline (the
  `<path>` from `/home/nathan/Downloads/claudecode-color.svg`, fill
  `#D97757`). Same inline shape used in `option-6.html`. The icon is
  Anthropic brand and reused at-name with CC's actual chrome colors —
  see Risks.

**Acceptance Criteria**:

- [ ] Renders nothing when `open: false`
- [ ] Renders full walkthrough when `open: true` AND localStorage flag absent
- [ ] Renders compact view when `open: true` AND localStorage flag === 'true'
- [ ] Clicking a copy button on a command writes that exact command string
      to `navigator.clipboard.writeText` AND shows "Copied" feedback for ~1.2s
- [ ] Ticking "Don't show again" + closing writes `'true'` to localStorage
- [ ] Closing WITHOUT ticking "Don't show again" does NOT touch localStorage
- [ ] "First-time setup?" link in compact mode switches internal mode to
      `'full'` for the duration of the current modal lifetime; does NOT
      clear localStorage
- [ ] ESC key while `open === true` calls `onclose`
- [ ] Backdrop click (not card click) calls `onclose`
- [ ] "Open session view →" calls `onopenSession` if provided, else `onclose`
- [ ] When `sessionId === null`: the CC pane renders without a copyable
      join line; shows a muted "Open a session view to copy its join
      command" placeholder. The two shell-command lines remain copyable.
- [ ] Long join command (full UUID) shows as ellipsis-truncated in the
      visible text but full string is what gets copied
- [ ] Negative-case verified per story: removing the localStorage read on
      mount → "compact view when flag present" test fails

---

### Unit 2: AttachHelpLink chrome trigger

**File**: `frontend/src/lib/components/AttachHelpLink.svelte`
**Story**: `story-portal-session-attach-onboarding-help-link`

```ts
type Props = {
  sessionId: string | null;             // forwarded to the walkthrough
  variant?: 'inline' | 'icon';          // default 'inline'; 'icon' for tighter chrome slots
};
```

**Implementation Notes**:

- Renders a chrome-friendly trigger that opens the walkthrough.
  - `'inline'` (default): text link reading "Setup help" prefixed with a
    small `?` glyph. Matches the existing `.signout-btn` aesthetic in
    `Home.svelte` / `SessionList.svelte` topbars.
  - `'icon'`: just the `?` glyph in a 28×28 ghost button. Reserved for
    tighter chrome slots; SessionList and SessionViewShell will use
    `'inline'` initially.
- Internally owns `let open = $state(false)` and renders
  `<SessionAttachWalkthrough open={open} sessionId={sessionId}
  onclose={() => open = false} />`.
- Pure presentational + state-toggle; no API calls, no navigation.

**Acceptance Criteria**:

- [ ] Renders a clickable link/button with the right label per variant
- [ ] Clicking sets `open = true`
- [ ] Closing the walkthrough (via any path: backdrop / ESC / Close /
      "Open session view" without onopenSession prop) sets `open = false`
- [ ] `sessionId` prop forwards to the walkthrough unchanged
- [ ] Default variant is `'inline'` when none specified

---

### Unit 3: SessionList integration (create-success + chrome affordance)

**File**: `frontend/src/lib/screens/SessionList.svelte`
**Story**: `story-portal-session-attach-onboarding-sessionlist-integration`

**Changes**:

1. **Create-success walkthrough**:
   - Add state: `let walkthroughSessionId = $state<string | null>(null);`
   - Modify `handleSessionCreated(newSession)`:
     ```ts
     sessions = [newSession, ...sessions];
     drawerOpen = false;
     walkthroughSessionId = newSession.id;   // <— new line
     ```
   - At the bottom of the markup (alongside the existing `{#if drawerOpen}`):
     ```svelte
     <SessionAttachWalkthrough
       open={walkthroughSessionId !== null}
       sessionId={walkthroughSessionId}
       onclose={() => walkthroughSessionId = null}
       onopenSession={() => {
         const id = walkthroughSessionId;
         walkthroughSessionId = null;
         if (id) navigate(`/orgs/${orgId}/sessions/${id}`);
       }}
     />
     ```
2. **Chrome affordance**: add `<AttachHelpLink sessionId={null} />` to the
   right side of the existing `.topbar` (next to / before the user
   strip / "New session" button).

**Acceptance Criteria**:

- [ ] Creating a new session opens the walkthrough with the new session's id
- [ ] Closing the walkthrough leaves the user on the session list with
      the new session visible at the top
- [ ] Clicking "Open session view →" inside the walkthrough navigates to
      `/orgs/{orgId}/sessions/{newSessionId}`
- [ ] The "Setup help" link is visible in the topbar chrome
- [ ] Clicking the chrome link opens the walkthrough with `sessionId={null}`
      (chrome-help mode)
- [ ] Existing SessionList tests still pass; no regression in load /
      filter / ws-subscribe behavior

---

### Unit 4: InviteAccept integration

**File**: `frontend/src/lib/screens/InviteAccept.svelte`
**Story**: `story-portal-session-attach-onboarding-inviteaccept-integration`

**Changes**:

- Add state: `let walkthroughOpen = $state(false);`
- In the POST 200 success branch, set `walkthroughOpen = true` INSTEAD of
  calling `navigate(...)` immediately. The other state transitions
  (rejection, error) are unaffected.
- Render `<SessionAttachWalkthrough open={walkthroughOpen}
  sessionId={sessionId} onclose={...} onopenSession={...} />` near the
  bottom of the markup. The InviteAccept page UI continues to render
  behind the modal so the user sees "accepted ✓" through the dimmed
  backdrop.
- Both `onclose` and `onopenSession` navigate to
  `/orgs/${orgId}/sessions/${sessionId}` — closing the modal anywhere
  proceeds to the session.

**Acceptance Criteria**:

- [ ] Successful accept opens the walkthrough; URL stays on the invite-accept
      route until the user dismisses
- [ ] Closing the walkthrough (any path) navigates to the session view
- [ ] "Open session view →" navigates to the session view
- [ ] Rejection (`auth.org_membership_required`) flow is UNCHANGED —
      still navigates immediately to `/orgs/{orgId}/sessions`, no walkthrough
- [ ] Error flow is UNCHANGED — still navigates to `/login` on auth failure
- [ ] Existing InviteAccept tests still pass

---

### Unit 5: SessionViewShell chrome affordance

**File**: `frontend/src/lib/screens/SessionViewShell.svelte`
**Story**: `story-portal-session-attach-onboarding-sessionviewshell-affordance`

**Changes**:

- Add `<AttachHelpLink sessionId={sessionId} />` to the SessionViewShell's
  chrome (alongside the existing ThemeToggle).
- `sessionId` is the prop the component already receives — use it.

**Acceptance Criteria**:

- [ ] The "Setup help" link is visible in the SessionViewShell chrome
- [ ] Clicking opens the walkthrough with the current `sessionId` (NOT null)
- [ ] The walkthrough's CC pane renders the correct join command for this
      session
- [ ] Existing SessionViewShell tests still pass

---

## Implementation Order

```
1                    foundation: walkthrough component
├── 2                chrome trigger wrapper (depends on 1)
│   ├── 3            SessionList integration (depends on 1, 2)
│   └── 5            SessionViewShell chrome (depends on 2)
└── 4                InviteAccept integration (depends on 1)
```

**Wave plan for `/agile-workflow:implement-orchestrator`**:
- Wave 1: story 1 alone
- Wave 2: stories 2 and 4 (both unblocked once 1 is done)
- Wave 3: stories 3 and 5 (both unblocked once 2 is done)

3 waves, max 2 parallel per wave. Well under the orchestrator's 3-parallel cap.

## Testing

### Unit tests per story

| Story | Test file | Coverage |
|---|---|---|
| 1 | `SessionAttachWalkthrough.test.ts` (new) | full component contract per acceptance criteria; mocks `navigator.clipboard` and `localStorage` |
| 2 | `AttachHelpLink.test.ts` (new) | trigger + open/close cycle; mocks `SessionAttachWalkthrough` via `vi.mock` to test the link in isolation |
| 3 | `SessionList.test.ts` (modify) | new test: post-create opens walkthrough; new test: chrome link renders |
| 4 | `InviteAccept.test.ts` (modify) | new test: success opens walkthrough; new test: navigate fires on close |
| 5 | `SessionViewShell.test.ts` (modify) | new test: AttachHelpLink renders in chrome with current sessionId |

### Test-integrity discipline

Each story's implementation must verify the negative case before claiming
completion — temporarily mutate the SUT to remove the new behavior,
confirm the test fails, restore, confirm the test passes. Document the
verification in implementation notes. This matches the project pattern
established during v0.3.0's gate-test stories.

### Clipboard + localStorage mocking notes (for story 1)

- jsdom does not provide `navigator.clipboard` by default. Tests must
  `Object.defineProperty(globalThis.navigator, 'clipboard', { value: {
  writeText: vi.fn().mockResolvedValue(undefined) }, configurable: true });`
  in setup.
- localStorage is provided by jsdom but tests should `localStorage.clear()`
  in `beforeEach` to isolate state. Set the dismissed flag explicitly in
  the test that needs it.

## Risks

- **Clipboard API in jsdom**: requires explicit mocking in tests; documented
  in Testing notes above. Story 1's first implementation attempt may hit this
  unexpectedly — flag in the story body so the agent isn't surprised.
- **CC icon brand usage**: `claudecode-color.svg` is presumably an
  Anthropic brand asset. We embed the path data inline. Consistent with
  the README's "Install the Claude Code plugin" section landed in v0.3.0
  (also references CC by name and command). If Anthropic publishes a
  brand-use policy that prohibits this, the SVG can be swapped for a
  generic abstract glyph — the component contract doesn't depend on the
  exact icon shape. Tracking concern, not blocking.
- **Body scroll while modal open**: the existing modal components in the
  SPA don't lock body scroll; this one won't either. Acceptable for v1;
  if it produces awkward scroll-through on long pages, add a
  `body { overflow: hidden }` toggle in a follow-up.
- **Stacked walkthroughs**: SessionList renders both a
  `<SessionAttachWalkthrough>` (create-success) AND an
  `<AttachHelpLink>` (chrome) — each owns its own walkthrough. If a
  user creates a session AND clicks "Setup help" within the same DOM
  commit, two modals could stack. Unlikely in practice; a defensive
  global mount-guard can be added later if it surfaces.
- **Mobile / narrow screens**: out of scope. The 760px first-time card
  will overflow on screens narrower than ~800px. Park a follow-up
  backlog idea (`idea-attach-onboarding-mobile-responsive`) only if a
  user reports it.

## Implementation summary (2026-05-21)

All five child stories landed across three orchestrator waves. Feature stage
advances `implementing → review`.

### Stories shipped

| Story | Commit | Stage | Tests added |
|---|---|---|---|
| `story-portal-session-attach-onboarding-walkthrough-component` | `ab007f4` | review | 25 (new SessionAttachWalkthrough.test.ts) |
| `story-portal-session-attach-onboarding-help-link` | `6be95b2` | review | 10 (new AttachHelpLink.test.ts) |
| `story-portal-session-attach-onboarding-inviteaccept-integration` | `44d8b87` | review | +3 (InviteAccept.test.ts) |
| `story-portal-session-attach-onboarding-sessionlist-integration` | `23106f8` | review | +4 (SessionList.test.ts) |
| `story-portal-session-attach-onboarding-sessionviewshell-affordance` | `993ceca` | review | +2 (SessionViewShell.test.ts) |

### Wave plan executed

- **Wave 1**: walkthrough-component alone (foundation)
- **Wave 2**: help-link + inviteaccept-integration (both gated only on Wave 1)
- **Wave 3**: sessionlist-integration + sessionviewshell-affordance (gated on
  help-link from Wave 2)

### Cross-cutting deviations

- **SessionList chrome placement**: the design body suggested the topbar; the
  implementing agent placed the link in `.page-actions` alongside the "New
  session" button because `Chrome.svelte` exposes no topbar slot.
  Functionally equivalent; visually the same right-aligned chrome region.
  Documented in story body for visibility.
- **SessionViewShell chrome placement**: link sits in `.app-chrome` immediately
  before `<ThemeToggle/>`. Natural alongside existing chrome controls.

### Verification (final)

- **Frontend tests**: 520 / 520 pass across 44 files (two consecutive runs).
  Net delta over the feature: +44 tests (from 476 at start of v0.3.0 cycle to
  520 now, ~+44 from this feature alone).
- **svelte-check**: 0 errors, 2 pre-existing warnings (unrelated:
  ModeSwitchDialog state-referenced-locally; FinalizeView unused CSS).
- **Negative-case discipline**: every story verified its core assertions
  catch regressions (mutation → confirmed failure → restored → confirmed pass).
  Documented per-story in implementation notes.

### Next

- `/agile-workflow:review feature-portal-session-attach-onboarding` (or
  `/agile-workflow:review --all` to drain all five stories + the feature in
  one batch).
