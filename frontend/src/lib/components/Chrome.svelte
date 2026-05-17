<script lang="ts">
  import type { Snippet } from 'svelte';
  import ThemeToggle from './ThemeToggle.svelte';
  import AuthorDot from './AuthorDot.svelte';
  import { auth } from '$lib/auth.svelte';

  let {
    orgChip,
    sessionChip,
    children,
  }: {
    orgChip?: string;
    sessionChip?: string;
    children: Snippet;
  } = $props();
</script>

<div class="chrome">
  <header class="topbar">
    <div class="left">
      <span class="wordmark">jam<span class="dot">·</span>sesh</span>
      {#if orgChip}
        <nav class="breadcrumb" aria-label="breadcrumb">
          <span class="chip" aria-current={sessionChip ? undefined : 'page'}>{orgChip}</span>
          {#if sessionChip}
            <span class="sep" aria-hidden="true">/</span>
            <span class="chip here" aria-current="page">{sessionChip}</span>
          {/if}
        </nav>
      {/if}
    </div>
    <div class="right">
      <ThemeToggle />
      {#if auth.currentUser}
        <AuthorDot
          authorId={auth.currentUser.id}
          size={26}
          title={auth.currentUser.displayName}
        />
      {/if}
    </div>
  </header>
  <main class="body">
    {@render children()}
  </main>
</div>

<style>
  .chrome {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
  }

  .topbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 10px 20px;
    border-bottom: 1px solid var(--color-border);
    flex-shrink: 0;
  }

  .left,
  .right {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .wordmark {
    font-family: var(--font-sans);
    font-size: var(--font-size-base);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.03em;
    color: var(--color-text-primary);
    user-select: none;
  }

  .wordmark .dot {
    color: var(--color-accent);
  }

  .breadcrumb {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
  }

  .sep {
    color: var(--color-text-tertiary);
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 9px;
    border-radius: var(--radius-md);
    background: var(--color-bg-tertiary);
    color: var(--color-text-secondary);
    font-family: var(--font-mono);
    font-size: 11px;
    line-height: 1;
    border: 1px solid var(--color-border);
  }

  .here {
    color: var(--color-text-primary);
    font-weight: var(--font-weight-medium);
  }

  .body {
    flex: 1;
    padding: var(--space-6);
  }
</style>
