// CountdownBadge.test.ts
// Tests the display-only CountdownBadge component.
//
// CountdownBadge accepts pre-computed idleRemainingMs and hardCapRemainingMs
// from the parent (SessionViewShell via createPlaygroundCountdown) and formats
// + renders them. The parent holds the clock and derives remaining times —
// CountdownBadge no longer has an onremainingupdate callback.

import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import CountdownBadge from './CountdownBadge.svelte';

// ── Helpers ───────────────────────────────────────────────────────────────────

const WARN_THRESHOLD_MS = 5 * 60 * 1000; // 5 min

function renderBadge(overrides: {
  idleRemainingMs?: number;
  hardCapRemainingMs?: number;
} = {}) {
  return render(CountdownBadge, {
    props: {
      idleRemainingMs: overrides.idleRemainingMs ?? 30 * 60 * 1000,    // 30m
      hardCapRemainingMs: overrides.hardCapRemainingMs ?? 60 * 60 * 1000, // 1h
    },
  });
}

describe('CountdownBadge', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders the hard-cap remaining time', () => {
    renderBadge({ hardCapRemainingMs: 60 * 60 * 1000 }); // 1h

    const hardCapEl = screen.getByTestId('hard-cap-remaining');
    expect(hardCapEl.textContent).toMatch(/1h/);
  });

  it('renders the idle remaining time', () => {
    renderBadge({ idleRemainingMs: 10 * 60 * 1000 }); // 10m

    const idleEl = screen.getByTestId('idle-remaining');
    expect(idleEl.textContent).toMatch(/10m/);
  });

  it('is not urgent when both timers are above 5 minutes', () => {
    renderBadge({
      idleRemainingMs: 30 * 60 * 1000,
      hardCapRemainingMs: 30 * 60 * 1000,
    });

    const badge = screen.getByTestId('countdown-badge');
    expect(badge).not.toHaveClass('urgent');
  });

  it('becomes urgent when idle remaining drops below 5 minutes', () => {
    renderBadge({
      idleRemainingMs: 4 * 60 * 1000,      // 4m — below threshold
      hardCapRemainingMs: 60 * 60 * 1000,  // 1h — above threshold
    });

    const badge = screen.getByTestId('countdown-badge');
    expect(badge).toHaveClass('urgent');
  });

  it('becomes urgent when hard-cap remaining drops below 5 minutes', () => {
    renderBadge({
      idleRemainingMs: 30 * 60 * 1000,    // 30m — above threshold
      hardCapRemainingMs: 4 * 60 * 1000,  // 4m — below threshold
    });

    const badge = screen.getByTestId('countdown-badge');
    expect(badge).toHaveClass('urgent');
  });

  it('updates display when props change (parent re-derives on each tick)', async () => {
    const { rerender } = renderBadge({ hardCapRemainingMs: 2 * 60 * 1000 }); // 2m

    const hardCapEl = screen.getByTestId('hard-cap-remaining');
    expect(hardCapEl.textContent).toMatch(/2m/);

    // Simulate the parent advancing its clock by 61 seconds.
    await rerender({
      idleRemainingMs: 30 * 60 * 1000,
      hardCapRemainingMs: (2 * 60 - 61) * 1000, // ~59s
    });

    await waitFor(() => {
      expect(hardCapEl.textContent).toMatch(/59s/);
    });
  });

  it('clamps display to 0s when a timer has expired', () => {
    renderBadge({ hardCapRemainingMs: 0 });

    const hardCapEl = screen.getByTestId('hard-cap-remaining');
    expect(hardCapEl.textContent).toBe('0s');
  });

  it('renders "ends in" and "idle" labels', () => {
    renderBadge();

    expect(screen.getByText('ends in')).toBeInTheDocument();
    expect(screen.getByText('idle')).toBeInTheDocument();
  });

  // ── Regression: per-tick child→parent write is gone (Unit 5) ────────────────

  it('badge has no onremainingupdate prop — remaining time is derived by the parent', () => {
    // CountdownBadge no longer accepts an onremainingupdate callback.
    // The parent (createPlaygroundCountdown) derives idleRemainingMs and
    // hardCapRemainingMs from its own clock and passes them as display props.
    // This test asserts the badge renders correctly with only the display props.
    renderBadge({ idleRemainingMs: 15 * 60 * 1000, hardCapRemainingMs: 45 * 60 * 1000 });

    expect(screen.getByTestId('idle-remaining').textContent).toMatch(/15m/);
    expect(screen.getByTestId('hard-cap-remaining').textContent).toMatch(/45m/);
    // No interval or callback in the badge — display updates come from re-renders.
  });

  it('urgent boundary: exactly at threshold is NOT urgent', () => {
    renderBadge({
      idleRemainingMs: WARN_THRESHOLD_MS,
      hardCapRemainingMs: WARN_THRESHOLD_MS,
    });

    // At exactly 5 min (not below), should not be urgent.
    const badge = screen.getByTestId('countdown-badge');
    expect(badge).not.toHaveClass('urgent');
  });

  it('urgent boundary: one ms below threshold IS urgent', () => {
    renderBadge({
      idleRemainingMs: WARN_THRESHOLD_MS - 1,
      hardCapRemainingMs: 60 * 60 * 1000,
    });

    const badge = screen.getByTestId('countdown-badge');
    expect(badge).toHaveClass('urgent');
  });
});
