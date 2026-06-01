<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/auth.svelte';
  import { navigate } from '$lib/router.svelte';

  // ── State machine ─────────────────────────────────────────────────────────
  //
  //  exchanging → confirming (200 + existing differing/unconfirmable identity)
  //  exchanging → done       (200 + no existing identity → adopt + navigate)
  //  exchanging → error      (non-200, network, or missing #rt in hash)
  //  confirming → done       (user accepts → adopt + navigate)
  //  confirming → error      (user declines → retry hint, bearer NOT persisted)

  type ViewState = 'exchanging' | 'confirming' | 'error';

  let viewState = $state<ViewState>('exchanging');

  // Retain only the display-safe fields in reactive state — never the bearer
  // or the raw exchange response.
  let pendingDisplayName = $state<string | null>(null);

  // Held outside reactive state so it is never rendered/logged. Cleared on
  // decline and after adoption.
  let _pendingBearer: string | null = null;
  let _pendingSessionId: string | null = null;
  let _pendingOrgId: string | null = null;
  let _pendingKind: 'playground' | 'durable' | null = null;
  let _pendingExpiresAt: string | null = null;

  // ── Helpers ───────────────────────────────────────────────────────────────

  const RETRY_HINT = 'This resume link expired or was already used — run the command again from your terminal.';

  const baseUrl = typeof window !== 'undefined' ? window.location.origin : '';

  function requiresConfirm(accountId: string): boolean {
    // No existing identity at all → adopt directly (common case).
    if (!auth.token && !auth.playgroundContext) return false;
    // Durable identity whose id matches → adopt directly.
    if (auth.currentUser?.id === accountId) return false;
    // Any other case (differing id, authenticated but currentUser null,
    // or an existing playground context with no account_id) → confirm.
    return true;
  }

  // ── Mount: read #rt from hash and exchange ─────────────────────────────────

  onMount(() => {
    // #rt arrives in the URL fragment so it is never sent to the server in
    // Referer headers and does not appear in proxy access logs.
    const hash = window.location.hash.slice(1); // strip leading '#'
    const params = new URLSearchParams(hash);
    const resumeToken = params.get('rt');

    if (!resumeToken) {
      viewState = 'error';
      return;
    }

    // Strip the fragment immediately — do NOT leave the token in browser
    // history or DevTools after this point. Include search so we don't lose
    // any query params on the route (none expected, but defensive).
    history.replaceState(null, '', window.location.pathname + window.location.search);

    void exchange(resumeToken);
  });

  // ── Exchange ───────────────────────────────────────────────────────────────

  async function exchange(resumeToken: string) {
    // BARE fetch — NOT the shared client. The shared client's bearerMiddleware
    // attaches any existing account's token, which must NOT accompany a
    // public exchange. The resume token is the sole credential here.
    let res: Response;
    try {
      res = await fetch(`${baseUrl}/api/session-resumes/exchange`, {
        method: 'POST',
        credentials: 'omit',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ resume_token: resumeToken }),
      });
    } catch {
      viewState = 'error';
      return;
    }

    if (!res.ok) {
      viewState = 'error';
      return;
    }

    let body: {
      bearer: string;
      expires_at: string;
      session_id: string;
      org_id: string;
      kind: 'playground' | 'durable';
      account_id: string;
      display_name: string;
    };
    try {
      body = await res.json() as typeof body;
    } catch {
      viewState = 'error';
      return;
    }

    if (!body.bearer || !body.session_id || !body.kind) {
      viewState = 'error';
      return;
    }

    // Stash the adoption data outside reactive state (never rendered).
    _pendingBearer = body.bearer;
    _pendingSessionId = body.session_id;
    _pendingOrgId = body.org_id;
    _pendingKind = body.kind;
    _pendingExpiresAt = body.expires_at;

    // Always capture the display_name so adopt() can use it as the nickname
    // on BOTH the direct and confirm paths (mirrors JoinerPicker's shape).
    pendingDisplayName = body.display_name;

    if (requiresConfirm(body.account_id)) {
      // Surface only the display-safe field; keep the bearer non-reactive.
      viewState = 'confirming';
      return;
    }

    // No existing identity — adopt directly.
    adopt();
  }

  // ── Adoption ───────────────────────────────────────────────────────────────

  function adopt() {
    if (!_pendingBearer || !_pendingSessionId || !_pendingKind) {
      viewState = 'error';
      return;
    }

    if (_pendingKind === 'playground') {
      if (!_pendingExpiresAt) {
        viewState = 'error';
        return;
      }
      // Mirror JoinerPicker's successful-join handling exactly.
      auth.setPlaygroundContext({
        sessionId: _pendingSessionId,
        bearer: _pendingBearer,
        // display_name is our closest equivalent to a nickname for a resumed
        // playground session — the server tracks the original nickname but
        // the exchange response uses display_name.
        nickname: pendingDisplayName ?? _pendingSessionId,
        expiresAt: _pendingExpiresAt,
      });
      navigate(`/orgs/org_playground/sessions/${encodeURIComponent(_pendingSessionId)}`);
    } else {
      // durable — access-only (no refresh token from the exchange)
      auth.setAccessOnly(_pendingBearer);
      const orgId = _pendingOrgId ?? '';
      navigate(`/orgs/${encodeURIComponent(orgId)}/sessions/${encodeURIComponent(_pendingSessionId)}`);
    }

    // Clear non-reactive holders after adoption so they are not retained in
    // memory longer than needed.
    _pendingBearer = null;
    _pendingSessionId = null;
    _pendingOrgId = null;
    _pendingKind = null;
    _pendingExpiresAt = null;
  }

  // ── Confirm handlers ───────────────────────────────────────────────────────

  function handleAccept() {
    adopt();
  }

  function handleDecline() {
    // Do NOT persist the bearer. Wipe all held adoption state.
    _pendingBearer = null;
    _pendingSessionId = null;
    _pendingOrgId = null;
    _pendingKind = null;
    _pendingExpiresAt = null;
    pendingDisplayName = null;
    viewState = 'error';
  }
</script>

<div class="page">
  <header class="topbar">
    <div class="wordmark">jam<span class="dot">·</span>sesh</div>
  </header>

  <main class="hero">
    {#if viewState === 'exchanging'}
      <p class="status" aria-busy="true">Resuming your session…</p>

    {:else if viewState === 'confirming'}
      <h1>Resume as a different account?</h1>
      <p class="lead">
        You are currently signed in. This resume link belongs to
        <strong>{pendingDisplayName ?? 'another account'}</strong>.
        Continuing will replace your current session.
      </p>
      <div class="actions">
        <button class="btn primary" type="button" onclick={handleAccept}>
          Continue as {pendingDisplayName ?? 'this account'}
        </button>
        <button class="btn ghost" type="button" onclick={handleDecline}>
          Cancel
        </button>
      </div>

    {:else}
      <h1>This resume link has expired</h1>
      <p class="lead">
        {RETRY_HINT}
      </p>
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

  .btn.primary {
    background: var(--color-accent);
    color: var(--color-bg-primary);
  }

  .btn.primary:hover {
    opacity: 0.9;
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
