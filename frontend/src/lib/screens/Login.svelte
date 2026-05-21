<script lang="ts">
  import { navigate } from '$lib/router.svelte';
  import { auth } from '$lib/auth.svelte';
  import { client } from '$lib/api/client';
  import Button from '$lib/components/Button.svelte';
  import Input from '$lib/components/Input.svelte';
  import Card from '$lib/components/Card.svelte';

  // Mode drives card state transitions.
  // Note: once /api/auth/magic-link/request is in openapi.yaml this fetch
  // call becomes a typed client.POST. Deferred to epic-portal-foundation-auth-flows.
  type Mode = 'choose' | 'magic-link-sent' | 'magic-link-error' | 'oauth-error';

  let email = $state('');
  let mode = $state<Mode>('choose');
  let errorMsg = $state<string | null>(null);
  let oauthPending = $state(false);

  // Query params read once at mount.
  const _searchParams =
    typeof window !== 'undefined' ? new URLSearchParams(window.location.search) : new URLSearchParams();

  // Optional session name surfaced when arriving via an invite link.
  // Consumed by the "resume strip" — callers pass via query param; routing
  // support lands in epic-portal-ui-session-view-shell.
  const resumeSession: string | null = _searchParams.get('resume');

  // `return_to` — set by the App.svelte auth gate when an unauthenticated
  // user lands on a protected route that benefits from post-login resumption
  // (currently only invite-accept).  After the user authenticates via any
  // client-side path (magic-link verify, future token exchange, etc.),
  // redirect them back to the original URL instead of the generic landing.
  //
  // `return_to` is preserved across the GitHub OAuth round-trip by writing
  // it to sessionStorage in signInWithGitHub() and reading it back in the
  // OAuthCallback screen. See OAuthCallback.svelte.
  const _returnTo = _searchParams.get('return_to');
  const returnTo: string | null =
    _returnTo && _returnTo.startsWith('/') && !_returnTo.startsWith('//')
      ? _returnTo
      : null;

  // When authentication state flips to true (e.g. a future client-side token
  // exchange completes), redirect to return_to if present.  The $effect re-runs
  // whenever `auth.isAuthenticated` changes so it's safe to leave in place.
  $effect(() => {
    if (auth.isAuthenticated) {
      navigate(returnTo ?? '/');
    }
  });

  // Known-safe OAuth provider hostnames. Add one entry per future provider.
  const AUTHORIZE_HOST_ALLOWLIST = ['github.com'] as const;

  // OAuth start is a two-step flow: POST to mint a state nonce and
  // receive the provider's authorize_url, then navigate the browser to
  // that URL.
  async function signInWithGitHub() {
    if (oauthPending) return;
    oauthPending = true;
    errorMsg = null;

    sessionStorage.setItem('oauth.provider', 'github');
    if (returnTo) {
      sessionStorage.setItem('oauth.return_to', returnTo);
    } else {
      sessionStorage.removeItem('oauth.return_to');
    }

    let authorizeUrl: string | null = null;
    try {
      const { data, error } = await client.POST('/api/auth/oauth/start', {
        body: { provider: 'github' },
      });
      if (!error && data) authorizeUrl = data.authorize_url;
    } catch {
      // Network failure (offline, CORS, DNS) — fall through to error UI.
    }

    if (authorizeUrl) {
      // Defense-in-depth: validate scheme and hostname before navigating.
      // Rejects javascript: URIs, http: URLs, and any host not in the allowlist.
      let parsed: URL;
      try {
        parsed = new URL(authorizeUrl);
      } catch {
        mode = 'oauth-error';
        errorMsg = 'Authorization URL could not be validated. Please try again.';
        oauthPending = false;
        return;
      }
      if (
        parsed.protocol !== 'https:' ||
        !AUTHORIZE_HOST_ALLOWLIST.includes(parsed.hostname as (typeof AUTHORIZE_HOST_ALLOWLIST)[number])
      ) {
        mode = 'oauth-error';
        errorMsg = 'Authorization URL could not be validated. Please try again.';
        oauthPending = false;
        return;
      }
      window.location.assign(authorizeUrl);
      return;
    }

    mode = 'oauth-error';
    errorMsg = 'Could not start GitHub sign-in. Please try again.';
    oauthPending = false;
  }

  async function requestMagicLink(e: Event) {
    e.preventDefault();
    errorMsg = null;
    // Raw fetch — not yet in openapi.yaml. Replace with typed client.POST once
    // epic-portal-foundation-auth-flows adds POST /api/auth/magic-link/request.
    const res = await fetch('/api/auth/magic-link/request', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email }),
    });
    if (res.ok) {
      mode = 'magic-link-sent';
    } else {
      mode = 'magic-link-error';
      errorMsg = 'Could not send magic link. Please try again.';
    }
  }
</script>

<div class="login-wrap">
  <Card padding="lg">
    <div class="card-inner">
      {#if mode === 'choose'}
        <h1>Sign in to jamsesh</h1>
        <p class="sub">Pick whichever flow your environment supports.</p>

        {#if resumeSession}
          <div class="resume-strip" role="note">
            <svg class="resume-icon" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
              <path d="M8 0a8 8 0 100 16A8 8 0 008 0zm.75 4v4.25l3.5 2-.75 1.3-4-2.3V4z"/>
            </svg>
            <span>You'll return to <strong>{resumeSession}</strong> after signing in.</span>
          </div>
        {/if}

        <div class="method-block">
          <div class="method-label">Sign in with</div>
          <button class="oauth-btn" onclick={signInWithGitHub} type="button" disabled={oauthPending}>
            <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
              <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.012 8.012 0 0016 8c0-4.42-3.58-8-8-8z"/>
            </svg>
            Continue with GitHub
          </button>
        </div>

        <div class="divider" aria-hidden="true">or</div>

        <div class="method-block">
          <div class="method-label">Email a magic link</div>
          <form class="magic-form" onsubmit={requestMagicLink}>
            <Input type="email" bind:value={email} placeholder="you@example.com" />
            <Button variant="primary" type="submit" size="md">
              {#snippet children()}Send link{/snippet}
            </Button>
          </form>
        </div>

        <p class="footer-line">
          New to jamsesh? Either method creates your account.
        </p>

      {:else if mode === 'magic-link-sent'}
        <h1>Check your inbox</h1>
        <p class="sub">We sent a magic link to <strong>{email}</strong>. Click it to finish signing in.</p>
        <p class="footer-line">
          Wrong address?
          <button class="link-btn" onclick={() => { mode = 'choose'; }} type="button">
            Try a different email
          </button>
        </p>

      {:else}
        <h1>Something went wrong</h1>
        <p class="sub">{errorMsg}</p>
        <Button variant="ghost" onclick={() => { mode = 'choose'; errorMsg = null; }}>
          {#snippet children()}Try again{/snippet}
        </Button>
      {/if}
    </div>
  </Card>
</div>

<style>
  .login-wrap {
    min-height: 100vh;
    display: flex;
    justify-content: center;
    align-items: flex-start;
    padding: 80px 24px 120px;
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
  }

  /* Card itself is max-width constrained */
  .login-wrap :global(.card) {
    width: 100%;
    max-width: 440px;
    border-radius: var(--radius-lg);
  }

  h1 {
    margin: 0 0 var(--space-2);
    font-size: var(--font-size-2xl);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.02em;
    text-align: center;
  }

  .sub {
    margin: 0 0 var(--space-6);
    text-align: center;
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
  }

  .resume-strip {
    background: var(--color-accent-muted);
    border: 1px solid var(--color-accent);
    border-radius: var(--radius-md);
    padding: 10px 14px;
    margin-bottom: var(--space-5);
    font-size: var(--font-size-sm);
    color: var(--color-text-primary);
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .resume-icon {
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    color: var(--color-accent);
  }

  .method-block {
    padding: var(--space-4) 0;
    border-top: 1px solid var(--color-border);
  }

  .method-block:first-of-type {
    border-top: 0;
    padding-top: 0;
  }

  .method-label {
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: var(--font-weight-semibold);
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--color-text-tertiary);
    margin-bottom: var(--space-3);
  }

  .oauth-btn {
    width: 100%;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    padding: 12px 18px;
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md);
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    font-family: var(--font-sans);
    font-weight: var(--font-weight-medium);
    font-size: var(--font-size-base);
    cursor: pointer;
    transition: background-color 120ms ease;
  }

  .oauth-btn:hover:not(:disabled) {
    background: var(--color-bg-tertiary);
  }

  .oauth-btn:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  .oauth-btn svg {
    width: 18px;
    height: 18px;
    flex-shrink: 0;
  }

  .magic-form {
    display: flex;
    gap: var(--space-2);
    align-items: flex-end;
  }

  .magic-form :global(.input) {
    flex: 1;
    background: var(--color-bg-primary);
  }

  .divider {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin: var(--space-1) 0;
    color: var(--color-text-tertiary);
    font-family: var(--font-mono);
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.12em;
  }

  .divider::before,
  .divider::after {
    content: '';
    flex: 1;
    height: 1px;
    background: var(--color-border);
  }

  .footer-line {
    margin-top: var(--space-5);
    text-align: center;
    font-size: var(--font-size-sm);
    color: var(--color-text-tertiary);
  }

  .link-btn {
    background: none;
    border: none;
    padding: 0;
    color: var(--color-text-link);
    font-size: inherit;
    cursor: pointer;
    text-decoration: underline dotted;
  }
</style>
