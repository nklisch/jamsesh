<script lang="ts">
  import { createNewSessionForm } from './useNewSessionForm.svelte';
  import SessionCommandPreview from './SessionCommandPreview.svelte';

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
  const form = createNewSessionForm();

  // ── Submit ────────────────────────────────────────────────────────────────

  function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    if (form.submit(orgId)) {
      viewState = 'output';
    }
  }

  function handleReset() {
    viewState = 'form';
    form.reset();
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
            value={form.goal}
            oninput={(e) => form.setGoal((e.target as HTMLTextAreaElement).value)}
          ></textarea>
        </div>

        <div class="field">
          <label for="session-scope">Scope</label>
          <textarea
            id="session-scope"
            placeholder="Comma-separated globs, e.g. src/auth/**, tests/auth/**"
            rows={2}
            value={form.scopeRaw}
            oninput={(e) => form.setScopeRaw((e.target as HTMLTextAreaElement).value)}
          ></textarea>
          <p class="field-hint">Comma-separated path globs defining the writable scope.</p>
        </div>

        <div class="field">
          <span class="label">Default mode</span>
          <div class="mode-toggle" role="group" aria-label="Default mode">
            <button
              type="button"
              class="mode-opt"
              class:active={form.defaultMode === 'sync'}
              onclick={() => form.setDefaultMode('sync')}
              aria-pressed={form.defaultMode === 'sync'}
            >
              sync
            </button>
            <button
              type="button"
              class="mode-opt"
              class:active={form.defaultMode === 'isolated'}
              onclick={() => form.setDefaultMode('isolated')}
              aria-pressed={form.defaultMode === 'isolated'}
            >
              isolated
            </button>
          </div>
          <p class="field-hint">
            {form.defaultMode === 'sync'
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
            value={form.inviteesRaw}
            oninput={(e) => form.setInviteesRaw((e.target as HTMLInputElement).value)}
          />
          <p class="field-hint">Comma-separated email addresses to invite.</p>
        </div>

        {#if form.validationError}
          <p class="submit-error" role="alert">{form.validationError}</p>
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
      <SessionCommandPreview
        skillCommand={form.skillCommand}
        cliCommand={form.cliCommand}
        onEditForm={handleReset}
        onDone={onclose}
      />
    {/if}
  </div>
</div>

<style>
  /* ── Shell ─────────────────────────────────────────────────────────────── */
  .drawer-backdrop {
    position: fixed; inset: 0; background: rgba(0, 0, 0, 0.45);
    z-index: 50; display: flex; align-items: flex-start; justify-content: flex-end;
  }
  .drawer {
    width: 480px; max-width: 100vw; height: 100%; overflow-y: auto;
    background: var(--color-bg-primary); border-left: 1px solid var(--color-border);
    display: flex; flex-direction: column;
  }
  .drawer-header {
    display: flex; align-items: center; justify-content: space-between;
    padding: 20px 24px 16px; border-bottom: 1px solid var(--color-border); flex-shrink: 0;
  }
  .drawer-header h2 { margin: 0; font-size: var(--font-size-lg); font-weight: var(--font-weight-semibold); letter-spacing: -0.01em; }
  .close-btn { background: transparent; border: 0; color: var(--color-text-secondary); cursor: pointer; font-size: 16px; padding: 4px; line-height: 1; }
  .close-btn:hover { color: var(--color-text-primary); }

  /* ── Form view ─────────────────────────────────────────────────────────── */
  .drawer-form { display: flex; flex-direction: column; gap: 20px; padding: 24px; flex: 1; }
  .field { display: flex; flex-direction: column; gap: 6px; }
  .field label, .field .label { font-size: var(--font-size-sm); font-weight: var(--font-weight-medium); color: var(--color-text-primary); }
  .field input, .field textarea {
    background: var(--color-bg-secondary); border: 1px solid var(--color-border);
    border-radius: var(--radius-md); padding: 8px 12px;
    color: var(--color-text-primary); font: var(--font-size-sm) var(--font-sans); resize: vertical;
  }
  .field input:focus, .field textarea:focus { outline: none; border-color: var(--color-accent); box-shadow: 0 0 0 2px var(--color-accent-muted); }
  .field-hint { margin: 0; font-size: var(--font-size-xs); color: var(--color-text-tertiary); }
  .mode-toggle { display: flex; gap: 0; border: 1px solid var(--color-border); border-radius: var(--radius-md); overflow: hidden; width: fit-content; }
  .mode-opt {
    padding: 6px 16px; background: transparent; border: 0; color: var(--color-text-secondary);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-mono);
    cursor: pointer; text-transform: uppercase; letter-spacing: 0.05em; font-size: 11px;
  }
  .mode-opt + .mode-opt { border-left: 1px solid var(--color-border); }
  .mode-opt.active { background: var(--color-bg-inverse); color: var(--color-text-inverse); }
  .submit-error { margin: 0; font-size: var(--font-size-sm); color: var(--color-danger); }

  /* ── Actions bar ───────────────────────────────────────────────────────── */
  .drawer-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: auto; padding-top: 8px; }
  .btn-ghost {
    padding: 8px 16px; background: transparent; border: 1px solid var(--color-border);
    border-radius: var(--radius-md); color: var(--color-text-secondary);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans); cursor: pointer;
  }
  .btn-ghost:hover { color: var(--color-text-primary); border-color: var(--color-border-strong); }
  .btn-primary {
    padding: 8px 16px; background: var(--color-bg-inverse); color: var(--color-text-inverse);
    border: 0; border-radius: var(--radius-md);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans); cursor: pointer;
  }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
