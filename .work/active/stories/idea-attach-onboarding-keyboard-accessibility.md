---
id: idea-attach-onboarding-keyboard-accessibility
kind: story
stage: implementing
tags: [ui, a11y]
parent: feature-attach-onboarding-a11y-robustness
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-21
updated: 2026-05-25
---

Convert click-only interactive surfaces in the walkthrough modal to native
`<button>` elements, removing all a11y-ignore suppressions.

## Scope

**Files**:
- `frontend/src/lib/components/walkthrough/FullCard.svelte` (`.term-line` divs → buttons)
- `frontend/src/lib/components/walkthrough/CcPane.svelte` (`.cc-input` div → button)
- `frontend/src/lib/components/walkthrough/CompactCard.svelte` (`.reopen-link` span → button)

## Implementation

### A. FullCard.svelte — `.term-line` elements

Replace each `<div class="term-line" ...>` with `<button>`. Remove both
`<!-- svelte-ignore -->` comments before each one.

```svelte
<!-- Before (marketplace line) -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="term-line"
  class:copied={copiedCmd === COMMANDS.marketplace}
  onclick={() => oncopy(COMMANDS.marketplace)}
>
  <span class="check">✓</span>
  ...
</div>

<!-- After -->
<button
  class="term-line"
  class:copied={copiedCmd === COMMANDS.marketplace}
  onclick={() => oncopy(COMMANDS.marketplace)}
  aria-label="Copy: claude plugin marketplace add nklisch/jamsesh"
>
  <span class="check" aria-hidden="true">✓</span>
  <span class="prompt" aria-hidden="true">$</span>
  <span class="cmd-text">claude plugin marketplace add <span class="arg">nklisch/jamsesh</span></span>
  <span class="hint" aria-hidden="true">...</span>
</button>
```

Same for the `install` command button (`aria-label="Copy: claude plugins install jamsesh"`).

Add CSS resets to `.term-line` rule so the button visually matches the existing div:
```css
.term-line {
  /* add to existing rules: */
  background: transparent;
  border: 0;
  font: inherit;
  color: inherit;
  width: 100%;
  text-align: left;
}
```

### B. CcPane.svelte — `.cc-input` element

Replace the interactive `<div class="cc-input">` (in the `{#if joinCmd !== null}` branch)
with a `<button>`. The placeholder `<div class="cc-input cc-input--placeholder">` stays
as a `<div>` — it is not interactive.

```svelte
<!-- After -->
<button
  class="cc-input"
  class:copied={copiedCmd === joinCmd}
  onclick={() => oncopy(joinCmd!)}
  aria-label="Copy: {joinCmd}"
>
  <span class="cc-arrow" aria-hidden="true">❯</span>
  <span class="cc-cmd">{joinCmd}</span>
  <span class="cc-hint" aria-hidden="true">...</span>
  <span class="cc-check" aria-hidden="true">✓</span>
</button>
```

Remove the `<!-- svelte-ignore -->` comments. Add CSS resets to `.cc-input`:
```css
.cc-input {
  /* add to existing rules: */
  background: transparent;
  border: 0;
  font: inherit;
  color: inherit;
  width: 100%;
  text-align: left;
}
```

### C. CompactCard.svelte — `.reopen-link` element

Replace `<span class="reopen-link" onclick={onshowfull}>` with a `<button>`.
Remove the `<!-- svelte-ignore -->` comments.

```svelte
<!-- After -->
<button class="reopen-link" onclick={onshowfull}>
  First-time setup? Show the full walkthrough &rarr;
</button>
```

Update `.reopen-link` CSS — the display/color/text-decoration rules are already
right; add button resets:
```css
.modal-card.compact .reopen-link {
  /* existing: display, margin, font-size, color, cursor, text-decoration */
  /* add: */
  background: transparent;
  border: 0;
  font: inherit;
  padding: 0;
}
```

## Acceptance Criteria

- [ ] Both `.term-line` elements are `<button>` — Tab-reachable, Enter triggers copy
- [ ] `.cc-input` (non-placeholder) is a `<button>` — Tab-reachable, Enter triggers copy
- [ ] `.reopen-link` is a `<button>` — Tab-reachable, Enter/click triggers mode switch
- [ ] Decorative spans (`check`, `prompt`, `cc-arrow`, `cc-check`, `hint`) have `aria-hidden="true"`
- [ ] `aria-label` on each copy button provides the full command string
- [ ] No `<!-- svelte-ignore a11y_click_events_have_key_events -->` comments remain
- [ ] No `<!-- svelte-ignore a11y_no_static_element_interactions -->` or
  `a11y_no_noninteractive_element_interactions` comments remain on converted elements
- [ ] Test: `document.querySelectorAll('button.term-line').length === 2` in full mode
- [ ] Test: `document.querySelector('button.cc-input')` exists when `sessionId` is set
- [ ] Test: `document.querySelector('button.reopen-link')` exists in compact mode
- [ ] Test: Enter keydown on each button triggers the correct action
