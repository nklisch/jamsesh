<script lang="ts">
  type Props = {
    lockConflict: { holderAccountId: string } | null;
    lockError: string | null;
    lock: { lock_id: string; is_caller: boolean } | null;
    isCaller: boolean;
    sessionEnded: boolean;
    onWait?: () => void;
    onOverride?: () => void;
    onDismissError?: () => void;
  };

  let {
    lockConflict,
    lockError,
    lock,
    isCaller,
    sessionEnded,
    onWait,
    onOverride,
    onDismissError,
  }: Props = $props();

  const showLockPill = $derived(lock !== null && isCaller && !sessionEnded);
</script>

{#if lockConflict}
  <!-- Lock-conflict banner -->
  <div class="conflict-banner" role="alert" aria-label="Another member is finalizing">
    <div class="conflict-text">
      <strong>{lockConflict.holderAccountId || 'Another member'}</strong>
      is finalizing this session. Wait for them, or override to
      restart curation from the current draft.
    </div>
    <div class="conflict-actions">
      <button class="btn ghost" type="button" onclick={() => onWait?.()}>
        Wait
      </button>
      <button class="btn primary" type="button" onclick={() => onOverride?.()}>
        Override
      </button>
    </div>
  </div>
{/if}

{#if lockError}
  <div class="error-banner" role="alert">
    {lockError}
    <button class="btn ghost" type="button" onclick={() => onDismissError?.()}>
      Dismiss
    </button>
  </div>
{/if}

{#if showLockPill}
  <span class="lock-pill" aria-label="You hold the lock">You hold the lock</span>
{/if}

<style>
  .conflict-banner {
    display: flex; align-items: center; gap: 16px;
    padding: 14px 22px;
    background: var(--color-warning-muted);
    border-bottom: 1px solid var(--color-border);
  }
  .conflict-text { flex: 1; font-size: var(--font-size-sm); color: var(--color-text-primary); }
  .conflict-actions { display: flex; gap: 8px; flex-shrink: 0; }

  .error-banner {
    display: flex; align-items: center; gap: 12px;
    padding: 10px 22px;
    background: var(--color-danger-muted);
    color: var(--color-danger);
    font-size: var(--font-size-sm);
  }

  .lock-pill {
    display: inline-flex; align-items: center;
    margin-left: 12px;
    padding: 4px 10px; border-radius: var(--radius-full);
    background: var(--color-accent-muted); color: var(--color-accent);
    font: var(--font-weight-medium) 11px var(--font-mono);
    letter-spacing: 0.04em; text-transform: uppercase;
  }

  .btn {
    padding: 7px 14px;
    border-radius: var(--radius-md);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    cursor: pointer;
  }
  .btn.primary {
    background: var(--color-bg-inverse); color: var(--color-text-inverse);
    border: 1px solid var(--color-bg-inverse);
    width: auto; padding: 7px 14px;
  }
  .btn.ghost {
    background: transparent; color: var(--color-text-primary);
    border: 1px solid var(--color-border-strong);
  }
</style>
