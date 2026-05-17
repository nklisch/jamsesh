<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    padding = 'md',
    children,
  }: {
    padding?: 'sm' | 'md' | 'lg';
    children: Snippet;
  } = $props();
</script>

<div class="card card-p-{padding}">
  {@render children()}
</div>

<style>
  .card {
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
  }

  .card-p-sm {
    padding: var(--space-3);
  }

  .card-p-md {
    padding: var(--space-4);
  }

  .card-p-lg {
    padding: var(--space-6);
  }
</style>
