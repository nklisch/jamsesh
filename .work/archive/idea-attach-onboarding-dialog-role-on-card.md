---
id: idea-attach-onboarding-dialog-role-on-card
kind: story
stage: done
tags: [ui, a11y]
parent: feature-attach-onboarding-a11y-robustness
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-21
updated: 2026-05-25
---

Move `role="dialog"`, `aria-modal`, and `aria-label` from the `.modal-backdrop`
`<div>` to the inner `<article class="modal-card">` in both `FullCard.svelte`
and `CompactCard.svelte`.

## Scope

**Files**:
- `frontend/src/lib/components/SessionAttachWalkthrough.svelte` (backdrop element)
- `frontend/src/lib/components/walkthrough/FullCard.svelte` (add role to `<article>`)
- `frontend/src/lib/components/walkthrough/CompactCard.svelte` (add role to `<article>`)

## Implementation

**`SessionAttachWalkthrough.svelte`** — strip dialog attributes from backdrop:
```svelte
<!-- Before -->
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

<!-- After -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="modal-backdrop"
  role="presentation"
  onclick={(e) => { if (e.target === e.currentTarget) handleClose(); }}
>
```

**`FullCard.svelte`** — add role to `<article>`:
```svelte
<article
  class="modal-card first-time"
  role="dialog"
  aria-modal="true"
  aria-label="Attach Claude Code to this jam"
  tabindex="-1"
>
```

**`CompactCard.svelte`** — same:
```svelte
<article
  class="modal-card compact"
  role="dialog"
  aria-modal="true"
  aria-label="Attach Claude Code to this jam"
  tabindex="-1"
>
```

The `tabindex="-1"` on the `<article>` allows programmatic focus; the actual
initial focus still goes to `closeBtn` via the existing `bind:this={closeBtnRef}`
binding in each card — no change needed to focus management.

## Acceptance Criteria

- [ ] `[role="dialog"]` is on `<article class="modal-card ...">`, not on `.modal-backdrop`
- [ ] `.modal-backdrop` has `role="presentation"` (no dialog role)
- [ ] `aria-modal="true"` and `aria-label="Attach Claude Code to this jam"` on the `<article>`
- [ ] No svelte-ignore comments on the dialog landmark itself
- [ ] Test: `.modal-backdrop` does not have `role="dialog"`
- [ ] Test: `.modal-card` has `role="dialog"`, `aria-modal="true"`, and `aria-label`
- [ ] Existing test `'has correct dialog role and aria attributes'` still passes

## Implementation notes

- `SessionAttachWalkthrough.svelte`: backdrop now has `role="presentation"`,
  no dialog attributes; `svelte-ignore a11y_click_events_have_key_events` +
  `a11y_no_static_element_interactions` cover the backdrop click handler.
- `FullCard.svelte` + `CompactCard.svelte`: `<article>` carries `role="dialog"`,
  `aria-modal="true"`, `aria-label="Attach Claude Code to this jam"`, and
  `tabindex="-1"`. A focused `svelte-ignore a11y_no_noninteractive_element_to_interactive_role`
  with a comment explaining the WAI-ARIA APG modal-content pattern justifies
  the role on `<article>`.
- New test `places role=dialog on the modal-card <article>, not on the
  backdrop scrim` pins the invariant; existing `has correct dialog role and
  aria attributes` continues to pass (its `[role="dialog"]` selector now
  matches the article).

Verified: `npm test -- --run SessionAttachWalkthrough.test.ts` → 26 passed.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Matches design exactly — backdrop is `role="presentation"`, dialog landmark moved to inner `<article>` in both FullCard and CompactCard, focused svelte-ignore comments are scoped and justified, new negative-assertion test pins the invariant. The orchestrator's separate AttachHelpLink.test.ts selector fix (61afa03) is the correct test-integrity response to the moved role.
