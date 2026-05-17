<script lang="ts">
  import { client } from '$lib/api/client';
  import type { components } from '$lib/api/types.gen';

  type Session = components['schemas']['Session'];

  let {
    orgId,
    oncreated,
    onclose,
  }: {
    orgId: string;
    oncreated: (session: Session) => void;
    onclose: () => void;
  } = $props();

  let name = $state('');
  let goal = $state('');
  let scopeRaw = $state('');
  let defaultMode = $state<'sync' | 'isolated'>('sync');
  let isSubmitting = $state(false);
  let submitError = $state<string | null>(null);

  function parseScopeGlobs(raw: string): string[] {
    return raw
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
  }

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    if (!name.trim()) return;

    const globs = parseScopeGlobs(scopeRaw);
    const scopeJson = JSON.stringify(globs);

    isSubmitting = true;
    submitError = null;

    const { data, error } = await client.POST('/api/orgs/{orgID}/sessions', {
      params: { path: { orgID: orgId } },
      body: {
        name: name.trim(),
        goal: goal.trim(),
        scope: scopeJson,
        default_mode: defaultMode,
      },
    });

    isSubmitting = false;

    if (error) {
      submitError = 'Failed to create session. Please try again.';
      return;
    }

    if (data) {
      oncreated(data);
    }
  }

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

    <form class="drawer-form" onsubmit={handleSubmit}>
      <div class="field">
        <label for="session-name">Name</label>
        <input
          id="session-name"
          type="text"
          placeholder="e.g. Fix auth bug"
          bind:value={name}
          required
          disabled={isSubmitting}
        />
      </div>

      <div class="field">
        <label for="session-goal">Goal</label>
        <textarea
          id="session-goal"
          placeholder="Describe the objective of this session…"
          rows={3}
          bind:value={goal}
          disabled={isSubmitting}
        ></textarea>
      </div>

      <div class="field">
        <label for="session-scope">Scope</label>
        <textarea
          id="session-scope"
          placeholder="Comma-separated globs, e.g. src/auth/**, tests/auth/**"
          rows={2}
          bind:value={scopeRaw}
          disabled={isSubmitting}
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
            disabled={isSubmitting}
          >
            sync
          </button>
          <button
            type="button"
            class="mode-opt"
            class:active={defaultMode === 'isolated'}
            onclick={() => (defaultMode = 'isolated')}
            aria-pressed={defaultMode === 'isolated'}
            disabled={isSubmitting}
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

      {#if submitError}
        <p class="submit-error" role="alert">{submitError}</p>
      {/if}

      <div class="drawer-actions">
        <button type="button" class="btn-ghost" onclick={onclose} disabled={isSubmitting}>
          Cancel
        </button>
        <button type="submit" class="btn-primary" disabled={isSubmitting || !name.trim()}>
          {isSubmitting ? 'Creating…' : 'Create session'}
        </button>
      </div>
    </form>
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
    width: 420px;
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

  .field input:disabled,
  .field textarea:disabled {
    opacity: 0.6;
    cursor: not-allowed;
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

  .mode-opt:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .submit-error {
    margin: 0;
    font-size: var(--font-size-sm);
    color: var(--color-danger);
  }

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
