<script lang="ts">
  // CompactCard — returning-user compact card.
  //
  // Renders the condensed "just the join command" view for users who have
  // previously dismissed the full walkthrough. Includes a link to re-open
  // the full walkthrough if needed.

  import CcPane from './CcPane.svelte';

  type Props = {
    sessionId: string | null;
    joinCmd: string | null;
    copiedCmd: string | null;
    /** Most-recent command whose clipboard.writeText rejected — passed
     *  through to CcPane for the failure-hint UI. */
    copyFailedCmd?: string | null;
    closeBtnRef?: HTMLButtonElement | null;
    oncopy: (cmd: string) => void;
    onshowfull: () => void;
    onclose: () => void;
    onopensession: () => void;
  };

  let {
    sessionId,
    joinCmd,
    copiedCmd,
    copyFailedCmd = null,
    closeBtnRef = $bindable(),
    oncopy,
    onshowfull,
    onclose,
    onopensession,
  }: Props = $props();
</script>

<!-- The modal content card carries the dialog landmark so screen readers
     focus the content (not the scrim). The Svelte warning about
     "non-interactive element with interactive role" does not apply here:
     `<article role="dialog">` is the recommended pattern in WAI-ARIA APG
     for modal content containers. -->
<!-- svelte-ignore a11y_no_noninteractive_element_to_interactive_role -->
<article
  class="modal-card compact"
  role="dialog"
  aria-modal="true"
  aria-label="Attach Claude Code to this jam"
  tabindex="-1"
>
  <span class="eyebrow">
    {#if sessionId}
      Session · {sessionId}
    {:else}
      Claude Code setup
    {/if}
  </span>
  <h1>Attach your Claude Code.</h1>
  <p class="lede">
    At the Claude Code prompt (CC running in a checkout of your source repo), click the line
    below to copy and paste:
  </p>

  <CcPane {joinCmd} {copiedCmd} {copyFailedCmd} oncopy={oncopy} />

  <button type="button" class="reopen-link" onclick={onshowfull}>
    First-time setup? Show the full walkthrough &rarr;
  </button>

  <div class="card-footer">
    <div></div>
    <div class="card-actions">
      <button class="btn-ghost" bind:this={closeBtnRef} onclick={onclose}>Close</button>
      <button class="btn-primary" onclick={onopensession}>Open session view &rarr;</button>
    </div>
  </div>
</article>

<style>
  /* Shared eyebrow */
  .eyebrow {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 4px 10px;
    background: var(--color-accent-muted);
    color: var(--color-accent);
    border-radius: var(--radius-full);
    font: var(--font-weight-medium) var(--font-size-xs) var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    margin-bottom: 14px;
  }

  /* Modal card shell */
  .modal-card {
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.25);
  }

  .modal-card h1 {
    margin: 0 0 10px;
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.02em;
    line-height: var(--line-height-tight);
    color: var(--color-text-primary);
  }

  .modal-card .lede {
    margin: 0 0 20px;
    color: var(--color-text-secondary);
    line-height: 1.55;
  }

  /* Card footer */
  .card-footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-top: 20px;
    margin-top: 20px;
    border-top: 1px solid var(--color-border);
  }

  .card-actions {
    display: flex;
    gap: 8px;
  }

  .btn-primary,
  .btn-ghost {
    border-radius: var(--radius-md);
    padding: 10px 20px;
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    cursor: pointer;
  }

  .btn-primary {
    background: var(--color-bg-inverse);
    color: var(--color-text-inverse);
    border: 0;
  }

  .btn-ghost {
    background: transparent;
    color: var(--color-text-secondary);
    border: 1px solid var(--color-border);
  }

  /* Returning: compact card sizing */
  .modal-card.compact {
    max-width: 520px;
    width: 100%;
    padding: 28px 32px;
  }

  .modal-card.compact h1 {
    font-size: var(--font-size-xl);
  }

  .modal-card.compact .lede {
    font-size: var(--font-size-sm);
    margin-bottom: 16px;
  }

  .modal-card.compact .reopen-link {
    /* Native <button> reset so the inline-link appearance survives. */
    background: transparent;
    border: 0;
    padding: 0;
    font: inherit;

    display: inline-block;
    margin-top: 14px;
    font-size: var(--font-size-xs);
    color: var(--color-text-link);
    cursor: pointer;
    text-decoration: underline dotted;
  }
</style>
