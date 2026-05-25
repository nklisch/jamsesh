---
id: idea-attach-onboarding-clipboard-error-handling
kind: story
stage: done
tags: [ui, bug]
parent: feature-attach-onboarding-a11y-robustness
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-21
updated: 2026-05-25
---

`SessionAttachWalkthrough.svelte` — `copyCmd()` wraps `navigator.clipboard.writeText`
in try/catch with graceful failure UI.

## Scope

**File**: `frontend/src/lib/components/SessionAttachWalkthrough.svelte`

The current `copyCmd` function is:
```ts
async function copyCmd(cmd: string) {
  await navigator.clipboard.writeText(cmd);
  copiedCmd = cmd;
  setTimeout(() => { if (copiedCmd === cmd) copiedCmd = null; }, 1200);
}
```

If `writeText` rejects (non-secure context, permissions-policy block, extension
sandbox), the rejection propagates unhandled — the "Copied" badge never fires
and the user gets no feedback.

## Implementation

Replace `let copiedCmd = $state<string | null>(null)` with:

```ts
type CopyFeedback = { cmd: string; ok: boolean } | null;
let copyFeedback = $state<CopyFeedback>(null);

// Backward-compat derived: child components use copiedCmd for the .copied class
let copiedCmd = $derived(copyFeedback?.ok ? copyFeedback.cmd : null);

// Failure hint: which cmd just failed to copy
let copyFailedCmd = $derived(!copyFeedback?.ok ? (copyFeedback?.cmd ?? null) : null);
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

Pass `copyFailedCmd` to `FullCard` and `CompactCard` as a new prop. In each
card (and `CcPane`), update the hint `<span>` to show the failure text when
`copyFailedCmd === <that cmd>`:

```svelte
<!-- term-line hint in FullCard.svelte -->
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

Same pattern for the `install` command line and for the `cc-input` hint in
`CcPane.svelte`.

## Acceptance Criteria

- [ ] `navigator.clipboard.writeText` rejection does not produce an unhandled promise rejection
- [ ] On rejection, the hint for the clicked element shows "Copy failed — select and copy manually"
- [ ] The `copied` CSS class is NOT applied on rejection
- [ ] Feedback (success or failure) clears after ~1.2s
- [ ] Test: mock `writeText` to reject → failure hint text appears
- [ ] Test: mock `writeText` to reject → `copied` class absent on the clicked element

## Implementation notes

- Introduced `type CopyFeedback = { cmd: string; ok: boolean } | null` and
  `let copyFeedback = $state<CopyFeedback>(null)` in
  `SessionAttachWalkthrough.svelte`.
- `copiedCmd` becomes a `$derived` (success-path only); `copyFailedCmd` is a
  new `$derived` (failure-path only). Child component prop names stay the same
  for backward compat.
- `copyCmd()` now `try/catch`-wraps `navigator.clipboard.writeText`. On
  rejection: `copyFeedback = { cmd, ok: false }`; success: `copyFeedback = { cmd, ok: true }`.
  The 1.2s timer clears feedback in both cases. No `console.warn` — clipboard
  denial in non-secure contexts is browser policy, not an app error.
- `FullCard.svelte`: term-line hint `<span>` now shows
  `"Copy failed — select and copy manually"` when `copyFailedCmd` matches the
  command; falls through to "copied" / "click to copy" otherwise. Added
  `copyFailedCmd?: string | null` to props.
- `CcPane.svelte`: same hint logic; added `copyFailedCmd?` prop with default
  `null`.
- `CompactCard.svelte`: added `copyFailedCmd?` prop and passes through to CcPane.
- `SessionAttachWalkthrough.svelte` template passes `copyFailedCmd` to both
  child cards.
- Four new tests cover: failure hint on term-line, no `copied` class on
  failure, cc-input failure hint, no unhandled-rejection escape.

Verified: `npm test -- --run SessionAttachWalkthrough.test.ts` → 35 passed.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: `copyFeedback` state machine collapses success/failure into one source; `copiedCmd` / `copyFailedCmd` $derived signals preserve child-prop API. try/catch swallows browser-policy rejections (correct — not an app error). Timer-clear branch correctly keys on cmd identity. All three hint sites (term-line ×2, cc-input) updated with consistent fallback text. Four new tests pin the contract including no-unhandled-rejection.
