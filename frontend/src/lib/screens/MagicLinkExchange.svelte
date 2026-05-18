<script lang="ts">
  import { onMount } from 'svelte';
  import { client } from '$lib/api/client';
  import { auth } from '$lib/auth.svelte';
  import { navigate } from '$lib/router.svelte';

  // ── State machine ─────────────────────────────────────────────────────────
  //
  //  exchanging → done (POST 200: store tokens, redirect)
  //  exchanging → error (POST non-200, or missing token in hash)

  type ViewState = 'exchanging' | 'error';

  let viewState = $state<ViewState>('exchanging');
  let errorCode = $state<string | null>(null);

  // ── Mount: read token from fragment and exchange ───────────────────────────

  onMount(() => {
    // Token arrives in the URL fragment (#token=...) so it is never sent to
    // the server and does not appear in proxy access logs.
    const hash = window.location.hash.slice(1); // strip leading '#'
    const params = new URLSearchParams(hash);
    const token = params.get('token');

    if (!token) {
      errorCode = 'missing_token';
      viewState = 'error';
      return;
    }

    // Clear the fragment immediately so the token is not left in browser
    // history or visible in developer tools after this point.
    history.replaceState(null, '', window.location.pathname);

    void exchange(token);
  });

  async function exchange(token: string) {
    const { data, error } = await client.POST('/api/auth/magic-link/exchange', {
      body: { token },
    });

    if (data) {
      auth.setTokens(data.access_token, data.refresh_token);
      // Navigate to the `return_to` destination if the URL carried one (the
      // auth gate may have attached it before redirecting to login, and login
      // forwards it as a query param here).  Fall back to /login, which will
      // transition to the sessions landing once the user's org is resolved.
      const searchParams = new URLSearchParams(window.location.search);
      const returnTo = searchParams.get('return_to');
      if (returnTo && returnTo.startsWith('/') && !returnTo.startsWith('//')) {
        navigate(returnTo);
      } else {
        navigate('/login');
      }
      return;
    }

    const errCode = (error as { error?: string } | undefined)?.error;
    errorCode = errCode ?? 'exchange_failed';
    viewState = 'error';
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
      <h1>This link is no longer valid</h1>
      <p class="lead">
        The magic link is expired, has already been used, or the token doesn't match.
      </p>
      <div class="alert danger" role="alert">
        <strong>Error: <code>{errorCode ?? 'unknown_error'}</code></strong>
        Request a new magic link from the sign-in page.
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
