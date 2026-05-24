<script lang="ts">
  // FullCard — first-time ceremonial card.
  //
  // Renders the full "two shell commands + CC join" walkthrough. Shown when
  // the user has never dismissed the full walkthrough (localStorage flag absent).

  import CcPane from './CcPane.svelte';

  const COMMANDS = {
    marketplace: 'claude plugin marketplace add nklisch/jamsesh',
    install: 'claude plugins install jamsesh',
  } as const;

  type Props = {
    sessionId: string | null;
    joinCmd: string | null;
    copiedCmd: string | null;
    dismissChecked: boolean;
    closeBtnRef?: HTMLButtonElement | null;
    oncopy: (cmd: string) => void;
    ondismisschange: (checked: boolean) => void;
    onclose: () => void;
    onopensession: () => void;
  };

  let {
    sessionId,
    joinCmd,
    copiedCmd,
    dismissChecked,
    closeBtnRef = $bindable(),
    oncopy,
    ondismisschange,
    onclose,
    onopensession,
  }: Props = $props();
</script>

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
        onclick={() => oncopy(COMMANDS.marketplace)}
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
        onclick={() => oncopy(COMMANDS.install)}
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
  <CcPane {joinCmd} {copiedCmd} oncopy={oncopy} />
  <p class="cc-aftermath">
    <span class="ok">→</span> After you run that, the local <code>bin/jamsesh</code> binary
    OAuths, clones the session, and wires up post-commit + SessionStart hooks. You can close
    this dialog as soon as your CC prompt shows the jam digest.
  </p>

  <div class="card-footer">
    <label class="checkbox-row">
      <input
        type="checkbox"
        checked={dismissChecked}
        onchange={(e) => ondismisschange((e.target as HTMLInputElement).checked)}
      />
      Don&rsquo;t show the full walkthrough next time &mdash; I&rsquo;ll grab just the join
      command.
    </label>
    <div class="card-actions">
      <button class="btn-ghost" bind:this={closeBtnRef} onclick={onclose}>Skip for now</button>
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

  /* Shell terminal pane */
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

  /* First-time: full ceremonial card sizing */
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
</style>
