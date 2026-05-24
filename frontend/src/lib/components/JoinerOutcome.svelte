<script lang="ts">
  // ── Props ──────────────────────────────────────────────────────────────────
  //
  // viewState drives which outcome panel is shown:
  //
  //   full   → session-full alert with "Try another playground" CTA
  //   error  → generic error with retry + back-to-playground actions

  let {
    viewState,
    errorMsg,
    onretry,
    onnavplayground,
  }: {
    viewState: 'full' | 'error';
    errorMsg: string | null;
    onretry: () => void;
    onnavplayground: () => void;
  } = $props();
</script>

{#if viewState === 'full'}
  <!-- Session full state -->
  <div class="full-state" role="alert">
    <div class="full-icon" aria-hidden="true">🚫</div>
    <h1 class="join-headline">This session is full.</h1>
    <p class="join-sub">
      This playground session has reached its participant limit.
      You can start or join a different playground session.
    </p>
    <div class="actions">
      <a
        class="btn primary"
        href="/playground"
        onclick={(e) => { e.preventDefault(); onnavplayground(); }}
      >
        Try another playground →
      </a>
    </div>
  </div>

{:else}
  <!-- Error state -->
  <div class="error-state" role="alert">
    <h1 class="join-headline">Something went wrong.</h1>
    <p class="join-sub">{errorMsg}</p>
    <div class="actions">
      <button
        class="btn ghost"
        type="button"
        onclick={onretry}
      >
        Try again
      </button>
      <a
        class="btn secondary"
        href="/playground"
        onclick={(e) => { e.preventDefault(); onnavplayground(); }}
      >
        Back to playground
      </a>
    </div>
  </div>
{/if}

<style>
  .join-headline {
    font-size: var(--font-size-2xl);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.02em;
    text-align: center;
    margin: 0 0 6px;
  }

  .join-sub {
    text-align: center;
    color: var(--color-text-secondary);
    font-size: var(--font-size-base);
    margin: 0 0 36px;
  }

  /* ── Full / Error states ──────────────────────────────────────────────── */

  .full-state,
  .error-state {
    text-align: center;
    padding: 48px 0;
  }

  .full-icon {
    font-size: 40px;
    margin-bottom: 16px;
  }

  .actions {
    display: flex;
    gap: 12px;
    justify-content: center;
    flex-wrap: wrap;
    margin-top: 28px;
  }

  .btn {
    padding: 12px 22px;
    border-radius: var(--radius-md);
    font-family: var(--font-sans);
    font-size: var(--font-size-base);
    font-weight: var(--font-weight-medium);
    cursor: pointer;
    text-decoration: none;
    border: 1px solid transparent;
    display: inline-flex;
    align-items: center;
    transition: background-color 120ms ease;
  }

  .btn.primary {
    background: var(--color-accent);
    color: var(--color-text-inverse);
  }

  .btn.primary:hover {
    background: var(--color-accent-hover);
  }

  .btn.ghost,
  .btn.secondary {
    background: var(--color-bg-secondary);
    color: var(--color-text-primary);
    border-color: var(--color-border-strong);
  }

  .btn.ghost:hover,
  .btn.secondary:hover {
    background: var(--color-bg-tertiary);
  }
</style>
