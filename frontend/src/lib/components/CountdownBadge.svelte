<script lang="ts">
  // CountdownBadge — client-side countdown ticker for playground sessions.
  //
  // Displays two timers:
  //   • hard cap: time until the session's absolute wall-clock limit fires
  //   • idle: time until the session is destroyed for inactivity
  //
  // Props:
  //   hardCapAt               — absolute Date when the hard cap fires
  //   idleTimeoutAt           — absolute Date when idle destruction fires,
  //                             computed from lastSubstantiveActivityAt
  //   lastSubstantiveActivityAt — mutable: replaced on playground.activity_reset
  //
  // A 1-second setInterval drives a `now` $state rune; $derived runes compute
  // the two remaining-time values. On Page Visibility API visibilitychange →
  // visible, we recompute `now` from Date.now() to correct backgrounded-tab
  // throttle drift.
  //
  // The badge exposes `idleRemaining` and `hardCapRemaining` (ms) to the parent
  // so SessionViewShell can conditionally render DestructionWarningBanner without
  // duplicating the math.
  //
  // TODO: replace inline event type annotation with openapi-typescript generated
  // type once session-lifecycle feature lands EventEnvelope schema additions.

  import { onMount } from 'svelte';

  let {
    hardCapAt,
    idleTimeoutAt,
    lastSubstantiveActivityAt,
    onremainingupdate,
  }: {
    hardCapAt: Date;
    idleTimeoutAt: Date;
    lastSubstantiveActivityAt: Date;
    /** Called whenever the computed remaining values change (on each tick). */
    onremainingupdate?: (idleMs: number, hardCapMs: number) => void;
  } = $props();

  // `now` is the single reactive clock — updated by the interval and on
  // visibility restore.
  let now = $state(Date.now());

  // idleTimeoutAt is passed directly from the parent. When the parent receives
  // a playground.activity_reset WS event it replaces this prop, which triggers
  // the $derived chain below to recompute. We read it inside $derived so Svelte
  // tracks the reactive dependency correctly.
  let idleRemainingMs = $derived(Math.max(0, idleTimeoutAt.getTime() - now));
  let hardCapRemainingMs = $derived(Math.max(0, hardCapAt.getTime() - now));

  // Notify parent on each recompute.
  $effect(() => {
    onremainingupdate?.(idleRemainingMs, hardCapRemainingMs);
  });

  // Whether either timer is within the warning threshold (< 5 min).
  // Used to switch the badge to an "urgent" visual state.
  const WARN_THRESHOLD_MS = 5 * 60 * 1000;
  let isUrgent = $derived(
    idleRemainingMs < WARN_THRESHOLD_MS || hardCapRemainingMs < WARN_THRESHOLD_MS,
  );

  function formatMs(ms: number): string {
    if (ms <= 0) return '0s';
    const totalSeconds = Math.floor(ms / 1000);
    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const seconds = totalSeconds % 60;
    if (hours > 0) return `${hours}h ${minutes}m`;
    if (minutes > 0) return `${minutes}m ${seconds}s`;
    return `${seconds}s`;
  }

  let hardCapFormatted = $derived(formatMs(hardCapRemainingMs));
  let idleFormatted = $derived(formatMs(idleRemainingMs));

  onMount(() => {
    const interval = setInterval(() => {
      now = Date.now();
    }, 1000);

    // Page Visibility API: on returning from a backgrounded tab, snap `now`
    // to Date.now() to correct any throttle-accumulated drift before the next
    // interval tick.
    function handleVisibility() {
      if (document.visibilityState === 'visible') {
        now = Date.now();
      }
    }
    document.addEventListener('visibilitychange', handleVisibility);

    return () => {
      clearInterval(interval);
      document.removeEventListener('visibilitychange', handleVisibility);
    };
  });
</script>

<span
  class="countdown"
  class:urgent={isUrgent}
  title="Time until session destruction"
  data-testid="countdown-badge"
>
  <span class="label">ends in</span>
  <span class="hard" data-testid="hard-cap-remaining">{hardCapFormatted}</span>
  <span class="sep" aria-hidden="true">·</span>
  <span class="idle-label">idle</span>
  <span class="idle" data-testid="idle-remaining">{idleFormatted}</span>
</span>

<style>
  .countdown {
    display: inline-flex;
    align-items: baseline;
    gap: 5px;
    padding: 5px 10px;
    background: var(--color-bg-tertiary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    font: var(--font-size-xs) var(--font-mono);
    white-space: nowrap;
  }

  .countdown.urgent {
    background: var(--color-danger-muted);
    color: var(--color-danger);
    border-color: rgba(161, 74, 74, 0.25);
    font-weight: var(--font-weight-semibold);
  }

  .label,
  .idle-label {
    color: var(--color-text-tertiary);
  }

  .countdown.urgent .label,
  .countdown.urgent .idle-label {
    color: var(--color-danger);
    opacity: 0.75;
  }

  .hard {
    color: var(--color-text-primary);
    font-weight: var(--font-weight-semibold);
  }

  .sep {
    color: var(--color-text-tertiary);
  }

  .idle {
    color: var(--color-warning);
    font-weight: var(--font-weight-medium);
  }

  .countdown.urgent .hard,
  .countdown.urgent .idle {
    color: var(--color-danger);
  }

  .countdown.urgent .sep {
    color: var(--color-danger);
    opacity: 0.5;
  }
</style>
