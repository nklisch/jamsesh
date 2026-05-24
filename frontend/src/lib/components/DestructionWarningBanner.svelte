<script lang="ts">
  // DestructionWarningBanner — renders when a playground session is approaching
  // its destruction deadline (idle or hard-cap).
  //
  // Priority rule: if both timers are within 5 minutes, the hard-cap warning
  // takes precedence (it's unresettable — no way to extend). Only one banner
  // is shown at a time.
  //
  // Props:
  //   idleRemainingMs     — ms until idle destruction (0 = already destroyed)
  //   hardCapRemainingMs  — ms until hard-cap destruction
  //   sessionId           — forwarded to the Finalize button nav
  //   orgId               — forwarded to the Finalize button nav
  //   onfinalize          — optional callback when Finalize is clicked
  //
  // Matches the .warning-banner visual from mockup 07-session-end.html
  // (substates 7a and 7b).

  import { navigate } from '$lib/router.svelte';

  const WARN_THRESHOLD_MS = 5 * 60 * 1000;

  let {
    idleRemainingMs,
    hardCapRemainingMs,
    sessionId,
    orgId,
    onfinalize,
  }: {
    idleRemainingMs: number;
    hardCapRemainingMs: number;
    sessionId: string;
    orgId: string;
    onfinalize?: () => void;
  } = $props();

  // Determine which banner to show, if any.
  // Priority: hard-cap beats idle when both are within threshold.
  type BannerKind = 'hardcap' | 'idle' | null;

  let bannerKind = $derived<BannerKind>(
    hardCapRemainingMs < WARN_THRESHOLD_MS
      ? 'hardcap'
      : idleRemainingMs < WARN_THRESHOLD_MS
        ? 'idle'
        : null,
  );

  function formatSeconds(ms: number): string {
    const totalSeconds = Math.max(0, Math.floor(ms / 1000));
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return `${minutes}:${String(seconds).padStart(2, '0')}`;
  }

  let timerDisplay = $derived(
    bannerKind === 'hardcap'
      ? formatSeconds(hardCapRemainingMs)
      : formatSeconds(idleRemainingMs),
  );

  function handleFinalize() {
    onfinalize?.();
    navigate(`/orgs/${orgId}/sessions/${sessionId}/finalize`);
  }
</script>

{#if bannerKind === 'idle'}
  <div
    class="warning-banner idle"
    role="alert"
    aria-live="assertive"
    data-testid="destruction-warning-idle"
  >
    <span class="icon" aria-hidden="true">⏱</span>
    <span class="text">
      <strong>Session ending in <span class="timer">{timerDisplay}</span> due to inactivity.</strong>
      Push a commit, send a comment, or click anywhere in the tree to reset the idle timer.
    </span>
    <div class="actions">
      <button class="warn-btn primary" onclick={handleFinalize}>Finalize now</button>
    </div>
  </div>
{:else if bannerKind === 'hardcap'}
  <div
    class="warning-banner hardcap"
    role="alert"
    aria-live="assertive"
    data-testid="destruction-warning-hardcap"
  >
    <span class="icon" aria-hidden="true">!</span>
    <span class="text">
      <strong>Session ending in <span class="timer">{timerDisplay}</span> — 24h wall-clock cap reached.</strong>
      Finalize locally now to keep your work, or it will be destroyed alongside the session.
      <code>jamsesh finalize --local</code> walks you through cherry-pick.
    </span>
    <div class="actions">
      <button class="warn-btn primary" onclick={handleFinalize}>Finalize now</button>
    </div>
  </div>
{/if}

<style>
  .warning-banner {
    padding: 12px 18px;
    display: flex;
    gap: 12px;
    align-items: center;
    border-bottom: 1px solid var(--color-border);
    font-size: var(--font-size-sm);
    flex-shrink: 0;
  }

  .warning-banner.idle {
    background: var(--color-warning-muted);
    border-left: 3px solid var(--color-warning);
  }

  .warning-banner.hardcap {
    background: var(--color-danger-muted);
    border-left: 3px solid var(--color-danger);
  }

  .icon {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 14px;
    flex-shrink: 0;
    color: white;
  }

  .warning-banner.idle .icon {
    background: var(--color-warning);
  }

  .warning-banner.hardcap .icon {
    background: var(--color-danger);
  }

  .text {
    flex: 1;
    color: var(--color-text-primary);
    line-height: 1.5;
  }

  .text strong {
    font-weight: var(--font-weight-semibold);
  }

  .text code {
    background: rgba(20, 22, 26, 0.08);
    padding: 1px 5px;
    border-radius: 3px;
    font: 11px var(--font-mono);
  }

  .timer {
    font: var(--font-weight-semibold) 13px var(--font-mono);
  }

  .warning-banner.idle .timer {
    color: var(--color-warning);
  }

  .warning-banner.hardcap .timer {
    color: var(--color-danger);
  }

  .actions {
    display: flex;
    gap: 6px;
    flex-shrink: 0;
  }

  .warn-btn {
    padding: 6px 12px;
    background: var(--color-bg-secondary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md);
    font: var(--font-weight-medium) var(--font-size-xs) var(--font-sans);
    cursor: pointer;
    white-space: nowrap;
  }

  .warn-btn.primary {
    background: var(--color-bg-inverse);
    color: var(--color-text-inverse);
    border-color: var(--color-bg-inverse);
  }

  .warn-btn:hover {
    opacity: 0.85;
  }
</style>
