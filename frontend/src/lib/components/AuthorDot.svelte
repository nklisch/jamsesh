<script lang="ts">
  // Deterministic hash of authorId to a 1–8 index for --author-N CSS var lookup.
  // Same input string always maps to the same index across page loads.
  // Uses djb2-style polynomial hash clamped to int32 via bitwise OR.
  function authorColorIndex(id: string): number {
    let h = 0;
    for (let i = 0; i < id.length; i++) {
      h = ((h << 5) - h) + id.charCodeAt(i);
      h |= 0; // clamp to 32-bit signed int
    }
    return (Math.abs(h) % 8) + 1;
  }

  let {
    authorId,
    size = 16,
    online = false,
    title,
  }: {
    authorId: string;
    size?: number;
    online?: boolean;
    title?: string;
  } = $props();

  const idx = $derived(authorColorIndex(authorId));
</script>

<span
  class="dot"
  class:online
  style="--dot-color: var(--author-{idx}); --dot-size: {size}px"
  title={title ?? authorId}
  role="img"
  aria-label={title ?? `author ${authorId}`}
>
  {#if online}
    <span class="pulse" aria-hidden="true"></span>
  {/if}
</span>

<style>
  .dot {
    position: relative;
    display: inline-block;
    width: var(--dot-size);
    height: var(--dot-size);
    border-radius: var(--radius-full);
    background: var(--dot-color);
    flex-shrink: 0;
  }

  /* Green online indicator — bottom-right corner of the dot */
  .pulse {
    position: absolute;
    bottom: -2px;
    right: -2px;
    width: 33%;
    height: 33%;
    min-width: 6px;
    min-height: 6px;
    background: var(--color-success);
    border: 2px solid var(--color-bg-secondary);
    border-radius: var(--radius-full);
  }
</style>
