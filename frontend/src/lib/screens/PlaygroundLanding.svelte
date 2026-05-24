<script lang="ts">
  import { navigate } from '$lib/router.svelte';

  // Copy-to-clipboard helper — no external deps needed.
  async function copyCmd(text: string, btn: HTMLButtonElement) {
    try {
      await navigator.clipboard.writeText(text);
      const orig = btn.textContent;
      btn.textContent = 'Copied!';
      setTimeout(() => { btn.textContent = orig; }, 1600);
    } catch {
      // Clipboard permission denied; silently no-op. The text is still visible.
    }
  }

  const INSTALL_CMD = 'claude plugin marketplace add nklisch/jamsesh && claude plugins install jamsesh';
  const NEW_CMD = 'jamsesh playground new';
</script>

<!-- Top bar -->
<div class="page">
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

  <section class="hero">
    <div class="eyebrow">Playground · no account needed</div>
    <h1>Run a multi-agent jam in under a minute.</h1>
    <p class="lede">
      A throwaway session you can spin up from your own repo, share with
      collaborators by URL, and let the auto-merger weave your agents'
      commits together — without OAuth, an org, or a single config file.
    </p>

    <div class="install-card">
      <p class="step-label">
        <span class="step-num">1</span>
        Install the Claude Code plugin
        <span class="step-note">(run inside any Claude Code session)</span>
      </p>
      <div class="cmd-box">
        <code>{INSTALL_CMD}</code>
        <button
          class="copy-btn"
          type="button"
          onclick={(e) => copyCmd(INSTALL_CMD, e.currentTarget)}
        >
          Copy
        </button>
      </div>

      <p class="step-label">
        <span class="step-num">2</span>
        From any local checkout
        <span class="step-note">
          (or run <code class="inline-code">jamsesh new --playground</code> if you prefer the unified create-command)
        </span>
      </p>
      <div class="cmd-box">
        <code>{NEW_CMD}</code>
        <button
          class="copy-btn"
          type="button"
          onclick={(e) => copyCmd(NEW_CMD, e.currentTarget)}
        >
          Copy
        </button>
      </div>

      <p class="skip-note">Already have jamsesh installed? Run <code class="inline-code">jamsesh playground new</code> from your checkout.</p>
    </div>

    <div class="ephemeral-note" role="note">
      <span class="ephemeral-icon" aria-hidden="true">⏳</span>
      <span>
        <strong>Ephemeral.</strong> Sessions last up to <strong>24h from creation</strong>
        or <strong>30 min of inactivity</strong>, whichever comes first.
        No data persists past destruction — finalize locally before the window closes if you want to keep work.
      </span>
    </div>
  </section>

  <section class="what-is" aria-label="What jamsesh playground gives you">
    <div class="what-card">
      <h3>Real git</h3>
      <p>Every push hits a real bare repo. No parallel VCS. Recovery is <code class="inline-code">git fetch</code>.</p>
    </div>
    <div class="what-card">
      <h3>Auto-merger</h3>
      <p>Non-conflicting commits weave into a shared draft as they arrive — no end-of-session merge ordeal.</p>
    </div>
    <div class="what-card">
      <h3>Addressed comments</h3>
      <p>Drop a line for <code class="inline-code">@quiet-fox</code> or <code class="inline-code">@all-agents</code> and it lands in their next turn's digest.</p>
    </div>
  </section>

  <footer class="doc-foot">
    Want a durable account instead?
    <a href="/" onclick={(e) => { e.preventDefault(); navigate('/'); }}>Sign up →</a>
  </footer>
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

  /* ── Hero ─────────────────────────────────────────────────────────────── */

  .hero {
    max-width: 720px;
    margin: 0 auto;
    padding: 80px 32px 48px;
    text-align: center;
    width: 100%;
    box-sizing: border-box;
  }

  .eyebrow {
    font-family: var(--font-mono);
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-medium);
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--color-accent);
    margin-bottom: 16px;
  }

  h1 {
    font-size: var(--font-size-3xl);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.025em;
    line-height: 1.15;
    margin: 0 0 16px;
  }

  .lede {
    font-size: var(--font-size-lg);
    color: var(--color-text-secondary);
    line-height: var(--line-height-base);
    margin: 0 auto 32px;
    max-width: 560px;
  }

  /* ── Install card ─────────────────────────────────────────────────────── */

  .install-card {
    max-width: 640px;
    margin: 0 auto;
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-lg);
    padding: 28px 32px;
    text-align: left;
  }

  .step-label {
    display: flex;
    gap: 10px;
    align-items: baseline;
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-semibold);
    color: var(--color-text-secondary);
    letter-spacing: 0.08em;
    text-transform: uppercase;
    margin: 0 0 8px;
    flex-wrap: wrap;
  }

  .step-num {
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: var(--color-accent-muted);
    color: var(--color-accent);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: var(--font-weight-semibold);
    letter-spacing: 0;
    flex-shrink: 0;
  }

  .step-note {
    font-weight: var(--font-weight-regular, 400);
    color: var(--color-text-tertiary);
    letter-spacing: 0;
    text-transform: none;
  }

  .cmd-box {
    position: relative;
    background: var(--color-bg-inverse);
    color: var(--color-text-inverse);
    border-radius: var(--radius-md);
    padding: 14px 72px 14px 16px;
    font-family: var(--font-mono);
    font-size: var(--font-size-sm);
    line-height: 1.4;
    margin: 0 0 24px;
    overflow-x: auto;
    white-space: nowrap;
  }

  .cmd-box code {
    font-family: inherit;
    font-size: inherit;
    color: inherit;
  }

  .copy-btn {
    position: absolute;
    top: 8px;
    right: 8px;
    background: rgba(250, 250, 250, 0.1);
    color: var(--color-text-inverse);
    border: 1px solid rgba(250, 250, 250, 0.2);
    border-radius: 4px;
    padding: 4px 10px;
    font-family: var(--font-sans);
    font-size: 11px;
    font-weight: var(--font-weight-medium);
    cursor: pointer;
    transition: background-color 120ms ease;
  }

  .copy-btn:hover {
    background: rgba(250, 250, 250, 0.18);
  }

  .skip-note {
    margin: 0;
    text-align: center;
    font-size: var(--font-size-xs);
    color: var(--color-text-tertiary);
  }

  .inline-code {
    font-family: var(--font-mono);
    font-size: 11px;
    background: var(--color-bg-tertiary);
    padding: 1px 5px;
    border-radius: var(--radius-sm);
    color: var(--color-text-secondary);
  }

  /* ── Ephemeral note ───────────────────────────────────────────────────── */

  .ephemeral-note {
    max-width: 640px;
    margin: 24px auto 0;
    padding: 14px 18px;
    border-radius: var(--radius-md);
    background: var(--color-warning-muted);
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    display: flex;
    gap: 10px;
    align-items: flex-start;
    text-align: left;
  }

  .ephemeral-note strong {
    color: var(--color-warning);
    font-weight: var(--font-weight-semibold);
  }

  .ephemeral-icon {
    flex-shrink: 0;
  }

  /* ── Feature cards ────────────────────────────────────────────────────── */

  .what-is {
    max-width: 720px;
    margin: 48px auto 0;
    padding: 0 32px;
    display: grid;
    grid-template-columns: 1fr 1fr 1fr;
    gap: 20px;
  }

  @media (max-width: 600px) {
    .what-is {
      grid-template-columns: 1fr;
    }
  }

  .what-card {
    padding: 18px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-bg-secondary);
  }

  .what-card h3 {
    margin: 0 0 6px;
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-semibold);
    color: var(--color-text-primary);
  }

  .what-card p {
    margin: 0;
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    line-height: var(--line-height-base);
  }

  /* ── Footer ───────────────────────────────────────────────────────────── */

  .doc-foot {
    max-width: 720px;
    margin: 48px auto 0;
    padding: 24px 32px 60px;
    text-align: center;
    color: var(--color-text-tertiary);
    font-size: var(--font-size-sm);
  }

  .doc-foot a {
    color: var(--color-text-link);
    text-decoration: none;
  }

  .doc-foot a:hover {
    text-decoration: underline;
  }
</style>
