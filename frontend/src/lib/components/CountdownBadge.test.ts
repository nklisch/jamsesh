import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import CountdownBadge from './CountdownBadge.svelte';

// ── Timer helpers ──────────────────────────────────────────────────────────────

function makeProps(overrides: {
  hardCapMs?: number;
  idleMs?: number;
  lastActivityMs?: number;
} = {}) {
  const now = Date.now();
  const hardCapMs = overrides.hardCapMs ?? 60 * 60 * 1000;      // 1h
  const idleMs = overrides.idleMs ?? 30 * 60 * 1000;            // 30m idle window
  const lastActivityMs = overrides.lastActivityMs ?? 0;          // 0 = no offset from "now"

  const lastSubstantiveActivityAt = new Date(now - lastActivityMs);
  return {
    hardCapAt: new Date(now + hardCapMs),
    idleTimeoutAt: new Date(lastSubstantiveActivityAt.getTime() + idleMs),
    lastSubstantiveActivityAt,
  };
}

describe('CountdownBadge', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('renders the hard-cap remaining time', () => {
    const props = makeProps({ hardCapMs: 60 * 60 * 1000 }); // 1h
    render(CountdownBadge, { props });

    const hardCapEl = screen.getByTestId('hard-cap-remaining');
    expect(hardCapEl.textContent).toMatch(/1h/);
  });

  it('renders the idle remaining time', () => {
    // 10 minutes idle window, no elapsed activity yet
    const props = makeProps({ idleMs: 10 * 60 * 1000, lastActivityMs: 0 });
    render(CountdownBadge, { props });

    const idleEl = screen.getByTestId('idle-remaining');
    expect(idleEl.textContent).toMatch(/10m/);
  });

  it('is not urgent when both timers are above 5 minutes', () => {
    const props = makeProps({ hardCapMs: 30 * 60 * 1000, idleMs: 30 * 60 * 1000 });
    render(CountdownBadge, { props });

    const badge = screen.getByTestId('countdown-badge');
    expect(badge).not.toHaveClass('urgent');
  });

  it('becomes urgent when idle remaining drops below 5 minutes', () => {
    // Idle window = 10 min; 6 min have elapsed → 4 min remaining
    const props = makeProps({
      hardCapMs: 60 * 60 * 1000,       // hard cap still far away
      idleMs: 10 * 60 * 1000,
      lastActivityMs: 6 * 60 * 1000,   // 6 min since last activity
    });
    render(CountdownBadge, { props });

    const badge = screen.getByTestId('countdown-badge');
    expect(badge).toHaveClass('urgent');
  });

  it('becomes urgent when hard-cap remaining drops below 5 minutes', () => {
    const props = makeProps({
      hardCapMs: 4 * 60 * 1000,        // 4 min until hard cap
      idleMs: 30 * 60 * 1000,          // idle still far
    });
    render(CountdownBadge, { props });

    const badge = screen.getByTestId('countdown-badge');
    expect(badge).toHaveClass('urgent');
  });

  it('advances the display after a second via setInterval', async () => {
    const props = makeProps({ hardCapMs: 2 * 60 * 1000 }); // 2 min
    render(CountdownBadge, { props });

    const hardCapEl = screen.getByTestId('hard-cap-remaining');
    const initialText = hardCapEl.textContent ?? '';

    vi.advanceTimersByTime(1000);
    await waitFor(() => {
      // After 1 second the display should have changed or at least be non-empty.
      expect(hardCapEl.textContent).toBeTruthy();
    });

    // Verify the text changed (countdown decreased).
    // With 2m initial, after 61s it should show "59s" not "2m 0s".
    vi.advanceTimersByTime(60_000);
    await waitFor(() => {
      expect(hardCapEl.textContent).not.toBe(initialText);
    });
  });

  it('calls onremainingupdate with current ms values', async () => {
    const onremainingupdate = vi.fn();
    const props = {
      ...makeProps({ hardCapMs: 60 * 60 * 1000, idleMs: 30 * 60 * 1000 }),
      onremainingupdate,
    };
    render(CountdownBadge, { props });

    // The effect fires on mount.
    await waitFor(() => {
      expect(onremainingupdate).toHaveBeenCalled();
    });

    const [idleMs, hardMs] = onremainingupdate.mock.calls[0] as [number, number];
    expect(idleMs).toBeGreaterThan(0);
    expect(hardMs).toBeGreaterThan(0);
    expect(hardMs).toBeGreaterThan(idleMs); // 1h > 30m
  });

  it('resets idle countdown when lastSubstantiveActivityAt prop changes', async () => {
    // Start with 6 minutes elapsed out of a 10-minute idle window → 4 min remaining (urgent)
    const now = Date.now();
    const idleWindowMs = 10 * 60 * 1000;
    const elapsedMs = 6 * 60 * 1000;
    const oldActivityAt = new Date(now - elapsedMs);

    const initialProps = {
      hardCapAt: new Date(now + 60 * 60 * 1000),
      idleTimeoutAt: new Date(oldActivityAt.getTime() + idleWindowMs),
      lastSubstantiveActivityAt: oldActivityAt,
    };

    const { rerender } = render(CountdownBadge, { props: initialProps });

    await waitFor(() => {
      const badge = screen.getByTestId('countdown-badge');
      expect(badge).toHaveClass('urgent'); // 4 min remaining → urgent
    });

    // Simulate playground.activity_reset: new activity just happened.
    const freshActivityAt = new Date(Date.now());
    await rerender({
      hardCapAt: initialProps.hardCapAt,
      idleTimeoutAt: new Date(freshActivityAt.getTime() + idleWindowMs),
      lastSubstantiveActivityAt: freshActivityAt,
    });

    await waitFor(() => {
      const badge = screen.getByTestId('countdown-badge');
      // With fresh activity the idle timer resets to full 10 min → not urgent.
      expect(badge).not.toHaveClass('urgent');
    });
  });

  it('clamps display to 0s when a timer has expired', () => {
    const props = makeProps({
      hardCapMs: -1000, // already expired
      idleMs: 30 * 60 * 1000,
    });
    render(CountdownBadge, { props });

    const hardCapEl = screen.getByTestId('hard-cap-remaining');
    expect(hardCapEl.textContent).toBe('0s');
  });

  it('renders "ends in" and "idle" labels', () => {
    render(CountdownBadge, { props: makeProps() });

    expect(screen.getByText('ends in')).toBeInTheDocument();
    expect(screen.getByText('idle')).toBeInTheDocument();
  });
});
