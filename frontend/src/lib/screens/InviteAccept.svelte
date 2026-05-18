<script lang="ts">
  import { onMount } from 'svelte';
  import { client } from '$lib/api/client';
  import { auth } from '$lib/auth.svelte';
  import { navigate } from '$lib/router.svelte';
  import type { components } from '$lib/api/types.gen';

  type SessionInviteDetails = components['schemas']['SessionInviteDetails'];

  // ── Props ──────────────────────────────────────────────────────────────────

  let { orgId, sessionId, inviteId }: { orgId: string; sessionId: string; inviteId: string } =
    $props();

  // ── State machine ─────────────────────────────────────────────────────────
  //
  //  loading  → ready (GET 200)
  //  loading  → error (GET non-200, missing token)
  //  ready    → accepting (POST in flight)
  //  accepting → navigate away (POST 200)
  //  accepting → rejection (POST 403 auth.org_membership_required)
  //  accepting → error (POST other failure)

  type ViewState = 'loading' | 'ready' | 'accepting' | 'rejection' | 'error';

  let viewState = $state<ViewState>('loading');
  let inviteDetails = $state<SessionInviteDetails | null>(null);
  let errorCode = $state<string | null>(null);
  let token = $state<string | null>(null);

  // ── Mount: extract token + fetch invite details ───────────────────────────

  onMount(() => {
    const params = new URLSearchParams(window.location.search);
    const t = params.get('token');
    if (!t) {
      errorCode = 'missing_token';
      viewState = 'error';
      return;
    }
    token = t;
    void loadInviteDetails(t);
  });

  async function loadInviteDetails(t: string) {
    const { data, error } = await client.GET(
      '/api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}',
      {
        params: {
          path: { orgID: orgId, sessionID: sessionId, inviteID: inviteId },
          query: { token: t },
        },
      },
    );

    if (error) {
      // error is typed as ErrorEnvelope; fall back to a generic code if the
      // field isn't populated (e.g. network failure).
      errorCode = (error as { error?: string }).error ?? 'network_error';
      viewState = 'error';
    } else if (data) {
      inviteDetails = data;
      viewState = 'ready';
    }
  }

  // ── Accept ────────────────────────────────────────────────────────────────

  async function handleAccept() {
    if (!token) return;
    viewState = 'accepting';

    const { data, error, response } = await client.POST(
      '/api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept',
      {
        params: { path: { orgID: orgId, sessionID: sessionId, inviteID: inviteId } },
        body: { token },
      },
    );

    if (data) {
      navigate(`/orgs/${orgId}/sessions/${sessionId}`);
      return;
    }

    // 403 + auth.org_membership_required → rejection state
    const errCode = (error as { error?: string } | undefined)?.error;
    if (response?.status === 403 && errCode === 'auth.org_membership_required') {
      viewState = 'rejection';
      return;
    }

    // All other failures → error state
    errorCode = errCode ?? 'accept_failed';
    viewState = 'error';
  }

  // ── Decline ───────────────────────────────────────────────────────────────
  //
  // Navigate to the org's session list if authenticated (the org in the URL
  // is the destination org; even if the user isn't a member yet, it's a
  // reasonable fallback — the backend will show an empty list or gate them).
  // If not authenticated, fall back to /login.

  function handleDecline() {
    if (auth.isAuthenticated) {
      navigate(`/orgs/${orgId}/sessions`);
    } else {
      navigate('/login');
    }
  }

  // ── Inviter initials helper ───────────────────────────────────────────────

  function initials(name: string): string {
    return name
      .split(' ')
      .filter(Boolean)
      .map((w) => w[0])
      .slice(0, 2)
      .join('')
      .toUpperCase();
  }
</script>

<div class="page">
  <!-- Top bar -->
  <header class="topbar">
    <div class="wordmark">jam<span class="dot">·</span>sesh</div>
    <div class="topbar-spacer"></div>
    {#if auth.currentUser}
      <span class="user-dot" title={auth.currentUser.displayName}>
        {initials(auth.currentUser.displayName)}
      </span>
    {/if}
  </header>

  <!-- Hero region -->
  <main class="hero">

    {#if viewState === 'loading'}
      <!-- Loading skeleton -->
      <p class="loading-text" aria-busy="true">Checking invite…</p>

    {:else if viewState === 'ready' || viewState === 'accepting'}
      <!-- Happy path: invite details + Accept/Decline -->
      {#if inviteDetails}
        <div class="from" aria-label="Invited by {inviteDetails.invited_by_name}">
          <span class="inviter-dot" aria-hidden="true">
            {initials(inviteDetails.invited_by_name)}
          </span>
          <span>Invited by <strong>{inviteDetails.invited_by_name}</strong></span>
        </div>

        <h1>
          You're invited to
          <span class="hl">{inviteDetails.session_name}</span>
        </h1>

        <p class="lead">
          A jamsesh session in <strong>{inviteDetails.org_name}</strong> where humans
          and their AI agents collaborate on shared artifacts in real git.
        </p>

        <div class="actions">
          <button class="btn link" type="button" onclick={handleDecline}>
            Decline
          </button>
          <button
            class="btn primary"
            type="button"
            disabled={viewState === 'accepting'}
            onclick={handleAccept}
          >
            {viewState === 'accepting' ? 'Joining…' : 'Accept & open session'}
          </button>
        </div>

        <div class="explainer">
          <h3>What happens when you accept</h3>
          <p>
            You'll join the session as a <code>member</code>. Your Claude Code instance
            can fetch the session's git history, push commits to your own ref namespace,
            and see everyone else's work as it lands.
          </p>
          <p>You can leave anytime. Your own source repo is never touched by jamsesh.</p>
        </div>
      {/if}

    {:else if viewState === 'rejection'}
      <!-- Members-only rejection -->
      {#if inviteDetails}
        <div class="from" aria-label="Invited by {inviteDetails.invited_by_name}">
          <span class="inviter-dot" aria-hidden="true">
            {initials(inviteDetails.invited_by_name)}
          </span>
          <span>Invited by <strong>{inviteDetails.invited_by_name}</strong></span>
        </div>
      {/if}

      <h1>
        {inviteDetails?.org_name ?? 'This org'} is
        <span class="hl hl-warning">members only</span>
      </h1>

      <p class="lead">This org requires you to be a member before joining sessions.</p>

      <div class="alert warning" role="alert">
        <strong>
          Ask an admin to add you to {inviteDetails?.org_name ?? 'the org'} first.
        </strong>
        After you're added as an org member, re-open this invite link and we'll add
        you to the <code>{inviteDetails?.session_name ?? 'session'}</code> session automatically.
      </div>

      <div class="actions">
        <button class="btn ghost" type="button" onclick={handleDecline}>
          Back to your sessions
        </button>
      </div>

    {:else}
      <!-- Error state -->
      <h1>This invite is no longer valid</h1>

      <p class="lead">
        The link is expired, has already been accepted, or the token doesn't match.
      </p>

      <div class="alert danger" role="alert">
        <strong>
          Server returned <code>{errorCode ?? 'unknown_error'}</code>.
        </strong>
        Ask whoever sent the invite to issue a fresh one. The original is no longer usable.
      </div>

      <div class="actions">
        <button class="btn ghost" type="button" onclick={handleDecline}>
          Back to your sessions
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

  /* ── Top bar ──────────────────────────────────────────────────────────── */

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

  .topbar-spacer {
    flex: 1;
  }

  .user-dot {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--author-4);
    color: #fff;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    font-weight: var(--font-weight-semibold);
    flex-shrink: 0;
  }

  /* ── Hero ─────────────────────────────────────────────────────────────── */

  .hero {
    max-width: 680px;
    margin: 0 auto;
    padding: 80px 32px 64px;
    text-align: center;
    width: 100%;
    box-sizing: border-box;
  }

  .loading-text {
    color: var(--color-text-secondary);
    font-size: var(--font-size-lg);
  }

  /* ── Invited-by pill ──────────────────────────────────────────────────── */

  .from {
    display: inline-flex;
    gap: 8px;
    align-items: center;
    padding: 4px 12px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-full);
    background: var(--color-bg-secondary);
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    margin-bottom: 24px;
  }

  .from strong {
    color: var(--color-text-primary);
    font-weight: var(--font-weight-medium);
  }

  .inviter-dot {
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: var(--author-2);
    color: #fff;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 10px;
    font-weight: var(--font-weight-semibold);
    flex-shrink: 0;
  }

  /* ── Headline ─────────────────────────────────────────────────────────── */

  h1 {
    margin: 0 0 12px;
    font-size: 36px;
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.025em;
    line-height: 1.15;
  }

  .hl {
    color: var(--color-accent);
    font-weight: var(--font-weight-semibold);
  }

  .hl-warning {
    color: var(--color-warning);
  }

  /* ── Lead ─────────────────────────────────────────────────────────────── */

  .lead {
    font-size: var(--font-size-lg);
    color: var(--color-text-secondary);
    line-height: 1.5;
    max-width: 540px;
    margin: 0 auto 32px;
  }

  /* ── CTA group ────────────────────────────────────────────────────────── */

  .actions {
    display: flex;
    gap: 12px;
    justify-content: center;
    margin-top: 32px;
  }

  .btn {
    border-radius: var(--radius-md);
    padding: 10px 24px;
    font-size: var(--font-size-base);
    font-weight: var(--font-weight-medium);
    border: 1px solid transparent;
    cursor: pointer;
    font-family: var(--font-sans);
    transition: background-color 120ms ease, opacity 120ms ease;
  }

  .btn:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .btn.primary {
    background: var(--color-accent);
    color: #fff;
    border-color: var(--color-accent);
  }

  .btn.primary:hover:not(:disabled) {
    background: var(--color-accent-hover);
    border-color: var(--color-accent-hover);
  }

  .btn.ghost {
    background: transparent;
    color: var(--color-text-primary);
    border-color: var(--color-border-strong);
  }

  .btn.ghost:hover {
    background: var(--color-bg-tertiary);
  }

  .btn.link {
    background: transparent;
    border: none;
    color: var(--color-text-link);
    padding: 10px 8px;
    cursor: pointer;
  }

  .btn.link:hover {
    text-decoration: underline;
  }

  /* ── Explainer card ───────────────────────────────────────────────────── */

  .explainer {
    max-width: 560px;
    margin: 32px auto 0;
    padding: 20px 24px;
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    text-align: left;
  }

  .explainer h3 {
    margin: 0 0 8px;
    font-size: var(--font-size-base);
    font-weight: var(--font-weight-semibold);
  }

  .explainer p {
    margin: 0 0 8px;
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    line-height: 1.6;
  }

  .explainer p:last-child {
    margin-bottom: 0;
  }

  .explainer code {
    font-family: var(--font-mono);
    background: var(--color-bg-tertiary);
    padding: 1px 5px;
    border-radius: var(--radius-sm);
    font-size: 0.9em;
  }

  /* ── Alert blocks ─────────────────────────────────────────────────────── */

  .alert {
    max-width: 560px;
    margin: 0 auto;
    padding: 16px 20px;
    border-radius: var(--radius-md);
    text-align: left;
    font-size: var(--font-size-sm);
    line-height: 1.6;
  }

  .alert.warning {
    background: var(--color-warning-muted);
    border-left: 4px solid var(--color-warning);
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
</style>
