<script lang="ts">
  import { onDestroy } from 'svelte';

  type Props = {
    command: string;       // "" when not ready
    ready: boolean;        // true when command is non-empty and usable
    errorMessage?: string; // user-facing error related to plan/command fetch
    disabled?: boolean;    // overall interactions disabled (e.g. !isCaller || !canRun)
    oncopy?: () => void;   // fired after a successful clipboard write
  };

  let { command, ready, errorMessage, disabled = false, oncopy }: Props = $props();

  let showToast = $state(false);
  let toastTimer: ReturnType<typeof setTimeout> | null = null;

  onDestroy(() => {
    if (toastTimer !== null) clearTimeout(toastTimer);
    toastTimer = null;
  });

  async function copy(): Promise<void> {
    if (!command) return;
    try {
      await navigator.clipboard.writeText(command);
    } catch {
      return;
    }
    showToast = true;
    if (toastTimer !== null) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
      showToast = false;
      toastTimer = null;
    }, 1500);
    oncopy?.();
  }
</script>

<div class="command-runner">
  {#if errorMessage}
    <div class="error" role="alert">{errorMessage}</div>
  {:else if !ready}
    <div class="hint">Preparing command…</div>
  {:else}
    <div class="copy-box">
      <code>{command}</code>
      <button type="button" onclick={() => void copy()} {disabled}>Copy</button>
    </div>
    <button
      class="primary"
      type="button"
      onclick={() => void copy()}
      {disabled}
    >
      Run locally
    </button>
    {#if showToast}
      <span class="toast" role="status" aria-live="polite">Copied!</span>
    {/if}
  {/if}
</div>

<style>
  .command-runner {
    display: contents;
  }

  .copy-box {
    display: flex; align-items: stretch;
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md);
    overflow: hidden; background: var(--color-bg-secondary);
    margin-bottom: 10px;
  }
  .copy-box code {
    flex: 1; padding: 10px 12px;
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-mono);
    color: var(--color-text-primary);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .copy-box button {
    border: 0; border-left: 1px solid var(--color-border);
    background: var(--color-bg-tertiary); color: var(--color-text-primary);
    padding: 0 14px; cursor: pointer;
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
  }
  .copy-box button:hover:not(:disabled) { background: var(--color-bg-inverse); color: var(--color-text-inverse); }
  .copy-box button:disabled { opacity: 0.5; cursor: not-allowed; }

  .primary {
    width: 100%; padding: 12px;
    background: var(--color-bg-inverse); color: var(--color-text-inverse);
    border: 0; border-radius: var(--radius-md);
    font: var(--font-weight-semibold) var(--font-size-sm) var(--font-sans);
    cursor: pointer;
  }
  .primary:hover:not(:disabled) { opacity: 0.92; }
  .primary:disabled { opacity: 0.5; cursor: not-allowed; }

  .toast {
    position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%);
    padding: 10px 18px;
    background: var(--color-bg-inverse); color: var(--color-text-inverse);
    border-radius: var(--radius-md);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    box-shadow: var(--shadow-lg, 0 4px 12px rgba(0, 0, 0, 0.2));
    z-index: 100;
  }

  .hint {
    font-size: var(--font-size-sm); color: var(--color-text-secondary);
    padding: 10px 0;
  }

  .error {
    font-size: var(--font-size-sm); color: var(--color-danger);
    padding: 10px 0;
  }
</style>
