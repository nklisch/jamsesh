<script lang="ts">
  import { client } from '$lib/api/client';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';

  let {
    orgId,
    sessionId,
    ref,
    currentMode,
    onclose,
    onsuccess,
  }: {
    orgId: string;
    sessionId: string;
    ref: string;
    currentMode?: 'sync' | 'isolated';
    onclose?: () => void;
    onsuccess?: () => void;
  } = $props();

  // Default to the opposite of current mode.
  let _initialMode: 'sync' | 'isolated' = currentMode === 'isolated' ? 'sync' : 'isolated';
  let selectedMode = $state<'sync' | 'isolated'>(_initialMode);
  let submitting = $state(false);
  let submitError = $state<string | null>(null);

  async function handleSubmit(e: Event) {
    e.preventDefault();
    submitting = true;
    submitError = null;

    const { error } = await client.POST(
      '/api/orgs/{orgID}/sessions/{sessionID}/ref-modes',
      {
        params: { path: { orgID: orgId, sessionID: sessionId } },
        body: { ref, mode: selectedMode },
      },
    );

    submitting = false;
    if (error) {
      submitError = 'Failed to update mode.';
    } else {
      onsuccess?.();
      onclose?.();
    }
  }

  let shortRef = $derived(ref.split('/').slice(-2).join('/'));
</script>

<Modal open={true} title="Switch mode" ariaLabel="Switch ref mode" size="sm" {onclose}>
  <form class="modal-body" onsubmit={handleSubmit}>
    <div class="field">
      <span class="label">Ref</span>
      <code class="mono-value" title={ref}>{shortRef}</code>
    </div>

    <div class="field">
      <span class="label">Current mode</span>
      <span class="mode-badge mode-{currentMode ?? 'unknown'}">{currentMode ?? '—'}</span>
    </div>

    <fieldset class="mode-fieldset">
      <legend class="legend">New mode</legend>
      <label class="radio-label">
        <input
          type="radio"
          name="mode"
          value="sync"
          bind:group={selectedMode}
        />
        <span class="radio-text">
          <strong>sync</strong>
          <span class="desc">Auto-merge peer commits into this ref.</span>
        </span>
      </label>
      <label class="radio-label">
        <input
          type="radio"
          name="mode"
          value="isolated"
          bind:group={selectedMode}
        />
        <span class="radio-text">
          <strong>isolated</strong>
          <span class="desc">No auto-merge; this ref stays independent.</span>
        </span>
      </label>
    </fieldset>

    {#if submitError}
      <p class="error" role="alert">{submitError}</p>
    {/if}

    <div class="actions">
      <Button variant="ghost" size="sm" onclick={() => onclose?.()}>Cancel</Button>
      <Button
        variant="accent"
        size="sm"
        type="submit"
        disabled={submitting || selectedMode === currentMode}
      >
        {submitting ? 'Switching…' : 'Switch mode'}
      </Button>
    </div>
  </form>
</Modal>

<style>
  .modal-body {
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .field {
    display: grid;
    grid-template-columns: 110px 1fr;
    align-items: center;
    gap: 10px;
  }

  .label {
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
  }

  .mono-value {
    font-family: var(--font-mono);
    font-size: var(--font-size-sm);
    color: var(--color-text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mode-badge {
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: var(--font-weight-semibold);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 3px;
  }

  .mode-sync {
    background: var(--color-accent-muted);
    color: var(--color-accent);
  }

  .mode-isolated {
    background: var(--color-warning-muted);
    color: var(--color-warning);
  }

  .mode-unknown {
    background: var(--color-bg-tertiary);
    color: var(--color-text-secondary);
  }

  .mode-fieldset {
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .legend {
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    padding: 0 4px;
  }

  .radio-label {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    cursor: pointer;
  }

  .radio-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .radio-text strong {
    font-size: var(--font-size-sm);
    color: var(--color-text-primary);
    font-family: var(--font-mono);
  }

  .desc {
    font-size: 11px;
    color: var(--color-text-tertiary);
  }

  .error {
    color: var(--color-danger);
    font-size: var(--font-size-sm);
    margin: 0;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding-top: 4px;
  }
</style>
