<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
  }: {
    children: Snippet;
  } = $props();
</script>

<code class="inline-code">{@render children()}</code>

<style>
  .inline-code {
    font-family: var(--font-mono);
    font-size: var(--font-size-sm);
    line-height: inherit;
    background: var(--color-bg-tertiary);
    color: var(--color-text-primary);
    padding: 1px var(--space-1);
    border-radius: var(--radius-sm);
    border: 1px solid var(--color-border);
  }
</style>
