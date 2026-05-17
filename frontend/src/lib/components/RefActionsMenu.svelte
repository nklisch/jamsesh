<script lang="ts">
  let {
    ref,
    x = 0,
    y = 0,
    onaction,
    onclose,
  }: {
    ref: string;
    x?: number;
    y?: number;
    onaction?: (action: 'fork' | 'mode-switch', ref: string) => void;
    onclose?: () => void;
  } = $props();

  function handleAction(action: 'fork' | 'mode-switch') {
    onaction?.(action, ref);
    onclose?.();
  }

  // Close on click outside.
  function handleBackdropClick() {
    onclose?.();
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="backdrop"
  onclick={handleBackdropClick}
  onkeydown={(e) => e.key === 'Escape' && onclose?.()}
  aria-hidden="true"
></div>

<div
  class="menu"
  role="menu"
  aria-label="Ref actions for {ref}"
  style="left: {x}px; top: {y}px;"
>
  <div class="menu-header">
    <span class="ref-label" title={ref}>{ref.split('/').slice(-2).join('/')}</span>
  </div>
  <button
    class="menu-item"
    role="menuitem"
    onclick={() => handleAction('fork')}
  >
    <span class="icon">⑂</span>
    Fork…
  </button>
  <button
    class="menu-item"
    role="menuitem"
    onclick={() => handleAction('mode-switch')}
  >
    <span class="icon">⇄</span>
    Switch mode…
  </button>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 99;
  }

  .menu {
    position: fixed;
    z-index: 100;
    min-width: 180px;
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18);
    overflow: hidden;
  }

  .menu-header {
    padding: 6px 12px;
    border-bottom: 1px solid var(--color-border);
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--color-text-tertiary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .ref-label {
    font-weight: var(--font-weight-medium);
  }

  .menu-item {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 10px 14px;
    background: transparent;
    border: 0;
    color: var(--color-text-primary);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    cursor: pointer;
    text-align: left;
  }

  .menu-item:hover {
    background: var(--color-bg-tertiary);
  }

  .icon {
    font-size: 14px;
    color: var(--color-text-secondary);
    width: 18px;
    text-align: center;
  }
</style>
