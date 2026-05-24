<script lang="ts">
  import { onMount } from 'svelte';
  import { client } from '$lib/api/client';
  import { navigate } from '$lib/router.svelte';
  import type { components } from '$lib/api/types.gen';

  type PlaygroundTombstone = components['schemas']['PlaygroundTombstone'];

  // ── Props ──────────────────────────────────────────────────────────────────

  let { sessionId }: { sessionId: string } = $props();

  // ── State machine ─────────────────────────────────────────────────────────
  //
  //  loading → done (GET 200, tombstone found)
  //  loading → live (GET 404, session still active → redirect to live session)
  //  loading → error (network / unexpected status)

  type ViewState = 'loading' | 'done' | 'error';

  let viewState = $state<ViewState>('loading');
  let tombstone = $state<PlaygroundTombstone | null>(null);
  let errorMsg = $state<string | null>(null);

  // ── Computed stats ─────────────────────────────────────────────────────────

  const durationLabel = $derived(
    tombstone ? formatDuration(tombstone.duration_seconds) : '',
  );

  function formatDuration(seconds: number): string {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
  }

  // ── Mount: fetch tombstone ─────────────────────────────────────────────────

  onMount(() => {
    void loadTombstone();
  });

  async function loadTombstone() {
    viewState = 'loading';
    errorMsg = null;

    let data, error, response;
    try {
      ({ data, error, response } = await client.GET(
        '/api/playground/sessions/{id}/tombstone',
        { params: { path: { id: sessionId } } },
      ));
    } catch {
      // Transport-level failure (network offline, CORS, DNS).
      errorMsg = 'Could not reach the server. Please try again.';
      viewState = 'error';
      return;
    }

    if (data) {
      tombstone = data;
      viewState = 'done';
      return;
    }

    if (response?.status === 404) {
      // 404 means the session is still active (tombstone doesn't exist yet).
      // Redirect to the live session view.
      navigate(`/orgs/org_playground/sessions/${sessionId}`);
      return;
    }

    // Unexpected error — show a simple error state.
    errorMsg = (error as { message?: string } | undefined)?.message
      ?? 'Could not load session details. Please try again.';
    viewState = 'error';
  }
</script>

<div class="page">
  <!-- Top bar -->
  <header class="top-bar">
    <div class="wordmark">jamsesh<span class="dot">.</span></div>
    <div class="spacer"></div>
    <a
      class="sign-in"
      href="/login"
      onclick={(e) => { e.preventDefault(); navigate('/login'); }}
    >
      Sign in →
    </a>
  </header>

  <main class="main">

    {#if viewState === 'loading'}
      <p class="loading-text" aria-busy="true">Loading session details…</p>

    {:else if viewState === 'done' && tombstone}
      <div class="destroyed-page">
        <div class="destroyed-emoji" aria-hidden="true">⏳</div>
        <h1>This playground session has ended.</h1>
        <p class="lede">
          All session data was destroyed — refs, comments, conflict events, presence,
          and the bare repo. If you finalized locally before the window closed,
          your work lives in your source repo now.
        </p>

        <!-- Stats row -->
        <div class="destroyed-stats" aria-label="Session summary">
          <div class="stat">
            <span class="stat-n">{tombstone.members_count}</span>
            <span class="stat-l">members</span>
          </div>
          <div class="stat">
            <span class="stat-n">{tombstone.commits_count}</span>
            <span class="stat-l">commits</span>
          </div>
          <div class="stat">
            <span class="stat-n">{tombstone.auto_merges_count}</span>
            <span class="stat-l">auto-merges</span>
          </div>
          <div class="stat">
            <span class="stat-n">{durationLabel}</span>
            <span class="stat-l">duration</span>
          </div>
        </div>

        <!-- CTAs -->
        <div class="destroyed-ctas">
          <a
            class="cta-btn primary"
            href="/playground"
            onclick={(e) => { e.preventDefault(); navigate('/playground'); }}
          >
            Try another playground →
          </a>
          <a
            class="cta-btn secondary"
            href="/"
            onclick={(e) => { e.preventDefault(); navigate('/'); }}
          >
            Sign up for a durable account
          </a>
        </div>

        <p class="destroyed-foot">
          Want to keep the next one?
          <a
            href="/"
            onclick={(e) => { e.preventDefault(); navigate('/'); }}
          >
            Sign in or create an org
          </a>
          — durable sessions don't expire and live inside your team's tenant.
        </p>
      </div>

    {:else if viewState === 'error'}
      <div class="error-state" role="alert">
        <h1>Something went wrong.</h1>
        <p class="lede">{errorMsg}</p>
        <div class="error-actions">
          <button class="cta-btn secondary" type="button" onclick={() => loadTombstone()}>
            Try again
          </button>
          <a
            class="cta-btn primary"
            href="/playground"
            onclick={(e) => { e.preventDefault(); navigate('/playground'); }}
          >
            Back to playground
          </a>
        </div>
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

  .spacer {
    flex: 1;
  }

  .sign-in {
    color: var(--color-text-secondary);
    text-decoration: none;
    font-size: var(--font-size-sm);
  }

  .sign-in:hover {
    color: var(--color-text-link);
  }

  /* ── Main ─────────────────────────────────────────────────────────────── */

  .main {
    flex: 1;
    display: flex;
    align-items: flex-start;
    justify-content: center;
  }

  /* ── Loading ─────────────────────────────────────────────────────────── */

  .loading-text {
    margin-top: 80px;
    color: var(--color-text-secondary);
    font-size: var(--font-size-lg);
  }

  /* ── Post-destruction page ────────────────────────────────────────────── */

  .destroyed-page {
    padding: 64px 24px 56px;
    text-align: center;
    max-width: 640px;
    width: 100%;
    box-sizing: border-box;
  }

  .destroyed-emoji {
    font-size: 40px;
    margin-bottom: 16px;
    filter: grayscale(0.4);
  }

  h1 {
    margin: 0 0 8px;
    font-size: var(--font-size-2xl);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.02em;
  }

  .lede {
    margin: 0 0 28px;
    color: var(--color-text-secondary);
    font-size: var(--font-size-base);
    line-height: 1.6;
    max-width: 480px;
    margin-left: auto;
    margin-right: auto;
  }

  /* ── Stats ────────────────────────────────────────────────────────────── */

  .destroyed-stats {
    display: inline-flex;
    gap: 28px;
    padding: 16px 24px;
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    margin-bottom: 32px;
    flex-wrap: wrap;
    justify-content: center;
  }

  .stat {
    text-align: center;
  }

  .stat-n {
    display: block;
    font-family: var(--font-mono);
    font-size: var(--font-size-xl);
    font-weight: var(--font-weight-semibold);
    color: var(--color-text-primary);
    letter-spacing: -0.02em;
  }

  .stat-l {
    display: block;
    font-family: var(--font-mono);
    font-size: var(--font-size-xs);
    color: var(--color-text-tertiary);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    margin-top: 4px;
  }

  /* ── CTAs ─────────────────────────────────────────────────────────────── */

  .destroyed-ctas {
    display: flex;
    gap: 12px;
    justify-content: center;
    flex-wrap: wrap;
  }

  .cta-btn {
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

  .cta-btn.primary {
    background: var(--color-accent);
    color: var(--color-text-inverse);
  }

  .cta-btn.primary:hover {
    background: var(--color-accent-hover);
  }

  .cta-btn.secondary {
    background: var(--color-bg-secondary);
    color: var(--color-text-primary);
    border-color: var(--color-border-strong);
  }

  .cta-btn.secondary:hover {
    background: var(--color-bg-tertiary);
  }

  /* ── Footer ───────────────────────────────────────────────────────────── */

  .destroyed-foot {
    margin-top: 36px;
    color: var(--color-text-tertiary);
    font-size: var(--font-size-sm);
  }

  .destroyed-foot a {
    color: var(--color-text-link);
    text-decoration: none;
  }

  .destroyed-foot a:hover {
    text-decoration: underline;
  }

  /* ── Error state ──────────────────────────────────────────────────────── */

  .error-state {
    padding: 64px 24px 56px;
    text-align: center;
    max-width: 640px;
    width: 100%;
    box-sizing: border-box;
  }

  .error-actions {
    display: flex;
    gap: 12px;
    justify-content: center;
    flex-wrap: wrap;
    margin-top: 24px;
  }
</style>
