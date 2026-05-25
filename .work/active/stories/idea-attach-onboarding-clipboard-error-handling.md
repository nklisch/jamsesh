---
id: idea-attach-onboarding-clipboard-error-handling
kind: story
stage: implementing
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
