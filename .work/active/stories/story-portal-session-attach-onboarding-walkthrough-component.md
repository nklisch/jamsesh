---
id: story-portal-session-attach-onboarding-walkthrough-component
kind: story
stage: review
tags: [ui]
parent: feature-portal-session-attach-onboarding
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
---

# SessionAttachWalkthrough shared component

Foundation story for the attach-onboarding feature. Implements the
shared modal component at
`frontend/src/lib/components/SessionAttachWalkthrough.svelte`.

The visual reference is locked at
`.mockups/screens/portal-session-attach-onboarding/option-6.html` —
port the DOM and CSS structure faithfully (same class names, same
overall layout). Drop the mock's `.mock-meta` state toggle (dev-only).

The full design is in the parent feature body under
`## Implementation Units → Unit 1`. Read that first — it contains the
props contract, internal-state design, behavior spec, accessibility
requirements, and the full acceptance-criteria checklist.

## Brief acceptance criteria summary

(See parent feature for the full list.)

- Component contract: `{ open, sessionId, onclose, onopenSession? }` props
- localStorage key `jamsesh.attach-walkthrough-dismissed` controls
  full-vs-compact mode on mount
- "First-time setup?" link in compact mode toggles to full for the
  current modal lifetime only
- Click-to-copy via `navigator.clipboard.writeText`; "Copied" badge for ~1.2s
- ESC / backdrop click / Close button all call `onclose`
- "Open session view →" calls `onopenSession ?? onclose`
- When `sessionId === null` (chrome-help mode): CC pane shows a placeholder
  hint instead of a copyable join line; install commands still copyable
- Long commands ellipsis-truncate visually; full string is what gets copied
- Real Claude Code icon embedded inline (path from
  `/home/nathan/Downloads/claudecode-color.svg`, `#D97757` fill)

## Test file

`frontend/src/lib/components/SessionAttachWalkthrough.test.ts` (new).

Setup notes:
- jsdom needs `navigator.clipboard` mocked explicitly:
  `Object.defineProperty(globalThis.navigator, 'clipboard', { value: {
  writeText: vi.fn().mockResolvedValue(undefined) }, configurable: true });`
- `localStorage.clear()` in `beforeEach`; set the dismissed flag
  explicitly in tests that need compact mode.

## Negative-case discipline

Before claiming completion, mutate the SUT (e.g. remove the localStorage
read on mount) and confirm the relevant test fails. Restore. Document in
implementation notes.

## Implementation notes

### Files created

- `frontend/src/lib/components/SessionAttachWalkthrough.svelte`
- `frontend/src/lib/components/SessionAttachWalkthrough.test.ts`

### Approach

**Mode-on-mount**: a `$effect` that watches `open`; when `open` becomes true
it reads `localStorage.getItem(DISMISS_KEY)` and sets `mode` to `'compact'`
or `'full'`. Guard pattern `typeof localStorage !== 'undefined'` matches
`auth.svelte.ts:16-21`. This ensures reopening the modal re-reads the flag.

**Focus management**: mirrors `Modal.svelte:34-47` exactly — `bind:this` on
the close/skip button, `requestAnimationFrame` deferred focus on open,
cleanup restores prior `document.activeElement`. Since both mode branches
render their own close button with `bind:this={closeBtn}`, the binding
always targets the visible button.

**Click-to-copy state**: `copiedCmd = $state<string | null>(null)`. Each
copyable element passes its full source string to `copyCmd(cmd)` — the
displayed text may be ellipsis-truncated but `navigator.clipboard.writeText`
always receives the untruncated source string. The 1.2s timeout clears
`copiedCmd` only if the same command is still "active" (prevents a fast
double-click from resetting the timer of a different command).

**Chrome-help mode (`sessionId === null`)**: `joinCmd = $derived(sessionId ?
\`/jamsesh:join ${sessionId}\` : null)`. When `joinCmd === null`, both mode
branches render a `.cc-input--placeholder` div (no onclick, muted copy
hint text). Shell install commands still render normally with full copy
behavior.

**CSS scoping**: the CC pane uses hardcoded hex values (`#131726`, `#D97757`,
`#f3f4f7`, `#6b7390`) from the mock — these intentionally diverge from
portal design tokens since the pane represents CC's own surface. Used
`var(--font-mono)` for all monospaced rendering.

**Backdrop click**: implemented on the `.modal-backdrop` div (not the
`article`) using `if (e.target === e.currentTarget)` guard; role="dialog" is
on the backdrop div with `tabindex="-1"` to satisfy Svelte's a11y checker.

### Test count

25 tests in `SessionAttachWalkthrough.test.ts`. All 501 project tests pass
(43 test files), confirmed twice with no flake.

### Negative-case verification

**AC under test**: "Renders compact view when `open: true` AND localStorage
flag === 'true'"

**Mutation applied**: Replaced the `$effect` body with `mode = 'full'`
(hardcoded), removing the `localStorage.getItem(DISMISS_KEY)` read entirely.

**Observed result**: 4 tests failed:
- "renders compact view when dismissed flag is 'true'" (primary AC)
- "switches to full mode when 'First-time setup?' is clicked in compact"
  (requires compact as starting state)
- "calls onclose when Close button is clicked in compact mode" (same)
- "shows compact placeholder in CC pane when sessionId is null and flag set" (same)

**Restoration**: SUT restored; all 501 tests pass.
