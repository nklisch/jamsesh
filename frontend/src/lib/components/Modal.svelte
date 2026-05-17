<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    open,
    title,
    ariaLabel,
    size = 'md',
    onclose,
    children,
  }: {
    open: boolean;
    title: string;
    ariaLabel?: string;
    size?: 'sm' | 'md';
    onclose?: () => void;
    children: Snippet;
  } = $props();

  let closeBtn = $state<HTMLButtonElement | null>(null);

  // ESC key handler — register while open, clean up on close.
  $effect(() => {
    if (!open) return;
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        onclose?.();
      }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  });

  // Focus management — move focus to close button on open, restore on close.
  $effect(() => {
    if (open) {
      const prev = document.activeElement as HTMLElement | null;
      // Defer so the DOM has been painted.
      const id = requestAnimationFrame(() => {
        closeBtn?.focus();
      });
      return () => {
        cancelAnimationFrame(id);
        prev?.focus();
      };
    }
  });
</script>

{#if open}
  <div
    class="modal-overlay"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) onclose?.();
    }}
  >
    <div
      class="modal size-{size}"
      role="dialog"
      aria-modal="true"
      aria-label={ariaLabel ?? title}
    >
      <div class="modal-header">
        <h2 class="modal-title">{title}</h2>
        <button
          class="close-btn"
          aria-label="Close"
          bind:this={closeBtn}
          onclick={() => onclose?.()}
        >×</button>
      </div>
      {@render children()}
    </div>
  </div>
{/if}

<style>
  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
  }

  .modal {
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md);
    box-shadow: 0 16px 40px rgba(0, 0, 0, 0.25);
  }

  .size-sm {
    min-width: 340px;
    max-width: 460px;
  }

  .size-md {
    min-width: 360px;
    max-width: 500px;
  }

  .modal-header {
    display: flex;
    align-items: center;
    padding: 14px 16px;
    border-bottom: 1px solid var(--color-border);
  }

  .modal-title {
    flex: 1;
    margin: 0;
    font-size: var(--font-size-base);
    font-weight: var(--font-weight-semibold);
    color: var(--color-text-primary);
  }

  .close-btn {
    background: transparent;
    border: 0;
    color: var(--color-text-secondary);
    font-size: 20px;
    cursor: pointer;
    padding: 0 2px;
    line-height: 1;
  }
</style>
