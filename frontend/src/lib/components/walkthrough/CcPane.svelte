<script lang="ts">
  // CcPane — Claude Code prompt pane.
  //
  // Renders the styled CC header + join-command input row. Used by both
  // FullCard (first-time walkthrough) and CompactCard (returning user).

  type Props = {
    joinCmd: string | null;
    copiedCmd: string | null;
    oncopy: (cmd: string) => void;
  };

  let { joinCmd, copiedCmd, oncopy }: Props = $props();
</script>

<div class="cc-pane">
  <div class="cc-header">
    <div class="cc-mascot">
      <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
        <title>Claude Code</title>
        <path
          clip-rule="evenodd"
          fill-rule="evenodd"
          fill="#D97757"
          d="M20.998 10.949H24v3.102h-3v3.028h-1.487V20H18v-2.921h-1.487V20H15v-2.921H9V20H7.488v-2.921H6V20H4.487v-2.921H3V14.05H0V10.95h3V5h17.998v5.949zM6 10.949h1.488V8.102H6v2.847zm10.51 0H18V8.102h-1.49v2.847z"
        />
      </svg>
    </div>
    <div class="cc-meta">
      <div class="cc-app">
        <span class="cc-app-name">Claude Code</span>
      </div>
      <div class="cc-model">Opus 4.7 (1M context) with xhigh effort &middot; Claude Max</div>
      <div class="cc-cwd">~/dev/your-source-repo</div>
    </div>
  </div>

  <div class="cc-input-wrap">
    {#if joinCmd !== null}
      <button
        type="button"
        class="cc-input"
        class:copied={copiedCmd === joinCmd}
        onclick={() => oncopy(joinCmd!)}
        aria-label={`Copy: ${joinCmd}`}
      >
        <span class="cc-arrow" aria-hidden="true">❯</span>
        <span class="cc-cmd">{joinCmd}</span>
        <span class="cc-hint" aria-hidden="true">{copiedCmd === joinCmd ? 'copied' : 'click to copy'}</span>
        <span class="cc-check" aria-hidden="true">✓</span>
      </button>
    {:else}
      <div class="cc-input cc-input--placeholder">
        <span class="cc-arrow">❯</span>
        <span class="cc-cmd cc-cmd--muted"
          >Open a session view to copy its join command.</span
        >
      </div>
    {/if}
  </div>
</div>

<style>
  /* Claude Code pane */
  .cc-pane {
    border-radius: 8px;
    overflow: hidden;
    background: #131726;
    border: 1px solid #232940;
    font: 12px / 1.5 var(--font-mono);
    color: #c8cdd6;
  }

  .cc-header {
    display: flex;
    gap: 16px;
    align-items: center;
    padding: 16px 18px 14px;
    background: #131726;
  }

  .cc-mascot {
    width: 44px;
    height: 44px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .cc-mascot svg {
    width: 100%;
    height: 100%;
    shape-rendering: crispEdges;
  }

  .cc-meta {
    line-height: 1.45;
    min-width: 0;
  }

  .cc-app {
    line-height: 1.3;
  }

  .cc-app-name {
    font: var(--font-weight-bold) 14px var(--font-mono);
    color: #f3f4f7;
  }

  .cc-model,
  .cc-cwd {
    font-size: 11px;
    color: #6b7390;
  }

  .cc-input-wrap {
    margin: 4px 14px 18px;
    border: 1px solid #2a3148;
    border-radius: 5px;
    background: #131726;
    overflow: hidden;
  }

  .cc-input {
    /* Native <button> reset so the CC prompt look survives. The placeholder
       branch stays as a <div> (non-interactive). */
    background: transparent;
    border: 0;
    color: inherit;
    font: inherit;
    width: 100%;
    text-align: left;

    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    cursor: pointer;
    min-width: 0;
  }

  .cc-input--placeholder {
    cursor: default;
  }

  .cc-input:not(.cc-input--placeholder):hover {
    background: rgba(255, 255, 255, 0.02);
  }

  .cc-input.copied {
    background: rgba(180, 95, 74, 0.1);
  }

  .cc-input .cc-arrow {
    flex-shrink: 0;
    color: #f3f4f7;
    font: var(--font-weight-medium) 14px / 1 var(--font-mono);
    width: 12px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  /* Truncate visually; copy reads the full source string, not the displayed text. */
  .cc-input .cc-cmd {
    flex: 1;
    min-width: 0;
    font: 13px var(--font-mono);
    color: #d9dde4;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cc-cmd--muted {
    color: #6b7390;
    font-style: italic;
  }

  .cc-input .cc-check {
    width: 16px;
    flex-shrink: 0;
    color: #4a8c5c;
    opacity: 0;
    font: var(--font-weight-bold) 13px var(--font-mono);
  }

  .cc-input.copied .cc-check {
    opacity: 1;
  }

  .cc-input .cc-hint {
    font-size: 10px;
    color: #6b7390;
    opacity: 0;
    flex-shrink: 0;
  }

  .cc-input:not(.cc-input--placeholder):hover .cc-hint {
    opacity: 1;
  }

  .cc-input.copied .cc-hint {
    opacity: 1;
    color: #4a8c5c;
  }
</style>
