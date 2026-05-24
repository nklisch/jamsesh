<script lang="ts">
  // Pure rendering component: receives the two generated command strings and
  // emits clipboard copy events. No internal mutations of parent state.

  let {
    skillCommand,
    cliCommand,
    onEditForm,
    onDone,
  }: {
    skillCommand: string;
    cliCommand: string;
    onEditForm: () => void;
    onDone: () => void;
  } = $props();

  let skillCopied = $state(false);
  let cliCopied = $state(false);

  async function copySkill() {
    await navigator.clipboard.writeText(skillCommand);
    skillCopied = true;
    setTimeout(() => { skillCopied = false; }, 2000);
  }

  async function copyCli() {
    await navigator.clipboard.writeText(cliCommand);
    cliCopied = true;
    setTimeout(() => { cliCopied = false; }, 2000);
  }
</script>

<div class="drawer-output">
  <p class="output-intro">
    Use one of these commands to create your session. Paste the skill
    form into a Claude Code composer, or the CLI form into a terminal.
  </p>

  <section class="cmd-section">
    <div class="cmd-section-header">
      <span class="cmd-label">Skill form</span>
      <span class="cmd-sublabel">Paste into Claude Code</span>
    </div>
    <div class="cmd-block">
      <code class="cmd-text">{skillCommand}</code>
      <button
        class="copy-btn"
        class:copied={skillCopied}
        onclick={copySkill}
        aria-label="Copy skill command"
      >
        {skillCopied ? 'Copied!' : 'Copy'}
      </button>
    </div>
  </section>

  <section class="cmd-section">
    <div class="cmd-section-header">
      <span class="cmd-label">CLI form</span>
      <span class="cmd-sublabel">Paste into a terminal</span>
    </div>
    <div class="cmd-block">
      <code class="cmd-text">{cliCommand}</code>
      <button
        class="copy-btn"
        class:copied={cliCopied}
        onclick={copyCli}
        aria-label="Copy CLI command"
      >
        {cliCopied ? 'Copied!' : 'Copy'}
      </button>
    </div>
  </section>

  <div class="drawer-actions">
    <button type="button" class="btn-ghost" onclick={onEditForm}>
      Edit form
    </button>
    <button type="button" class="btn-primary" onclick={onDone}>
      Done
    </button>
  </div>
</div>

<style>
  .drawer-output {
    display: flex;
    flex-direction: column;
    gap: 20px;
    padding: 24px;
    flex: 1;
  }

  .output-intro {
    margin: 0;
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    line-height: 1.5;
  }

  .cmd-section {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .cmd-section-header {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }

  .cmd-label {
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-semibold);
    color: var(--color-text-primary);
  }

  .cmd-sublabel {
    font-size: var(--font-size-xs);
    color: var(--color-text-tertiary);
  }

  .cmd-block {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    padding: 12px;
  }

  .cmd-text {
    flex: 1;
    font: var(--font-size-sm) var(--font-mono);
    color: var(--color-text-primary);
    word-break: break-all;
    white-space: pre-wrap;
    line-height: 1.5;
  }

  .copy-btn {
    flex-shrink: 0;
    padding: 4px 10px;
    background: transparent;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    color: var(--color-text-secondary);
    font: var(--font-weight-medium) var(--font-size-xs) var(--font-sans);
    cursor: pointer;
    transition: color 0.1s, border-color 0.1s, background 0.1s;
    white-space: nowrap;
  }

  .copy-btn:hover {
    color: var(--color-text-primary);
    border-color: var(--color-border-strong);
  }

  .copy-btn.copied {
    color: var(--color-success, #22c55e);
    border-color: var(--color-success, #22c55e);
    background: var(--color-success-muted, rgba(34, 197, 94, 0.08));
  }

  /* ── Shared actions bar ────────────────────────────────────────────────── */

  .drawer-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: auto;
    padding-top: 8px;
  }

  .btn-ghost {
    padding: 8px 16px;
    background: transparent;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    color: var(--color-text-secondary);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    cursor: pointer;
  }

  .btn-ghost:hover {
    color: var(--color-text-primary);
    border-color: var(--color-border-strong);
  }

  .btn-primary {
    padding: 8px 16px;
    background: var(--color-bg-inverse);
    color: var(--color-text-inverse);
    border: 0;
    border-radius: var(--radius-md);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    cursor: pointer;
  }

  .btn-primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
