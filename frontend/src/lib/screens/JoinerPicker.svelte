<script lang="ts">
  import { client } from '$lib/api/client';
  import { auth } from '$lib/auth.svelte';
  import { navigate } from '$lib/router.svelte';
  import JoinerForm from '$lib/components/JoinerForm.svelte';
  import JoinerOutcome from '$lib/components/JoinerOutcome.svelte';

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
  let errorMsg = $state<string | null>(null);

  // ── Join handler ───────────────────────────────────────────────────────────

  async function handleJoin(nickname: string) {
    if (viewState === 'joining') return;

    viewState = 'joining';
    errorMsg = null;

    const { data, error, response } = await client.POST('/api/playground/sessions/{id}/join', {
      params: { path: { id: sessionId } },
      body: { nickname },
    });

    if (data) {
      // Write playground context (bearer + sessionId + confirmed nickname)
      auth.setPlaygroundContext({
        sessionId: data.session.id,
        bearer: data.bearer,
        nickname: data.nickname,
        expiresAt: data.expires_at,
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

  function handleRetry() {
    viewState = 'idle';
    errorMsg = null;
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

    {#if viewState === 'full' || viewState === 'error'}
      <JoinerOutcome
        viewState={viewState}
        errorMsg={errorMsg}
        onretry={handleRetry}
        onnavplayground={() => navigate('/playground')}
      />
    {:else}
      <JoinerForm
        viewState={viewState}
        onjoin={handleJoin}
        onnavplayground={() => navigate('/playground')}
      />
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
</style>
