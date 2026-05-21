<script lang="ts">
  // SessionAttachWalkthrough — tiered-disclosure attach modal.
  //
  // Mode state machine:
  //   'full'    → first-time ceremonial (three steps: two shell installs + CC join)
  //   'compact' → returning user (CC pane only + "First-time setup?" link)
  //
  // Mode is read from localStorage on mount (and whenever open flips true).
  // "Don't show again" checkbox in full mode persists the flag on close.

  type Mode = 'full' | 'compact';

  type Props = {
    open: boolean;
    sessionId: string | null;      // null = chrome-help mode (no specific session)
    onclose: () => void;
    onopenSession?: () => void;
  };

  let { open, sessionId, onclose, onopenSession }: Props = $props();

  // ── Constants ──────────────────────────────────────────────────────────────
  const DISMISS_KEY = 'jamsesh.attach-walkthrough-dismissed';

  const COMMANDS = {
    marketplace: 'claude plugin marketplace add nklisch/jamsesh',
    install: 'claude plugins install jamsesh',
  } as const;

  // Composed at render time from sessionId.
  let joinCmd = $derived(sessionId ? `/jamsesh:join ${sessionId}` : null);

  // ── Internal state ─────────────────────────────────────────────────────────
  let mode = $state<Mode>('full');
  let dismissChecked = $state(false);
  let copiedCmd = $state<string | null>(null);

  // DOM ref for focus management.
  let closeBtn = $state<HTMLButtonElement | null>(null);

  // ── Mode-on-mount: re-read flag whenever open flips true ──────────────────
  $effect(() => {
    if (!open) return;
    mode =
      typeof localStorage !== 'undefined' &&
      localStorage.getItem(DISMISS_KEY) === 'true'
        ? 'compact'
        : 'full';
  });

  // ── ESC handler — registered while open, torn down on close ───────────────
  $effect(() => {
    if (!open) return;
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        handleClose();
      }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  });

  // ── Focus management — move focus to Close button on open, restore on close
  $effect(() => {
    if (open) {
      const prev = document.activeElement as HTMLElement | null;
      const id = requestAnimationFrame(() => {
        closeBtn?.focus();
      });
      return () => {
        cancelAnimationFrame(id);
        prev?.focus();
      };
    }
  });

  // ── Behaviors ──────────────────────────────────────────────────────────────

  /** Persist dismiss flag if checked, then call parent onclose. */
  function handleClose() {
    if (dismissChecked) {
      localStorage.setItem(DISMISS_KEY, 'true');
    }
    onclose();
  }

  /** "Open session view →" button handler. */
  function handleOpenSession() {
    if (dismissChecked) {
      localStorage.setItem(DISMISS_KEY, 'true');
    }
    if (onopenSession) {
      onopenSession();
    } else {
      onclose();
    }
  }

  /** Click-to-copy with ~1.2s "Copied" feedback badge. */
  async function copyCmd(cmd: string) {
    await navigator.clipboard.writeText(cmd);
    copiedCmd = cmd;
    setTimeout(() => {
      if (copiedCmd === cmd) copiedCmd = null;
    }, 1200);
  }
</script>

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div
    class="modal-backdrop"
    role="dialog"
    aria-modal="true"
    aria-label="Attach Claude Code to this jam"
    tabindex="-1"
    onclick={(e) => {
      if (e.target === e.currentTarget) handleClose();
    }}
  >
    {#if mode === 'full'}
      <!-- ── First-time: full ceremonial card ─────────────────────────────── -->
      <article class="modal-card first-time">
        <span class="eyebrow">
          {#if sessionId}
            Session · {sessionId}
          {:else}
            Claude Code setup
          {/if}
        </span>
        <h1>Let&rsquo;s get your Claude Code attached.</h1>
        <p class="lede">
          Two <strong>shell</strong> commands to install the plugin (one-time per machine),
          then one <strong>slash command inside Claude Code</strong> to join this session.
          Click any line to copy it; a green check stays lit on copied lines.
        </p>

        <!-- Shell pane: one-time install commands -->
        <div class="terminal-wrap">
          <div class="terminal-bar">
            <span class="dots"><span></span><span></span><span></span></span>
            <span class="label">your terminal &middot; shell</span>
            <span class="label-aside">one-time setup</span>
          </div>
          <div class="terminal">
            <span class="term-comment"
              ># Skip these if you&rsquo;ve already installed the jamsesh plugin on this machine.</span
            >
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="term-line"
              class:copied={copiedCmd === COMMANDS.marketplace}
              onclick={() => copyCmd(COMMANDS.marketplace)}
            >
              <span class="check">✓</span>
              <span class="prompt">$</span>
              <span class="cmd-text"
                >claude plugin marketplace add <span class="arg">nklisch/jamsesh</span></span
              >
              <span class="hint"
                >{copiedCmd === COMMANDS.marketplace ? 'copied' : 'click to copy'}</span
              >
            </div>
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="term-line"
              class:copied={copiedCmd === COMMANDS.install}
              onclick={() => copyCmd(COMMANDS.install)}
            >
              <span class="check">✓</span>
              <span class="prompt">$</span>
              <span class="cmd-text"
                >claude plugins install <span class="arg">jamsesh</span></span
              >
              <span class="hint">{copiedCmd === COMMANDS.install ? 'copied' : 'click to copy'}</span>
            </div>
          </div>
        </div>

        <!-- Claude Code pane -->
        <p style="margin: 16px 0 8px; font-size: 12px; color: var(--color-text-tertiary); font-style: italic;">
          # Start Claude Code from a checkout of your source repo, then click the prompt
          below to copy and paste at the CC prompt:
        </p>
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
              <!-- svelte-ignore a11y_click_events_have_key_events -->
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <div
                class="cc-input"
                class:copied={copiedCmd === joinCmd}
                onclick={() => copyCmd(joinCmd!)}
              >
                <span class="cc-arrow">❯</span>
                <span class="cc-cmd">{joinCmd}</span>
                <span class="cc-hint">{copiedCmd === joinCmd ? 'copied' : 'click to copy'}</span>
                <span class="cc-check">✓</span>
              </div>
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
        <p class="cc-aftermath">
          <span class="ok">→</span> After you run that, the local <code>bin/jamsesh</code> binary
          OAuths, clones the session, and wires up post-commit + SessionStart hooks. You can close
          this dialog as soon as your CC prompt shows the jam digest.
        </p>

        <div class="card-footer">
          <label class="checkbox-row">
            <input type="checkbox" bind:checked={dismissChecked} />
            Don&rsquo;t show the full walkthrough next time &mdash; I&rsquo;ll grab just the join
            command.
          </label>
          <div class="card-actions">
            <button class="btn-ghost" bind:this={closeBtn} onclick={handleClose}>Skip for now</button>
            <button class="btn-primary" onclick={handleOpenSession}>Open session view &rarr;</button>
          </div>
        </div>
      </article>
    {:else}
      <!-- ── Returning: compact card ────────────────────────────────────────── -->
      <article class="modal-card compact">
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
              <!-- svelte-ignore a11y_click_events_have_key_events -->
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <div
                class="cc-input"
                class:copied={copiedCmd === joinCmd}
                onclick={() => copyCmd(joinCmd!)}
              >
                <span class="cc-arrow">❯</span>
                <span class="cc-cmd">{joinCmd}</span>
                <span class="cc-hint">{copiedCmd === joinCmd ? 'copied' : 'click to copy'}</span>
                <span class="cc-check">✓</span>
              </div>
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

        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <span class="reopen-link" onclick={() => (mode = 'full')}>
          First-time setup? Show the full walkthrough &rarr;
        </span>

        <div class="card-footer">
          <div></div>
          <div class="card-actions">
            <button class="btn-ghost" bind:this={closeBtn} onclick={handleClose}>Close</button>
            <button class="btn-primary" onclick={handleOpenSession}>Open session view &rarr;</button>
          </div>
        </div>
      </article>
    {/if}
  </div>
{/if}

<style>
  /* Modal backdrop — same in both states */
  .modal-backdrop {
    position: fixed;
    inset: 56px 0 0 0;
    background: rgba(20, 22, 26, 0.55);
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
  }

  /* Shared eyebrow (block flow, no stretch issue) */
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

  /* Shared modal-card shell */
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

  /* Shell terminal pane (install commands) */
  .terminal-wrap {
    border-radius: var(--radius-md);
    overflow: hidden;
    border: 1px solid #1f232a;
  }

  .terminal-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 14px;
    background: #14161a;
    color: #d9dde4;
    gap: 12px;
  }

  .terminal-bar .dots {
    display: inline-flex;
    gap: 6px;
    flex-shrink: 0;
  }

  .terminal-bar .dots span {
    width: 10px;
    height: 10px;
    border-radius: var(--radius-full);
    background: rgba(255, 255, 255, 0.2);
  }

  .terminal-bar .label {
    font: var(--font-weight-medium) var(--font-size-xs) var(--font-mono);
    opacity: 0.85;
    flex: 1;
  }

  .terminal-bar .label-aside {
    font: var(--font-size-xs) var(--font-mono);
    opacity: 0.55;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .terminal {
    background: #0c0e12;
    color: #d9dde4;
    font: var(--font-size-xs) / 1.65 var(--font-mono);
    padding: 16px 18px;
    min-width: 0;
    overflow-x: hidden;
  }

  .term-comment {
    display: block;
    padding: 4px 0 6px 4px;
    color: #6f7480;
    font-style: italic;
    word-break: break-word;
  }

  .term-line {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 8px;
    cursor: pointer;
    border-radius: var(--radius-sm);
    margin-bottom: 2px;
    min-width: 0;
    overflow-x: auto;
    white-space: nowrap;
  }

  .term-line:hover {
    background: rgba(255, 255, 255, 0.04);
  }

  .term-line.copied {
    background: rgba(74, 140, 92, 0.14);
  }

  .term-line .check {
    width: 16px;
    flex-shrink: 0;
    color: #4a8c5c;
    opacity: 0;
    font: var(--font-weight-bold) 13px var(--font-mono);
  }

  .term-line.copied .check {
    opacity: 1;
  }

  .term-line .prompt {
    color: #6f7480;
    user-select: none;
  }

  .term-line .cmd-text {
    color: #d9dde4;
    min-width: 0;
  }

  .term-line .arg {
    color: #7ec8c9;
  }

  .term-line .hint {
    margin-left: auto;
    font-size: 11px;
    color: #6f7480;
    opacity: 0;
  }

  .term-line:hover .hint {
    opacity: 1;
  }

  .term-line.copied .hint {
    color: #4a8c5c;
    opacity: 1;
  }

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

  .cc-aftermath {
    margin: 10px 4px 0;
    font-size: 11px;
    color: var(--color-text-tertiary);
    font-style: italic;
    line-height: 1.5;
  }

  .cc-aftermath .ok {
    color: var(--color-success);
    font-style: normal;
  }

  .cc-aftermath code {
    font-style: normal;
    color: var(--color-text-secondary);
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

  .checkbox-row {
    display: flex;
    gap: 8px;
    align-items: center;
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
  }

  .checkbox-row input {
    margin: 0;
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

  /* First-time: full ceremonial card */
  .modal-card.first-time {
    max-width: 760px;
    width: 100%;
    padding: 36px 40px;
  }

  .modal-card.first-time h1 {
    font-size: var(--font-size-2xl);
  }

  .modal-card.first-time .lede {
    font-size: var(--font-size-base);
    max-width: 64ch;
  }

  /* Returning: compact card */
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
    display: inline-block;
    margin-top: 14px;
    font-size: var(--font-size-xs);
    color: var(--color-text-link);
    cursor: pointer;
    text-decoration: underline dotted;
  }
</style>
