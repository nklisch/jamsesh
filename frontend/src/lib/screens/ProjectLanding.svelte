<script lang="ts">
  import { navigate } from '$lib/router.svelte';
</script>

<!-- Top bar -->
<header class="topbar">
  <div class="grid">
    <a
      href="/"
      class="wordmark"
      onclick={(e) => { e.preventDefault(); navigate('/'); }}
    >jamsesh<span class="dot">.</span></a>
    <nav class="nav">
      <a
        href="/"
        class="current"
        onclick={(e) => { e.preventDefault(); navigate('/'); }}
      >Home</a>
      <a href="#docs">Docs</a>
      <a href="https://github.com/nklisch/jamsesh/blob/main/docs/SELF_HOST.md" target="_blank" rel="noopener noreferrer">Self-host</a>
      <a
        href="/playground"
        onclick={(e) => { e.preventDefault(); navigate('/playground'); }}
      >Playground</a>
    </nav>
    <div class="end">
      <a
        href="https://github.com/nklisch/jamsesh"
        class="icon-link"
        aria-label="GitHub repository"
        title="GitHub"
        target="_blank"
        rel="noopener noreferrer"
      >
        <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
          <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.56v-2.18c-3.2.7-3.87-1.37-3.87-1.37-.52-1.32-1.28-1.68-1.28-1.68-1.05-.71.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.76 2.7 1.25 3.36.96.1-.74.4-1.25.73-1.54-2.55-.29-5.24-1.27-5.24-5.66 0-1.25.45-2.27 1.18-3.07-.12-.29-.51-1.46.11-3.04 0 0 .96-.31 3.15 1.17a10.9 10.9 0 0 1 5.74 0c2.19-1.48 3.15-1.17 3.15-1.17.62 1.58.23 2.75.11 3.04.74.8 1.18 1.82 1.18 3.07 0 4.4-2.7 5.37-5.27 5.65.41.36.78 1.07.78 2.16v3.21c0 .31.21.68.79.56C20.71 21.39 24 17.08 24 12 24 5.65 18.85.5 12.5.5h-.5z"/>
        </svg>
      </a>
      <a
        href="/login"
        class="signin"
        onclick={(e) => { e.preventDefault(); navigate('/login'); }}
      >Sign in →</a>
    </div>
  </div>
</header>

<!-- Section 01 — Hero -->
<section class="hero">
  <div class="grid">
    <div class="num"><strong>01</strong> &nbsp; HERO</div>
    <div class="eyebrow">For teams jamming on code, specs, docs</div>
    <h1>Your team on a call.<br>Your agents <span class="accent">in the loop.</span></h1>
    <p class="lede">
      Each person drives their own Claude Code agent. Hash out a spec out loud, push code references
      back and forth, write docs together — every agent in the session sees the whole picture as it unfolds.
    </p>
    <div class="cta-line">
      <a
        href="/playground"
        onclick={(e) => { e.preventDefault(); navigate('/playground'); }}
      >Try the playground →</a>
    </div>
    <div class="cta-meta">
      Throwaway session · no account · ends in 24h or after 30 min idle
    </div>
  </div>
</section>

<!-- Section 02 — Schematic -->
<section class="schematic">
  <div class="grid">
    <div class="num"><strong>02</strong> &nbsp; SCHEMATIC</div>
    <div class="label">How the loop closes</div>
    <div class="body-text">
      <p>Three humans, three agents, one git-backed session. Each human speaks; their agent commits; the auto-merger weaves non-conflicting work into a shared draft as it lands.</p>
      <p>Comments addressed to <code>@quiet-fox</code> or <code>@all-agents</code> arrive in their next turn's digest — humans and agents share one conversation.</p>
    </div>
    <div class="diagram" aria-label="Example session activity">
      <div class="line"><span class="who">amber</span><span class="arrow">→ commit</span><span>refines auth flow</span></div>
      <div class="line"><span class="who">teal-fox</span><span class="arrow">→ commit</span><span>adds magic-link spec</span></div>
      <div class="line"><span class="who">auto</span><span class="arrow">→ merge</span><span>amber + teal-fox into draft</span></div>
      <div class="line"><span class="who">slate</span><span class="arrow">→ comment</span><span>@all-agents: scope is just /auth/*</span></div>
      <div class="line"><span class="who">amber</span><span class="arrow">→ next turn</span><span>narrows changes to /auth/</span></div>
      <div class="line"><span class="who">auto</span><span class="arrow">→ merge</span><span>clean — draft @ 0xa3f1c</span></div>
    </div>
  </div>
</section>

<!-- Section 03 — Method -->
<section class="method">
  <div class="grid">
    <div class="num"><strong>03</strong> &nbsp; METHOD</div>
    <div class="label">What you sign up for</div>
    <div class="method-list">
      <div class="step">
        <div class="step-num">A · VOICE-PACED</div>
        <h3>Talk through direction.</h3>
        <p>Your agent catches up from peers' commits and addressed comments before its next turn. No one has to recap state for anyone else's agent.</p>
      </div>
      <div class="step">
        <div class="step-num">B · EVERYONE IN THE LOOP</div>
        <h3>Comments cross the boundary.</h3>
        <p>Address <code>@anyone</code> in a comment; it lands in their next turn. Humans and agents share one conversation, not two.</p>
      </div>
      <div class="step">
        <div class="step-num">C · REAL GIT UNDERNEATH</div>
        <h3>Nothing parallel to learn.</h3>
        <p>Every push lands in a real bare repo. Real branches, real diffs, real <code>git fetch</code>. Recovery is your existing toolchain.</p>
      </div>
    </div>
  </div>
</section>

<!-- Colophon -->
<section class="colophon-sec">
  <div class="grid">
    <!-- __APP_VERSION__ is replaced at build time by Vite's `define` block
         from frontend/package.json. See vite.config.ts.
         (gate-tests-projectlanding-hardcoded-version-string) -->
    <div class="colophon-meta">jamsesh / Apache-2.0 / {__APP_VERSION__} / 2026</div>
    <div class="links">
      <a href="https://github.com/nklisch/jamsesh" target="_blank" rel="noopener noreferrer">GitHub</a>
      <a href="#docs">Docs</a>
      <a href="https://github.com/nklisch/jamsesh/blob/main/docs/SELF_HOST.md" target="_blank" rel="noopener noreferrer">Self-host</a>
      <a
        href="/login"
        onclick={(e) => { e.preventDefault(); navigate('/login'); }}
      >Sign in</a>
    </div>
  </div>
</section>

<style>
  /* 12-col Swiss grid */
  .grid {
    max-width: 1280px;
    margin: 0 auto;
    padding: 0 32px;
    display: grid;
    grid-template-columns: repeat(12, 1fr);
    gap: 16px;
  }

  /* Top bar */
  .topbar {
    border-bottom: 1px solid var(--color-text-primary);
    padding: 16px 0;
  }
  .topbar .grid { align-items: baseline; }
  .topbar .wordmark {
    grid-column: 1 / span 3;
    font: 600 16px/1 var(--font-sans);
    letter-spacing: -0.03em;
    text-decoration: none;
    color: inherit;
  }
  .topbar .wordmark .dot { color: var(--color-accent); }
  .topbar .nav {
    grid-column: 5 / span 4;
    display: flex;
    gap: 28px;
    align-items: baseline;
    font: 400 13px/1 var(--font-sans);
  }
  .topbar .nav a {
    color: var(--color-text-secondary);
    text-decoration: none;
    letter-spacing: -0.005em;
  }
  .topbar .nav a:hover { color: var(--color-accent); }
  .topbar .nav a.current {
    color: var(--color-text-primary);
    border-bottom: 2px solid var(--color-accent);
    padding-bottom: 2px;
  }
  .topbar .end {
    grid-column: 10 / span 3;
    display: flex;
    gap: 18px;
    align-items: center;
    justify-content: flex-end;
  }
  .topbar .icon-link {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    color: var(--color-text-secondary);
    text-decoration: none;
    transition: color 0.1s ease;
  }
  .topbar .icon-link:hover { color: var(--color-accent); }
  .topbar .icon-link svg { width: 18px; height: 18px; display: block; }
  .topbar .signin {
    font: 400 13px/1 var(--font-sans);
    color: var(--color-text-secondary);
    text-decoration: none;
  }
  .topbar .signin:hover { color: var(--color-accent); }

  /* Hero — section 01 */
  .hero {
    padding: 96px 0 80px;
    border-bottom: 1px solid var(--color-text-primary);
  }
  .hero .num {
    grid-column: 1 / span 2;
    font: 500 11px/1 var(--font-mono);
    letter-spacing: 0.08em;
    color: var(--color-text-tertiary);
  }
  .hero .num strong { color: var(--color-accent); font-weight: 500; }
  .hero .eyebrow {
    grid-column: 4 / span 5;
    font: 500 11px/1 var(--font-mono);
    letter-spacing: 0.12em;
    color: var(--color-text-secondary);
    text-transform: uppercase;
    align-self: end;
  }
  .hero h1 {
    grid-column: 1 / -1;
    margin: 32px 0 0;
    font: 600 88px/0.95 var(--font-sans);
    letter-spacing: -0.04em;
  }
  .hero h1 .accent { color: var(--color-accent); }
  .hero .lede {
    grid-column: 4 / span 6;
    margin: 40px 0 0;
    font: 400 var(--font-size-lg)/1.55 var(--font-sans);
    color: var(--color-text-secondary);
  }
  .hero .cta-line {
    grid-column: 1 / span 3;
    margin: 40px 0 0;
    align-self: end;
  }
  .hero .cta-line a {
    display: inline-flex;
    align-items: baseline;
    gap: 8px;
    color: var(--color-text-primary);
    font: 500 var(--font-size-base)/1 var(--font-sans);
    text-decoration: none;
    border-bottom: 2px solid var(--color-accent);
    padding-bottom: 4px;
  }
  .hero .cta-line a:hover { color: var(--color-accent); }
  .hero .cta-meta {
    grid-column: 5 / span 4;
    margin: 40px 0 0;
    align-self: end;
    font: 400 11px/1.5 var(--font-mono);
    color: var(--color-text-tertiary);
    letter-spacing: 0.04em;
  }

  /* Schematic — section 02 */
  .schematic {
    padding: 80px 0;
    border-bottom: 1px solid var(--color-text-primary);
  }
  .schematic .num {
    grid-column: 1 / span 2;
    font: 500 11px/1 var(--font-mono);
    letter-spacing: 0.08em;
    color: var(--color-text-tertiary);
  }
  .schematic .num strong { color: var(--color-accent); font-weight: 500; }
  .schematic .label {
    grid-column: 4 / span 5;
    font: 500 11px/1 var(--font-mono);
    letter-spacing: 0.12em;
    color: var(--color-text-secondary);
    text-transform: uppercase;
  }
  .schematic .body-text {
    grid-column: 1 / span 5;
    margin: 32px 0 0;
    font: 400 var(--font-size-base)/1.55 var(--font-sans);
    color: var(--color-text-primary);
  }
  .schematic .body-text p { margin: 0 0 16px; }
  .schematic .body-text p:last-child { margin-bottom: 0; }
  .schematic .body-text code {
    font-family: var(--font-mono);
    font-size: 13px;
    color: var(--color-accent);
  }
  .schematic .diagram {
    grid-column: 7 / span 6;
    margin: 32px 0 0;
    padding: 32px;
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border-strong);
    font: 400 13px/1.6 var(--font-mono);
    color: var(--color-text-secondary);
  }
  .schematic .diagram .line {
    display: flex;
    gap: 12px;
    align-items: baseline;
    padding: 2px 0;
  }
  .schematic .diagram .line .who {
    color: var(--color-accent);
    min-width: 80px;
  }
  .schematic .diagram .line .arrow { color: var(--color-text-tertiary); }

  /* Method — section 03 */
  .method {
    padding: 80px 0;
    border-bottom: 1px solid var(--color-text-primary);
  }
  .method .num {
    grid-column: 1 / span 2;
    font: 500 11px/1 var(--font-mono);
    letter-spacing: 0.08em;
    color: var(--color-text-tertiary);
  }
  .method .num strong { color: var(--color-accent); font-weight: 500; }
  .method .label {
    grid-column: 4 / span 5;
    font: 500 11px/1 var(--font-mono);
    letter-spacing: 0.12em;
    color: var(--color-text-secondary);
    text-transform: uppercase;
  }
  .method-list {
    grid-column: 4 / span 9;
    margin: 32px 0 0;
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 32px;
  }
  .method-list .step h3 {
    font: 500 var(--font-size-base) var(--font-sans);
    margin: 0 0 8px;
  }
  .method-list .step .step-num {
    font: 500 11px/1 var(--font-mono);
    letter-spacing: 0.08em;
    color: var(--color-accent);
    margin-bottom: 12px;
  }
  .method-list .step p {
    margin: 0;
    font: 400 14px/1.55 var(--font-sans);
    color: var(--color-text-secondary);
  }
  .method-list .step p code {
    font-family: var(--font-mono);
    font-size: 12px;
  }

  /* Colophon */
  .colophon-sec { padding: 32px 0 64px; }
  .colophon-sec .colophon-meta {
    grid-column: 1 / span 4;
    font: 400 11px/1.5 var(--font-mono);
    color: var(--color-text-tertiary);
    letter-spacing: 0.04em;
  }
  .colophon-sec .links {
    grid-column: 9 / span 4;
    text-align: right;
    display: flex;
    gap: 20px;
    justify-content: flex-end;
  }
  .colophon-sec .links a {
    font: 400 12px/1 var(--font-sans);
    color: var(--color-text-secondary);
    text-decoration: none;
  }
  .colophon-sec .links a:hover { color: var(--color-accent); }
</style>
