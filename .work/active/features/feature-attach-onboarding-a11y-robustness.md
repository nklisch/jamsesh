---
id: feature-attach-onboarding-a11y-robustness
kind: feature
stage: implementing
tags: [ui, a11y, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# SessionAttachWalkthrough modal — a11y + robustness pass

## Brief

Close the four review nits surfaced during the v0.3.1 review of
`feature-portal-session-attach-onboarding`. All four touch the same file
(`frontend/src/lib/screens/SessionAttachWalkthrough.svelte`) and follow the
same shape: a missing error path, a misplaced ARIA role, or click-only
interaction. Bounded — single file, no design system shift, no foundation-doc
impact.

## Member stories

- `idea-attach-onboarding-clipboard-error-handling` —
  wrap `navigator.clipboard.writeText` in try/catch with graceful UI
- `idea-attach-onboarding-dialog-role-on-card` —
  move `role="dialog"` from backdrop to inner `<article class="modal-card">`
- `idea-attach-onboarding-keyboard-accessibility` —
  convert click-only `.term-line` / `.cc-input` / `.reopen-link` to real
  buttons; remove a11y-ignore suppressions
- `idea-attach-onboarding-localstorage-error-handling` —
  wrap `localStorage.setItem`/`getItem` in try/catch so QuotaExceededError
  / SecurityError don't keep the modal mounted

## Mockups

Parent feature `portal-session-attach-onboarding` has the canonical screens:
`.mockups/screens/portal-session-attach-onboarding/` — no new mock surfaces,
these stories fix structural and behavioral bugs in the existing design.

## Design decisions

- **Are the four stories genuinely parallel?** Yes — each story owns a
  distinct sub-file and fix shape. `dialog-role` edits `SessionAttachWalkthrough.svelte`
  (the backdrop wrapper only); `localstorage-error-handling` edits the two
  `localStorage` call sites in the same file's `$effect` and `handleClose`/
  `handleOpenSession`; `keyboard-accessibility` edits `FullCard.svelte` (`.term-line`),
  `CcPane.svelte` (`.cc-input`), and `CompactCard.svelte` (`.reopen-link`);
  `clipboard-error-handling` edits `copyCmd()` in `SessionAttachWalkthrough.svelte`.
  The only overlap is two stories touching `SessionAttachWalkthrough.svelte` —
  `dialog-role` at lines 112–123 and `localstorage-error-handling` at lines 83–98
  plus the `$effect` at line 47–48. These are non-adjacent hunks; two agents
  can work them concurrently without conflict. Declared order is all-parallel.
- **`dialog-role`: backdrop or card as the dialog landmark?** Inner `<article>`
  — this matches `Modal.svelte`'s pattern (`role="dialog"` on `.modal`, not
  `.modal-overlay`) and what screen readers expect: the landmark wraps content,
  not the scrim. The backdrop becomes a plain `<div role="presentation">`.
- **`keyboard-accessibility`: `<button>` or `onkeydown`?** Native `<button>` for
  all three surfaces. Gets role, focus ring, Enter/Space handling for free;
  no keyboard handler to maintain; no a11y-ignore suppression needed.
  Styling: `background: transparent; border: 0; padding: 0; text-align: left;
  cursor: pointer; width: 100%;` on term-line and cc-input wrappers preserves
  visual appearance. The `.reopen-link` becomes `<button class="reopen-link">`.
- **`clipboard-error-handling`: failure UI placement?** Inline next to the
  copied element using the existing `copiedCmd` state machine. Extend it to
  `type CopiedState = { cmd: string; ok: boolean }` — on success `ok: true`
  (shows "copied" badge), on failure `ok: false` (shows "Copy failed — select
  and copy manually"). Same 1.2s timeout clears it. No new state vars needed
  beyond the type change.
- **`localstorage-error-handling`: failure behavior?** Swallow and proceed —
  the user clicked Close; the intent is to close. Log to `console.warn` so
  devs can diagnose, then call `onclose()` regardless. Same for `getItem` in
  the `$effect`: on throw, fall back to `'full'` mode (safe default).

## Architectural choice

**Surgical patch in-place.** The component hierarchy is already well-decomposed
(orchestrator + FullCard + CompactCard + CcPane). Each fix is a local change
in the appropriate file; no new abstractions, no new components, no interface
changes. This is appropriate for bug/a11y fixes: minimum churn, maximum
reviewability.

Alternative (wrap in a generic `<ClipboardButton>` or `<StorageSafe>` helper):
over-engineered for three call sites; the codebase does not have a pattern for
these micro-helpers; defer if the pattern emerges elsewhere.

## Implementation Units

### Unit 1: dialog-role fix
**File**: `frontend/src/lib/components/SessionAttachWalkthrough.svelte`
**Story**: `idea-attach-onboarding-dialog-role-on-card`

Change the `.modal-backdrop` `<div>` from:
```svelte
<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="modal-backdrop"
  role="dialog"
  aria-modal="true"
  aria-label="Attach Claude Code to this jam"
  tabindex="-1"
  onclick={(e) => { if (e.target === e.currentTarget) handleClose(); }}
>
```
to:
```svelte
<div
  class="modal-backdrop"
  role="presentation"
  onclick={(e) => { if (e.target === e.currentTarget) handleClose(); }}
>
```

Then update `FullCard.svelte` and `CompactCard.svelte` to carry the dialog
role on their `<article>` element:
```svelte
<article
  class="modal-card first-time"
  role="dialog"
  aria-modal="true"
  aria-label="Attach Claude Code to this jam"
  tabindex="-1"
>
```
(Same treatment on `<article class="modal-card compact">`.)

The a11y-ignore comments on the backdrop are removed entirely — a plain `<div
role="presentation">` with a click handler on a non-interactive element will
still warn; use `<!-- svelte-ignore a11y_no_static_element_interactions -->` if
needed, or restructure the onclick to be on a real button overlay. Preferred:
keep the backdrop click via `onclick` on the `role="presentation"` div and add
the minimal ignore comment only there.

**Implementation Notes**:
- `tabindex="-1"` moves to the `<article>` so focus-trap helpers (and the
  existing `$effect` that calls `closeBtn?.focus()`) continue to work; the
  initial focus still goes to `closeBtn` via the `bind:this={closeBtnRef}`
  binding, which is unaffected.
- The existing test `'has correct dialog role and aria attributes'` selects
  `document.querySelector('[role="dialog"]')` — after the fix this will match
  the `<article>` instead of the backdrop `<div>`. The assertions still hold.

**Acceptance Criteria**:
- [ ] `[role="dialog"]` is on `<article class="modal-card ...">`, not on `.modal-backdrop`
- [ ] `.modal-backdrop` has `role="presentation"` (not `role="dialog"`)
- [ ] `aria-modal="true"` and `aria-label` are on the `<article>`
- [ ] No a11y-ignore comments remain on the dialog container itself
- [ ] Existing test `'has correct dialog role and aria attributes'` still passes

---

### Unit 2: localStorage error handling
**File**: `frontend/src/lib/components/SessionAttachWalkthrough.svelte`
**Story**: `idea-attach-onboarding-localstorage-error-handling`

Three call sites to harden:

**A. `$effect` mode-on-mount (line 44–51):**
```svelte
$effect(() => {
  if (!open) return;
  let dismissed = false;
  try {
    dismissed = localStorage.getItem(DISMISS_KEY) === 'true';
  } catch {
    // SecurityError / quota-blocked context — default to full mode
  }
  mode = dismissed ? 'compact' : 'full';
});
```

**B. `handleClose()` (line 82–87):**
```ts
function handleClose() {
  if (dismissChecked) {
    try {
      localStorage.setItem(DISMISS_KEY, 'true');
    } catch (e) {
      console.warn('[jamsesh] Could not persist walkthrough dismiss flag:', e);
    }
  }
  onclose();
}
```

**C. `handleOpenSession()` (line 90–98):** Same try/catch around the `setItem`.

**Implementation Notes**:
- `onclose()` / `onopenSession()` are called unconditionally after the
  try/catch — the user's intent to close must never be blocked by storage.
- The `typeof localStorage !== 'undefined'` guard in the original `$effect`
  was only for SSR safety; the new try/catch subsumes it entirely. Remove the
  `typeof` guard to keep the code clean.

**Acceptance Criteria**:
- [ ] `localStorage.setItem` failures do not prevent `onclose` from firing
- [ ] `localStorage.getItem` throwing falls back to `mode = 'full'`
- [ ] `console.warn` is called on `setItem` failure (verifiable in test via `vi.spyOn`)
- [ ] Test: mock `localStorage.setItem` to throw → assert `onclose` still fires
- [ ] Test: mock `localStorage.getItem` to throw → assert modal renders full mode

---

### Unit 3: keyboard accessibility
**Files**: `frontend/src/lib/components/walkthrough/FullCard.svelte`,
           `frontend/src/lib/components/walkthrough/CcPane.svelte`,
           `frontend/src/lib/components/walkthrough/CompactCard.svelte`
**Story**: `idea-attach-onboarding-keyboard-accessibility`

**A. `.term-line` in `FullCard.svelte`** — each `<div class="term-line">` becomes:
```svelte
<button
  class="term-line"
  class:copied={copiedCmd === COMMANDS.marketplace}
  onclick={() => oncopy(COMMANDS.marketplace)}
  aria-label="Copy: claude plugin marketplace add nklisch/jamsesh"
>
  <span class="check" aria-hidden="true">✓</span>
  <span class="prompt" aria-hidden="true">$</span>
  <span class="cmd-text">claude plugin marketplace add <span class="arg">nklisch/jamsesh</span></span>
  <span class="hint" aria-hidden="true">{copiedCmd === COMMANDS.marketplace ? 'copied' : 'click to copy'}</span>
</button>
```
(Same for the `install` line.) Remove both `<!-- svelte-ignore -->` comments.

CSS adjustment: add to existing `.term-line` rule:
```css
.term-line {
  /* existing rules ... */
  background: transparent;
  border: 0;
  font: inherit;
  color: inherit;
  width: 100%;
  text-align: left;
}
```

**B. `.cc-input` in `CcPane.svelte`** — the `<div class="cc-input">` in the
`{#if joinCmd !== null}` branch becomes:
```svelte
<button
  class="cc-input"
  class:copied={copiedCmd === joinCmd}
  onclick={() => oncopy(joinCmd!)}
  aria-label="Copy: {joinCmd}"
>
  ...
</button>
```
Remove `<!-- svelte-ignore -->` comments. CSS adjustment on `.cc-input`:
```css
.cc-input {
  /* existing rules ... */
  background: transparent;
  border: 0;
  font: inherit;
  color: inherit;
  width: 100%;
  text-align: left;
}
```
The placeholder branch (`cc-input--placeholder`) stays as a `<div>` — it is
not interactive.

**C. `.reopen-link` in `CompactCard.svelte`** — the `<span class="reopen-link">` becomes:
```svelte
<button class="reopen-link" onclick={onshowfull}>
  First-time setup? Show the full walkthrough &rarr;
</button>
```
Remove `<!-- svelte-ignore -->` comments. CSS: add `background: transparent;
border: 0; font: inherit; color: inherit; cursor: pointer; padding: 0;
text-decoration: underline dotted;` to `.reopen-link`.

**Implementation Notes**:
- `aria-label` on term-line and cc-input buttons provides a clean accessible
  name that includes the command text, decoupled from visual truncation in `.cc-cmd`.
- Decorative spans (check mark, prompt `$`, hint text) get `aria-hidden="true"`
  to reduce screen-reader noise.
- No focus ring changes needed — native `<button>` focus ring is already styled
  correctly by the browser / project global styles.

**Acceptance Criteria**:
- [ ] `.term-line` elements are `<button>` elements reachable by Tab
- [ ] `.cc-input` (non-placeholder) is a `<button>` reachable by Tab
- [ ] `.reopen-link` is a `<button>` reachable by Tab
- [ ] Enter key on each interactive element triggers the expected action (copy / expand)
- [ ] No `<!-- svelte-ignore a11y_click_events_have_key_events -->` comments remain
  on any converted element
- [ ] Test: `fireEvent.keyDown(termLineBtn, { key: 'Enter' })` triggers copy
- [ ] Test: Tab order reaches cc-input button and reopen-link button

---

### Unit 4: clipboard error handling
**File**: `frontend/src/lib/components/SessionAttachWalkthrough.svelte`
**Story**: `idea-attach-onboarding-clipboard-error-handling`

Extend the state type and `copyCmd` function:

```ts
// Replace: let copiedCmd = $state<string | null>(null);
type CopyFeedback = { cmd: string; ok: boolean } | null;
let copyFeedback = $state<CopyFeedback>(null);

// Convenience derived for child components (backward-compat prop name):
let copiedCmd = $derived(copyFeedback?.ok ? copyFeedback.cmd : null);

// New state for failure hint:
let copyFailedCmd = $derived(!copyFeedback?.ok ? copyFeedback?.cmd ?? null : null);
```

Updated `copyCmd`:
```ts
async function copyCmd(cmd: string) {
  try {
    await navigator.clipboard.writeText(cmd);
    copyFeedback = { cmd, ok: true };
  } catch {
    copyFeedback = { cmd, ok: false };
  }
  setTimeout(() => {
    if (copyFeedback?.cmd === cmd) copyFeedback = null;
  }, 1200);
}
```

Pass `copyFailedCmd` down to child cards as a new prop, and in `FullCard` /
`CcPane` / `CompactCard` show the failure hint when `copyFailedCmd === <that cmd>`:
```svelte
<!-- In FullCard term-line: -->
<span class="hint" aria-hidden="true">
  {#if copyFailedCmd === COMMANDS.marketplace}
    Copy failed — select and copy manually
  {:else if copiedCmd === COMMANDS.marketplace}
    copied
  {:else}
    click to copy
  {/if}
</span>
```

**Implementation Notes**:
- The `copiedCmd` derived keeps child component props backward-compatible —
  `class:copied={copiedCmd === cmd}` still works with no changes to child templates
  for the success path.
- Failure text "Copy failed — select and copy manually" is visible for the same
  1.2s window, then clears. This is sufficient; no persistent error state.
- `console.warn` is intentionally omitted here — clipboard denial in non-secure
  contexts is an expected browser policy, not an application error.

**Acceptance Criteria**:
- [ ] `navigator.clipboard.writeText` rejection does not propagate as an unhandled promise rejection
- [ ] On rejection, the hint text for the clicked element reads "Copy failed — select and copy manually"
- [ ] The `copied` CSS class is NOT applied on rejection
- [ ] Feedback clears after ~1.2s in both success and failure cases
- [ ] Test: mock `writeText` to reject → assert failure hint text appears
- [ ] Test: mock `writeText` to reject → assert `copied` class absent

---

## Implementation Order

All four stories are **parallel** — no `depends_on` between them.

1. (parallel) `idea-attach-onboarding-dialog-role-on-card`
2. (parallel) `idea-attach-onboarding-localstorage-error-handling`
3. (parallel) `idea-attach-onboarding-keyboard-accessibility`
4. (parallel) `idea-attach-onboarding-clipboard-error-handling`

Rationale: each story edits a different set of lines/files. The only shared
file is `SessionAttachWalkthrough.svelte`, where the two touching stories
(`dialog-role` and `localstorage-error-handling`) operate on non-adjacent
hunks (lines 112–123 vs lines 44–51, 82–98). Clipboard handling also touches
`SessionAttachWalkthrough.svelte` but at `copyCmd()` and state declarations,
which are separate hunks from both others. Merge conflicts are unlikely and
resolvable trivially if two agents finish simultaneously.

## Testing

### Test file: `frontend/src/lib/components/SessionAttachWalkthrough.test.ts`

The existing test file has 20+ tests covering the happy paths. New tests to add
per story:

**clipboard-error-handling additions:**
```ts
it('shows "Copy failed" hint when clipboard.writeText rejects', async () => {
  writeText.mockRejectedValueOnce(new DOMException('Not allowed', 'NotAllowedError'));
  renderWalkthrough({ open: true });
  const lines = document.querySelectorAll('.term-line');
  await fireEvent.click(lines[0]);
  await waitFor(() => {
    expect(lines[0]).toHaveTextContent(/copy failed.*select and copy manually/i);
  });
});

it('does not apply "copied" class when clipboard.writeText rejects', async () => {
  writeText.mockRejectedValueOnce(new DOMException('Not allowed', 'NotAllowedError'));
  renderWalkthrough({ open: true });
  const lines = document.querySelectorAll('.term-line');
  await fireEvent.click(lines[0]);
  await waitFor(() => {
    expect(lines[0]).not.toHaveClass('copied');
  });
});
```

**localstorage-error-handling additions:**
```ts
it('still calls onclose when localStorage.setItem throws', async () => {
  const onclose = vi.fn();
  const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new DOMException('QuotaExceededError');
  });
  renderWalkthrough({ open: true, onclose });
  const checkbox = screen.getByRole('checkbox');
  await fireEvent.click(checkbox);
  const skipBtn = screen.getByRole('button', { name: /skip for now/i });
  await fireEvent.click(skipBtn);
  expect(onclose).toHaveBeenCalledTimes(1);
  setItem.mockRestore();
});

it('falls back to full mode when localStorage.getItem throws', async () => {
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
    throw new DOMException('SecurityError');
  });
  renderWalkthrough({ open: true });
  expect(document.querySelector('.modal-card.first-time')).toBeInTheDocument();
});
```

**keyboard-accessibility additions:**
```ts
it('term-line buttons are reachable by Tab and trigger copy on Enter', async () => {
  renderWalkthrough({ open: true });
  const buttons = document.querySelectorAll('button.term-line');
  expect(buttons.length).toBe(2);
  await fireEvent.keyDown(buttons[0], { key: 'Enter' });
  await waitFor(() => {
    expect(writeText).toHaveBeenCalledWith('claude plugin marketplace add nklisch/jamsesh');
  });
});

it('cc-input is a button and triggers copy on Enter', async () => {
  renderWalkthrough({ open: true, sessionId: 'abc' });
  const ccBtn = document.querySelector('button.cc-input');
  expect(ccBtn).toBeInTheDocument();
  await fireEvent.keyDown(ccBtn!, { key: 'Enter' });
  await waitFor(() => {
    expect(writeText).toHaveBeenCalledWith('/jamsesh:join abc');
  });
});

it('reopen-link is a button', async () => {
  localStorage.setItem('jamsesh.attach-walkthrough-dismissed', 'true');
  renderWalkthrough({ open: true });
  const btn = document.querySelector('button.reopen-link');
  expect(btn).toBeInTheDocument();
});
```

**dialog-role additions:**
```ts
it('dialog role is on the modal-card article, not the backdrop', () => {
  renderWalkthrough({ open: true });
  const backdrop = document.querySelector('.modal-backdrop');
  expect(backdrop).not.toHaveAttribute('role', 'dialog');
  const card = document.querySelector('.modal-card');
  expect(card).toHaveAttribute('role', 'dialog');
  expect(card).toHaveAttribute('aria-modal', 'true');
});
```

Note: the existing test `'has correct dialog role and aria attributes'`
selects `document.querySelector('[role="dialog"]')` which will now match
the `<article>` — assertions still hold. Update the test description if
desired, but no assertion change needed.

## Risks

- **Existing test `'has correct dialog role and aria attributes'`**: The test
  queries `[role="dialog"]` and checks it has the aria attributes. After the
  dialog-role fix, this query hits the `<article>` instead of the backdrop.
  The assertions still pass — no change needed, but the story implementor
  should run the full test suite after the fix to confirm.
- **`<button>` inside `.terminal` styling**: Resetting button appearance
  (border, background, font) inside the dark terminal `<div>` must be done
  explicitly; browser UA styles will otherwise show a gray box. The design
  above specifies the resets; implementor must verify visually.
- **`copyFeedback` rename**: `copiedCmd` is used as a prop name in child
  components. The design uses a `$derived` alias to keep backward compatibility.
  If any implementor renames the prop in child components, the test selectors
  (`class:copied`) must be updated consistently.
