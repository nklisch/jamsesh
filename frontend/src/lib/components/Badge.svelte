<script lang="ts">
  import type { Snippet } from 'svelte';

  // Variants:
  // - neutral: muted secondary treatment
  // - success: green-toned (currently maps to accent-muted/accent)
  // - warning/isolated: amber — used by the isolated mode pill
  // - danger/conflict: red — used by conflict pill and danger states
  // - accent: filled accent background (inverse text)
  // - sync: alias of success styling (sync mode pill)
  type Variant =
    | 'neutral'
    | 'success'
    | 'warning'
    | 'danger'
    | 'accent'
    | 'sync'
    | 'isolated'
    | 'conflict';

  let {
    variant = 'neutral',
    children,
  }: {
    variant?: Variant;
    children: Snippet;
  } = $props();
</script>

<span class="pill pill-{variant}">
  {@render children()}
</span>

<style>
  .pill {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: 2px var(--space-2);
    border-radius: var(--radius-full);
    font-family: var(--font-mono);
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-semibold);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    white-space: nowrap;
    line-height: 1.4;
  }

  .pill-neutral {
    background: var(--color-bg-tertiary);
    color: var(--color-text-secondary);
  }

  /* success and sync share accent-muted/accent styling (teal) */
  .pill-success,
  .pill-sync {
    background: var(--color-accent-muted);
    color: var(--color-accent);
  }

  /* warning and isolated share amber styling */
  .pill-warning,
  .pill-isolated {
    background: var(--color-warning-muted);
    color: var(--color-warning);
  }

  /* danger and conflict share red styling */
  .pill-danger,
  .pill-conflict {
    background: var(--color-danger-muted);
    color: var(--color-danger);
  }

  /* accent: filled accent background */
  .pill-accent {
    background: var(--color-accent);
    color: var(--color-text-inverse);
  }
</style>
