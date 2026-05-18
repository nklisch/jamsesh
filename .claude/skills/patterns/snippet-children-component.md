# Pattern: Snippet-Children Polymorphic Component

Reusable Svelte 5 presentational components take a typed
`children: Snippet` prop and render it once via `{@render children()}`.
Variant/size/state inputs are typed as string-literal unions
destructured from `$props()`. No slots, no event-handler props beyond
standard DOM ones, no internal state.

## Rationale

Snippets are Svelte 5's replacement for slots; explicit typing makes
the component generic over content but constrained over presentation.
Co-locating the variant union with the prop destructure keeps the API
surface and CSS class mapping (`class="btn btn-{variant} btn-{size}"`)
one screen apart, so a new variant is one edit per location.

## Examples

### Example 1: Button

**File**: `frontend/src/lib/components/Button.svelte:1`

```svelte
<script lang="ts">
  import type { Snippet } from 'svelte';
  type Variant = 'primary' | 'ghost' | 'accent';
  type Size = 'sm' | 'md' | 'lg';
  let {
    variant = 'primary', size = 'md', disabled = false,
    type = 'button', onclick, children,
  }: { variant?: Variant; size?: Size; ...; children: Snippet; } = $props();
</script>
<button class="btn btn-{variant} btn-{size}" {type} {disabled} {onclick}>
  {@render children()}
</button>
```

### Example 2: Badge with 8-way variant union

**File**: `frontend/src/lib/components/Badge.svelte:1` — same shape:
`Variant` union, `children: Snippet` destructured from `$props()`,
single `{@render children()}`.

### Example 3: Card / InlineCode / Chrome / Modal

**Files**: `frontend/src/lib/components/{Card,InlineCode,Chrome,Modal}.svelte`
— every primitive presentational component in `components/` uses the
same `children: Snippet` + `$props()` shape. 6+ instances.

## When to Use

- A reusable presentational primitive (button, badge, card, modal,
  chrome) that takes arbitrary inner content.
- Variants/sizes/states are a small closed set of string literals —
  encode as union types.

## When NOT to Use

- Screens with their own state and side-effects (e.g. `Login.svelte`,
  `FinalizeView.svelte`) — those live in `frontend/src/lib/screens/`,
  use `$state`/`$effect`, and don't take `children`.
- Components that need multiple insertion points — use named snippets
  (e.g. `orgChip`, `sessionChip` on `Chrome.svelte` are exactly this)
  rather than a single `children`.

## Common Violations

- Falling back to `<slot />` from Svelte 4 — works at runtime but
  breaks the typed-children contract; TypeScript can't enforce
  `children: Snippet`.
- Embedding business logic or `$state` inside a primitive — promotes
  the file to a screen but keeps it under `components/`. Move it to
  `screens/`.
- Using `any` or omitting the `$props()` type annotation — loses
  prop-shape safety in consumer call sites.
