<script lang="ts">
  import { onMount } from 'svelte';
  import { client } from '$lib/api/client';
  import { auth } from '$lib/auth.svelte';
  import { navigate } from '$lib/router.svelte';

  // ── State machine ─────────────────────────────────────────────────────────
  //
  //  exchanging → done (POST 200: store tokens, redirect)
  //  exchanging → error (POST non-200, missing params, or network failure)

  type ViewState = 'exchanging' | 'error';

  let viewState = $state<ViewState>('exchanging');
  let errorCode = $state<string | null>(null);

  // ── Mount: read code+state from query params and exchange ─────────────────

  onMount(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get('code');
    const state = params.get('state');

    if (!code || !state) {
      errorCode = 'missing_params';
      viewState = 'error';
      return;
    }

    // Clear code+state from browser history so they don't linger after exchange.
    history.replaceState(null, '', window.location.pathname);

    const provider = sessionStorage.getItem('oauth.provider') ?? 'github';
    const storedReturnTo = sessionStorage.getItem('oauth.return_to');
    const returnTo =
      storedReturnTo && storedReturnTo.startsWith('/') && !storedReturnTo.startsWith('//')
        ? storedReturnTo
        : null;

    sessionStorage.removeItem('oauth.provider');
    sessionStorage.removeItem('oauth.return_to');

    void exchange(provider, code, state, returnTo);
  });

  async function exchange(provider: string, code: string, state: string, returnTo: string | null) {
    try {
      const { data, error } = await client.POST('/api/auth/oauth/callback', {
        body: { provider, code, state },
      });

      if (data) {
        auth.setTokens(data.access_token, data.refresh_token);
        try {
          await auth.loadCurrentUser();
        } catch {
          // /api/me failed — tokens are valid, navigate anyway.
          // App.svelte's bootstrap effect will retry on next render.
        }
        navigate(returnTo ?? '/');
        return;
      }

      errorCode = (error as { error?: string } | undefined)?.error ?? 'exchange_failed';
      viewState = 'error';
    } catch {
      errorCode = 'exchange_failed';
      viewState = 'error';
    }
  }
</script>

<div class="page">
  <header class="topbar">
    <div class="wordmark">jam<span class="dot">·</span>sesh</div>
  </header>

  <main class="hero">
    {#if viewState === 'exchanging'}
      <p class="status" aria-busy="true">Signing you in…</p>

    {:else}
      <h1>This sign-in link is no longer valid</h1>
      <p class="lead">
        The sign-in attempt is expired, has already been used, or the state doesn't match.
      </p>
      <div class="alert danger" role="alert">
        <strong>Error: <code>{errorCode ?? 'unknown_error'}</code></strong>
        Return to the sign-in page and try again.
      </div>
      <div class="actions">
        <button class="btn ghost" type="button" onclick={() => navigate('/login')}>
          Back to sign in
        </button>
      </div>
    {/if}
  </main>
</div>

<style>
  .page {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    font-family: var(--font-sans);
  }

  .topbar {
    padding: 14px 24px;
    display: flex;
    align-items: center;
    border-bottom: 1px solid var(--color-border);
    background: var(--color-bg-secondary);
    flex-shrink: 0;
  }

  .wordmark {
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.02em;
    font-size: var(--font-size-lg);
    user-select: none;
  }

  .wordmark .dot {
    color: var(--color-accent);
  }

  .hero {
    max-width: 560px;
    margin: 0 auto;
    padding: 80px 32px 64px;
    text-align: center;
    width: 100%;
    box-sizing: border-box;
  }

  .status {
    color: var(--color-text-secondary);
    font-size: var(--font-size-lg);
  }

  h1 {
    margin: 0 0 12px;
    font-size: 32px;
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.025em;
    line-height: 1.2;
  }

  .lead {
    font-size: var(--font-size-lg);
    color: var(--color-text-secondary);
    line-height: 1.5;
    margin: 0 auto 24px;
  }

  .alert {
    padding: 16px 20px;
    border-radius: var(--radius-md);
    text-align: left;
    font-size: var(--font-size-sm);
    line-height: 1.6;
    margin: 0 auto 24px;
  }

  .alert.danger {
    background: var(--color-danger-muted);
    border-left: 4px solid var(--color-danger);
  }

  .alert strong {
    display: block;
    margin-bottom: 4px;
    font-size: var(--font-size-base);
  }

  .alert code {
    font-family: var(--font-mono);
    background: rgba(0, 0, 0, 0.06);
    padding: 1px 5px;
    border-radius: var(--radius-sm);
    font-size: 0.9em;
  }

  .actions {
    display: flex;
    gap: 12px;
    justify-content: center;
  }

  .btn {
    border-radius: var(--radius-md);
    padding: 10px 24px;
    font-size: var(--font-size-base);
    font-weight: var(--font-weight-medium);
    border: 1px solid transparent;
    cursor: pointer;
    font-family: var(--font-sans);
    transition: background-color 120ms ease;
  }

  .btn.ghost {
    background: transparent;
    color: var(--color-text-primary);
    border-color: var(--color-border-strong);
  }

  .btn.ghost:hover {
    background: var(--color-bg-tertiary);
  }
</style>
