<script lang="ts">
  // AttachHelpLink — chrome-friendly trigger that wraps SessionAttachWalkthrough.
  //
  // Props:
  //   sessionId  — forwarded unchanged to the walkthrough (null = chrome-help mode)
  //   variant    — 'inline' (default): text link reading "? Setup help"
  //                'icon': 28×28 ghost button containing just the "?" glyph
  //
  // Internally owns open state; pure presentational + state toggle; no API calls.

  import SessionAttachWalkthrough from './SessionAttachWalkthrough.svelte';

  type Props = {
    sessionId: string | null;
    variant?: 'inline' | 'icon';
  };

  let { sessionId, variant = 'inline' }: Props = $props();

  let open = $state(false);
</script>

<!-- svelte-ignore a11y_consider_explicit_label -->
{#if variant === 'icon'}
  <button
    class="help-btn help-btn--icon"
    onclick={() => (open = true)}
    aria-label="Setup help"
    title="Setup help"
  >
    ?
  </button>
{:else}
  <button
    class="help-btn help-btn--inline"
    onclick={() => (open = true)}
  >
    <span class="help-glyph" aria-hidden="true">?</span>
    Setup help
  </button>
{/if}

<SessionAttachWalkthrough
  {open}
  {sessionId}
  onclose={() => (open = false)}
/>

<style>
  /* Shared ghost-button base — matches .signout-btn aesthetic in Home.svelte
     but slightly more subtle (xs font, lighter weight). */
  .help-btn {
    background: transparent;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    color: var(--color-text-secondary);
    font-family: var(--font-sans);
    cursor: pointer;
    transition: background-color 120ms ease;
    line-height: 1;
  }

  .help-btn:hover {
    background: var(--color-bg-tertiary);
    border-color: var(--color-border-strong);
  }

  /* Inline variant: text link with ? glyph prefix */
  .help-btn--inline {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 5px 10px;
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-medium);
  }

  .help-glyph {
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-medium);
    opacity: 0.7;
  }

  /* Icon variant: 28×28 square ghost button */
  .help-btn--icon {
    width: 28px;
    height: 28px;
    padding: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-medium);
  }
</style>
