<script lang="ts">
  import { onMount } from 'svelte';
  import { client } from '$lib/api/client';
  import { auth } from '$lib/auth.svelte';
  import { navigate } from '$lib/router.svelte';
  import type { components } from '$lib/api/types.gen';

  type PlaygroundSessionSummary = components['schemas']['PlaygroundSessionSummary'];

  // ── Props ──────────────────────────────────────────────────────────────────

  let { sessionId }: { sessionId: string } = $props();

  // ── State machine ─────────────────────────────────────────────────────────
  //
  //  idle       → joining (submit in-flight)
  //  joining    → full (409)
  //  joining    → ended (410 → redirect)
  //  joining    → error (network / 5xx)
  //  joining    → done (200 → navigate to session)

  type ViewState = 'idle' | 'joining' | 'full' | 'error';

  let viewState = $state<ViewState>('idle');
  let nickname = $state(suggestNickname());
  let errorMsg = $state<string | null>(null);

  // Nickname avatar: first two chars of nickname or first char of each word
  const avatarText = $derived(
    nickname.trim()
      ? nickname
          .trim()
          .split('-')
          .map((w: string) => w[0] ?? '')
          .slice(0, 2)
          .join('')
          .toLowerCase()
      : '??',
  );

  // ── Client-side nickname suggestion ──────────────────────────────────────
  //
  // Mirrors the server's wordlist-generator style (adjective-animal).
  // The server may assign a different handle on collision, but this gives
  // the user something reasonable to start with.

  function suggestNickname(): string {
    const adj = [
      'amber', 'azure', 'bright', 'calm', 'cool', 'crisp', 'dark', 'dim',
      'dull', 'early', 'fast', 'firm', 'flat', 'free', 'gold', 'gray',
      'green', 'high', 'kind', 'lean', 'light', 'lime', 'low', 'mint',
      'mist', 'neat', 'pale', 'pink', 'pure', 'quiet', 'rare', 'red',
      'sage', 'sharp', 'shy', 'silk', 'slim', 'slow', 'soft', 'still',
      'swift', 'teal', 'warm', 'wild', 'wise', 'woody',
    ];
    const animal = [
      'bear', 'buck', 'crow', 'deer', 'dove', 'duck', 'elk', 'fish',
      'fawn', 'finch', 'fox', 'frog', 'gull', 'hawk', 'hare', 'heron',
      'ibis', 'jay', 'kite', 'lark', 'lynx', 'mink', 'mole', 'moth',
      'newt', 'oryx', 'owl', 'pika', 'puma', 'quail', 'rail', 'rat',
      'rook', 'seal', 'shrew', 'skunk', 'slug', 'snipe', 'stag', 'tern',
      'toad', 'vole', 'wren', 'yak',
    ];
    const a = adj[Math.floor(Math.random() * adj.length)];
    const n = animal[Math.floor(Math.random() * animal.length)];
    return `${a}-${n}`;
  }

  function rerollNickname() {
    nickname = suggestNickname();
  }

  // ── Nickname validation ────────────────────────────────────────────────────

  const nicknameValid = $derived(
    /^[a-z0-9][a-z0-9-]{0,22}[a-z0-9]$|^[a-z0-9]{2}$/.test(nickname.trim()),
  );

  // ── Join handler ───────────────────────────────────────────────────────────

  async function handleJoin(e: Event) {
    e.preventDefault();
    if (!nicknameValid || viewState === 'joining') return;

    viewState = 'joining';
    errorMsg = null;

    const { data, error, response } = await client.POST('/api/playground/sessions/{id}/join', {
      params: { path: { id: sessionId } },
      body: { nickname: nickname.trim() },
    });

    if (data) {
      // Write playground context (bearer + sessionId + confirmed nickname)
      auth.setPlaygroundContext({
        sessionId: data.session.id,
        bearer: data.bearer,
        nickname: data.nickname,
      });
      navigate(`/orgs/org_playground/sessions/${data.session.id}`);
      return;
    }

    const errCode = (error as { error?: string } | undefined)?.error;

    if (response?.status === 409) {
      viewState = 'full';
      return;
    }

    if (response?.status === 410) {
      navigate(`/playground/s/${sessionId}/ended`);
      return;
    }

    errorMsg = errCode === 'playground.session_not_found'
      ? 'This session no longer exists.'
      : 'Something went wrong. Please try again.';
    viewState = 'error';
  }
</script>

<div class="page">
  <!-- Top bar -->
  <header class="top-bar">
    <div class="wordmark">jamsesh<span class="dot">.</span></div>
    <span class="playground-chip" aria-label="Playground session">⏳ playground</span>
    <div class="spacer"></div>
  </header>

  <section class="joiner-wrap">

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
            onclick={(e) => { e.preventDefault(); navigate('/playground'); }}
          >
            Try another playground →
          </a>
        </div>
      </div>

    {:else if viewState === 'error'}
      <!-- Error state -->
      <div class="error-state" role="alert">
        <h1 class="join-headline">Something went wrong.</h1>
        <p class="join-sub">{errorMsg}</p>
        <div class="actions">
          <button
            class="btn ghost"
            type="button"
            onclick={() => { viewState = 'idle'; errorMsg = null; }}
          >
            Try again
          </button>
          <a
            class="btn secondary"
            href="/playground"
            onclick={(e) => { e.preventDefault(); navigate('/playground'); }}
          >
            Back to playground
          </a>
        </div>
      </div>

    {:else}
      <!-- Join form (idle | joining) -->
      <div class="join-eyebrow">You were invited to a playground</div>
      <h1 class="join-headline">Joining a playground session</h1>
      <p class="join-sub">Pick a handle and hop in — no account, no email needed.</p>

      <form class="nickname-form" onsubmit={handleJoin} novalidate>
        <label class="field-label" for="nick">You'll join as</label>
        <div class="nickname-input-wrap">
          <span class="nickname-avatar" aria-hidden="true">{avatarText}</span>
          <input
            id="nick"
            class="nickname-input"
            type="text"
            bind:value={nickname}
            placeholder="e.g. quiet-fox"
            minlength="2"
            maxlength="24"
            pattern="[a-z0-9][a-z0-9\-]*[a-z0-9]"
            autocomplete="off"
            autocapitalize="none"
            spellcheck="false"
            disabled={viewState === 'joining'}
            aria-describedby="nick-hint"
          />
          <button
            type="button"
            class="reroll-btn"
            title="Pick a different suggestion"
            onclick={rerollNickname}
            disabled={viewState === 'joining'}
            aria-label="Suggest a different nickname"
          >⟲</button>
        </div>
        <p class="hint-row" id="nick-hint">
          <span class="info-icon" aria-hidden="true">i</span>
          We suggested <strong>{nickname || '…'}</strong> — keep it, or pick your own
          (2–24 chars, lowercase letters / digits / dashes).
        </p>

        {#if nickname.trim() && !nicknameValid}
          <p class="validation-error" role="alert">
            Nickname must be 2–24 characters, lowercase letters, digits, or dashes.
          </p>
        {/if}

        <button
          type="submit"
          class="join-btn"
          disabled={!nicknameValid || viewState === 'joining'}
        >
          {viewState === 'joining' ? 'Joining…' : `Join as ${nickname.trim() || '…'} →`}
        </button>

        <div class="ephemeral-mini" role="note">
          ⏳ <strong>Playground sessions are throwaway.</strong> When the window closes,
          your handle, comments, and commits in this session are destroyed.
          Finalize locally first if you want to keep work.
        </div>
      </form>

      <div class="doc-foot">
        <a
          href="/playground"
          onclick={(e) => { e.preventDefault(); navigate('/playground'); }}
        >
          What's a playground session?
        </a>
      </div>
    {/if}

  </section>
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

  .top-bar {
    padding: 16px 32px;
    display: flex;
    align-items: center;
    border-bottom: 1px solid var(--color-border);
    background: var(--color-bg-secondary);
    flex-shrink: 0;
  }

  .wordmark {
    font-weight: var(--font-weight-semibold);
    font-size: var(--font-size-lg);
    letter-spacing: -0.03em;
    user-select: none;
  }

  .wordmark .dot {
    color: var(--color-accent);
  }

  .playground-chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 9px;
    background: var(--color-warning-muted);
    color: var(--color-warning);
    border-radius: var(--radius-md);
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: var(--font-weight-semibold);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    margin-left: 12px;
  }

  .spacer {
    flex: 1;
  }

  /* ── Joiner container ─────────────────────────────────────────────────── */

  .joiner-wrap {
    max-width: 520px;
    margin: 0 auto;
    padding: 60px 32px 48px;
    width: 100%;
    box-sizing: border-box;
  }

  .join-eyebrow {
    font-family: var(--font-mono);
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-medium);
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--color-accent);
    margin-bottom: 12px;
    text-align: center;
  }

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

  /* ── Nickname form ────────────────────────────────────────────────────── */

  .nickname-form {
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-lg);
    padding: 24px;
  }

  .field-label {
    display: block;
    font-family: var(--font-mono);
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-semibold);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--color-text-secondary);
    margin: 0 0 8px;
  }

  .nickname-input-wrap {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .nickname-avatar {
    width: 40px;
    height: 40px;
    border-radius: 50%;
    background: var(--author-6, #7c3aed);
    color: white;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-family: var(--font-mono);
    font-size: 13px;
    font-weight: var(--font-weight-semibold);
    flex-shrink: 0;
    user-select: none;
  }

  .nickname-input {
    flex: 1;
    padding: 12px 16px;
    background: var(--color-bg-primary);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md);
    font-family: var(--font-mono);
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-medium);
    color: var(--color-text-primary);
    letter-spacing: -0.01em;
    min-width: 0;
  }

  .nickname-input:focus {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
    border-color: var(--color-accent);
  }

  .nickname-input:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .reroll-btn {
    width: 40px;
    height: 40px;
    padding: 0;
    background: var(--color-bg-tertiary);
    color: var(--color-text-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    cursor: pointer;
    font-size: 16px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    transition: background-color 120ms ease;
  }

  .reroll-btn:hover:not(:disabled) {
    background: var(--color-border);
    color: var(--color-text-primary);
  }

  .reroll-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .hint-row {
    margin: 10px 0 0;
    font-size: var(--font-size-xs);
    color: var(--color-text-tertiary);
    display: flex;
    gap: 6px;
    align-items: flex-start;
    line-height: 1.5;
  }

  .hint-row strong {
    color: var(--color-text-secondary);
  }

  .info-icon {
    width: 14px;
    height: 14px;
    border-radius: 50%;
    background: var(--color-text-tertiary);
    color: white;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 9px;
    font-weight: var(--font-weight-bold);
    flex-shrink: 0;
    margin-top: 1px;
  }

  .validation-error {
    margin: 8px 0 0;
    font-size: var(--font-size-xs);
    color: var(--color-danger, #e53e3e);
  }

  .join-btn {
    width: 100%;
    margin-top: 24px;
    padding: 14px;
    background: var(--color-accent);
    color: var(--color-text-inverse);
    border: 0;
    border-radius: var(--radius-md);
    font-family: var(--font-sans);
    font-size: var(--font-size-base);
    font-weight: var(--font-weight-semibold);
    cursor: pointer;
    transition: background-color 120ms ease;
  }

  .join-btn:hover:not(:disabled) {
    background: var(--color-accent-hover);
  }

  .join-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .ephemeral-mini {
    margin-top: 20px;
    padding: 12px 16px;
    background: var(--color-warning-muted);
    border-radius: var(--radius-md);
    border-left: 3px solid var(--color-warning);
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    text-align: center;
  }

  .ephemeral-mini strong {
    color: var(--color-warning);
    font-weight: var(--font-weight-semibold);
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

  /* ── Footer ───────────────────────────────────────────────────────────── */

  .doc-foot {
    text-align: center;
    margin-top: 28px;
    font-size: var(--font-size-sm);
    color: var(--color-text-tertiary);
  }

  .doc-foot a {
    color: var(--color-text-link);
    text-decoration: none;
  }

  .doc-foot a:hover {
    text-decoration: underline;
  }
</style>
