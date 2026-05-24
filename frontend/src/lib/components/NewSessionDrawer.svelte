<script lang="ts">
  // ── View state ────────────────────────────────────────────────────────────
  //
  //  form   → initial state; user fills in fields and clicks "Generate commands"
  //  output → commands rendered; user copies skill or raw CLI form and closes
  //
  type ViewState = 'form' | 'output';

  let {
    orgId,
    onclose,
  }: {
    orgId: string;
    onclose: () => void;
  } = $props();

  let viewState = $state<ViewState>('form');

  // ── Form fields ───────────────────────────────────────────────────────────

  let goal = $state('');
  let scopeRaw = $state('');
  let defaultMode = $state<'sync' | 'isolated'>('sync');
  let inviteesRaw = $state('');
  let validationError = $state<string | null>(null);

  // ── Clipboard copy state ──────────────────────────────────────────────────

  let skillCopied = $state(false);
  let cliCopied = $state(false);

  // ── Command output (set on submit) ────────────────────────────────────────

  let skillCommand = $state('');
  let cliCommand = $state('');

  // ── Helpers ───────────────────────────────────────────────────────────────

  /**
   * Shell-escape a value: if it contains any character that would be
   * interpreted by the shell (spaces, quotes, $, !, *, ?, etc.) wrap it
   * in single quotes, escaping any embedded single quotes as '\'' .
   * Plain alphanumeric / dash / dot / slash / star / comma values are
   * returned as-is (common in globs like "src/**" and emails like "a@x.com").
   */
  function shellEscape(value: string): string {
    // Characters safe without quoting inside a shell argument:
    //   alphanumeric, -, _, ., /, *, **, comma, @
    if (/^[A-Za-z0-9@._,/*-]+$/.test(value)) {
      return value;
    }
    // Wrap in single quotes; escape embedded single quotes as '\''
    return `'${value.replace(/'/g, "'\\''")}'`;
  }

  function parseScopeGlobs(raw: string): string[] {
    return raw
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
  }

  function parseInvitees(raw: string): string[] {
    return raw
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
  }

  /**
   * Build the flag list shared between both command forms.
   * Empty-field omission: if a value is empty after trimming, the
   * corresponding flag is excluded from the rendered command.
   */
  function buildFlags(org: string, goalVal: string, scopeVal: string, mode: string, inviteesVal: string): string {
    const flags: string[] = [];

    flags.push(`--org ${shellEscape(org)}`);

    const goalTrimmed = goalVal.trim();
    if (goalTrimmed) {
      flags.push(`--goal ${shellEscape(goalTrimmed)}`);
    }

    const globs = parseScopeGlobs(scopeVal);
    if (globs.length > 0) {
      // Join globs back into a single comma-separated value, then escape as one arg
      flags.push(`--scope ${shellEscape(globs.join(','))}`);
    }

    flags.push(`--mode ${mode}`);

    const invitees = parseInvitees(inviteesVal);
    if (invitees.length > 0) {
      flags.push(`--invite ${shellEscape(invitees.join(','))}`);
    }

    return flags.join(' ');
  }

  // ── Submit ────────────────────────────────────────────────────────────────

  function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    validationError = null;

    if (!orgId.trim()) {
      validationError = 'No org selected.';
      return;
    }

    // Validate scope globs if provided
    const globs = parseScopeGlobs(scopeRaw);
    if (scopeRaw.trim() && globs.length === 0) {
      validationError = 'Scope must be at least one valid path glob.';
      return;
    }

    const flags = buildFlags(orgId, goal, scopeRaw, defaultMode, inviteesRaw);
    skillCommand = `/jamsesh:jam ${flags}`;
    cliCommand = `jamsesh new ${flags}`;
    viewState = 'output';
  }

  function handleReset() {
    viewState = 'form';
    skillCopied = false;
    cliCopied = false;
  }

  // ── Clipboard ─────────────────────────────────────────────────────────────

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

  // ── Keyboard / backdrop ───────────────────────────────────────────────────

  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) {
      onclose();
    }
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      onclose();
    }
  }
</script>

<svelte:window onkeydown={handleKeyDown} />

<div
  class="drawer-backdrop"
  role="dialog"
  aria-modal="true"
  aria-label="New session"
  tabindex="-1"
  onclick={handleBackdropClick}
  onkeydown={(e) => { if (e.key === 'Escape') onclose(); }}
>
  <div class="drawer">
    <div class="drawer-header">
      <h2>New session</h2>
      <button class="close-btn" onclick={onclose} aria-label="Close">✕</button>
    </div>

    {#if viewState === 'form'}
      <form class="drawer-form" onsubmit={handleSubmit}>
        <div class="field">
          <label for="session-goal">Goal</label>
          <textarea
            id="session-goal"
            placeholder="Describe the objective of this session…"
            rows={3}
            bind:value={goal}
          ></textarea>
        </div>

        <div class="field">
          <label for="session-scope">Scope</label>
          <textarea
            id="session-scope"
            placeholder="Comma-separated globs, e.g. src/auth/**, tests/auth/**"
            rows={2}
            bind:value={scopeRaw}
          ></textarea>
          <p class="field-hint">Comma-separated path globs defining the writable scope.</p>
        </div>

        <div class="field">
          <span class="label">Default mode</span>
          <div class="mode-toggle" role="group" aria-label="Default mode">
            <button
              type="button"
              class="mode-opt"
              class:active={defaultMode === 'sync'}
              onclick={() => (defaultMode = 'sync')}
              aria-pressed={defaultMode === 'sync'}
            >
              sync
            </button>
            <button
              type="button"
              class="mode-opt"
              class:active={defaultMode === 'isolated'}
              onclick={() => (defaultMode = 'isolated')}
              aria-pressed={defaultMode === 'isolated'}
            >
              isolated
            </button>
          </div>
          <p class="field-hint">
            {defaultMode === 'sync'
              ? 'Commits auto-merge into the shared draft.'
              : 'Branches stay isolated until manually merged.'}
          </p>
        </div>

        <div class="field">
          <label for="session-invitees">Invite</label>
          <input
            id="session-invitees"
            type="text"
            placeholder="e.g. alice@example.com, bob@example.com"
            bind:value={inviteesRaw}
          />
          <p class="field-hint">Comma-separated email addresses to invite.</p>
        </div>

        {#if validationError}
          <p class="submit-error" role="alert">{validationError}</p>
        {/if}

        <div class="drawer-actions">
          <button type="button" class="btn-ghost" onclick={onclose}>
            Cancel
          </button>
          <button type="submit" class="btn-primary">
            Generate commands
          </button>
        </div>
      </form>
    {:else}
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
          <button type="button" class="btn-ghost" onclick={handleReset}>
            Edit form
          </button>
          <button type="button" class="btn-primary" onclick={onclose}>
            Done
          </button>
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .drawer-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.45);
    z-index: 50;
    display: flex;
    align-items: flex-start;
    justify-content: flex-end;
  }

  .drawer {
    width: 480px;
    max-width: 100vw;
    height: 100%;
    background: var(--color-bg-primary);
    border-left: 1px solid var(--color-border);
    display: flex;
    flex-direction: column;
    overflow-y: auto;
  }

  .drawer-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 20px 24px 16px;
    border-bottom: 1px solid var(--color-border);
    flex-shrink: 0;
  }

  .drawer-header h2 {
    margin: 0;
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.01em;
  }

  .close-btn {
    background: transparent;
    border: 0;
    color: var(--color-text-secondary);
    cursor: pointer;
    font-size: 16px;
    padding: 4px;
    line-height: 1;
  }

  .close-btn:hover {
    color: var(--color-text-primary);
  }

  /* ── Form view ─────────────────────────────────────────────────────────── */

  .drawer-form {
    display: flex;
    flex-direction: column;
    gap: 20px;
    padding: 24px;
    flex: 1;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .field label,
  .field .label {
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-medium);
    color: var(--color-text-primary);
  }

  .field input,
  .field textarea {
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    padding: 8px 12px;
    color: var(--color-text-primary);
    font: var(--font-size-sm) var(--font-sans);
    resize: vertical;
  }

  .field input:focus,
  .field textarea:focus {
    outline: none;
    border-color: var(--color-accent);
    box-shadow: 0 0 0 2px var(--color-accent-muted);
  }

  .field-hint {
    margin: 0;
    font-size: var(--font-size-xs);
    color: var(--color-text-tertiary);
  }

  .mode-toggle {
    display: flex;
    gap: 0;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    overflow: hidden;
    width: fit-content;
  }

  .mode-opt {
    padding: 6px 16px;
    background: transparent;
    border: 0;
    color: var(--color-text-secondary);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-mono);
    cursor: pointer;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    font-size: 11px;
  }

  .mode-opt + .mode-opt {
    border-left: 1px solid var(--color-border);
  }

  .mode-opt.active {
    background: var(--color-bg-inverse);
    color: var(--color-text-inverse);
  }

  .submit-error {
    margin: 0;
    font-size: var(--font-size-sm);
    color: var(--color-danger);
  }

  /* ── Output view ───────────────────────────────────────────────────────── */

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
