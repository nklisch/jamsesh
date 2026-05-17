<script lang="ts">
  // Per-session WebSocket reconnect indicator.
  //
  // Subscribes to the `wsStatus` rune store exported by `ws.svelte.ts`
  // and renders a `role="status"` text banner while the session's
  // socket is reconnecting. When the status is anything else (`'open'`,
  // `'connecting'`, or `null`), the banner is **absent** from the DOM —
  // not just `visibility: hidden` — so screen readers don't announce
  // an empty status region and so `getByRole('status')` queries in
  // tests aren't satisfied by a hidden node on every page load.
  //
  // Mounted once per session view (D5 in the feature design): the
  // session has exactly one live socket, so showing one banner per
  // WebSocket-consuming component would stack identical banners.
  import { wsStatus } from '$lib/ws.svelte';

  let { sessionId }: { sessionId: string } = $props();

  let status = $derived(wsStatus.for(sessionId));
</script>

{#if status === 'reconnecting'}
  <div class="ws-status" role="status" aria-live="polite">
    <span class="dot" aria-hidden="true"></span>
    <span class="label">Reconnecting…</span>
  </div>
{/if}

<style>
  .ws-status {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 20px;
    background: var(--color-warning-muted);
    color: var(--color-warning);
    border-bottom: 1px solid var(--color-border);
    flex-shrink: 0;
  }

  .dot {
    display: inline-block;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--color-warning);
    animation: ws-pulse 1.5s ease infinite;
    flex-shrink: 0;
    font-family: var(--font-mono);
  }

  .label {
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
  }

  @keyframes ws-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
</style>
