<script lang="ts">
  import type { Snippet } from 'svelte';

  type Variant = 'primary' | 'ghost' | 'accent';
  type Size = 'sm' | 'md' | 'lg';

  let {
    variant = 'primary',
    size = 'md',
    disabled = false,
    type = 'button',
    onclick,
    children,
  }: {
    variant?: Variant;
    size?: Size;
    disabled?: boolean;
    type?: 'button' | 'submit' | 'reset';
    onclick?: (e: MouseEvent) => void;
    children: Snippet;
  } = $props();
</script>

<button class="btn btn-{variant} btn-{size}" {type} {disabled} {onclick}>
  {@render children()}
</button>

<style>
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    font-family: var(--font-sans);
    font-weight: var(--font-weight-medium);
    border-radius: var(--radius-md);
    cursor: pointer;
    border: 1px solid transparent;
    transition:
      background-color 120ms ease,
      color 120ms ease,
      border-color 120ms ease;
    text-decoration: none;
  }

  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  /* Sizes */
  .btn-sm {
    font-size: var(--font-size-sm);
    padding: var(--space-1) var(--space-3);
  }

  .btn-md {
    font-size: var(--font-size-base);
    padding: var(--space-2) var(--space-4);
  }

  .btn-lg {
    font-size: var(--font-size-lg);
    padding: var(--space-3) var(--space-6);
  }

  /* Variants */
  .btn-primary {
    background: var(--color-bg-inverse);
    color: var(--color-text-inverse);
    border-color: var(--color-bg-inverse);
  }

  .btn-primary:hover:not(:disabled) {
    background: var(--color-text-secondary);
    border-color: var(--color-text-secondary);
  }

  .btn-ghost {
    background: transparent;
    color: var(--color-text-primary);
    border-color: var(--color-border-strong);
  }

  .btn-ghost:hover:not(:disabled) {
    background: var(--color-bg-tertiary);
  }

  .btn-accent {
    background: var(--color-accent);
    color: var(--color-text-inverse);
    border-color: var(--color-accent);
  }

  .btn-accent:hover:not(:disabled) {
    background: var(--color-accent-hover);
    border-color: var(--color-accent-hover);
  }
</style>
