<script lang="ts">
  // CountdownBadge — display-only countdown badge for playground sessions.
  //
  // Accepts pre-computed remaining-time values in milliseconds from the parent
  // (SessionViewShell via createPlaygroundCountdown). The parent holds the
  // clock and derives remaining times; this component only formats and renders.
  //
  // Props:
  //   idleRemainingMs    — milliseconds until idle destruction
  //   hardCapRemainingMs — milliseconds until hard-cap destruction

  let {
    idleRemainingMs,
    hardCapRemainingMs,
  }: {
    idleRemainingMs: number;
    hardCapRemainingMs: number;
  } = $props();

  // Whether either timer is within the warning threshold (< 5 min).
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
