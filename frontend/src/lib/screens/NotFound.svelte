<script lang="ts">
  import { navigate } from '$lib/router.svelte';
</script>

<div class="not-found-wrap">
  <div class="not-found-card">
    <p class="code" aria-hidden="true">404</p>
    <h1>Page not found</h1>
    <p class="message">The page you're looking for doesn't exist.</p>
    <a
      href="/login"
      class="back-link"
      onclick={(e) => { e.preventDefault(); navigate('/login'); }}
    >
      Go to sign in
    </a>
  </div>
</div>

<style>
  .not-found-wrap {
    min-height: 100vh;
    display: grid;
    place-items: center;
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    padding: var(--space-6);
  }

  .not-found-card {
    text-align: center;
    max-width: 320px;
  }

  .code {
    font-family: var(--font-mono);
    font-size: 72px;
    font-weight: var(--font-weight-semibold);
    color: var(--color-text-tertiary);
    margin: 0 0 var(--space-2);
    line-height: 1;
  }

  h1 {
    margin: 0 0 var(--space-2);
    font-size: var(--font-size-xl);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.02em;
  }

  .message {
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    margin: 0 0 var(--space-5);
  }

  .back-link {
    color: var(--color-text-link);
    font-size: var(--font-size-sm);
    text-decoration: none;
    border-bottom: 1px dotted var(--color-text-link);
  }

  .back-link:hover {
    opacity: 0.8;
  }
</style>
