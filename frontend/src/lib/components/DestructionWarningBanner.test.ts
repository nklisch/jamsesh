import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import DestructionWarningBanner from './DestructionWarningBanner.svelte';

// ── Module mocks ──────────────────────────────────────────────────────────────

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  navigate: (...args: unknown[]) => mockNavigate(...args),
}));

// ── Constants ─────────────────────────────────────────────────────────────────

const SAFE_MS = 10 * 60 * 1000;          // 10 min — above 5-min warn threshold
const WARN_MS = 4 * 60 * 1000;           // 4 min — below 5-min warn threshold

function baseProps(overrides: Partial<{
  idleRemainingMs: number;
  hardCapRemainingMs: number;
}> = {}) {
  return {
    idleRemainingMs: SAFE_MS,
    hardCapRemainingMs: SAFE_MS,
    sessionId: 'sess-1',
    orgId: 'org_playground',
    ...overrides,
  };
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('DestructionWarningBanner', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders nothing when both timers are above the threshold', () => {
    const { container } = render(DestructionWarningBanner, {
      props: baseProps(),
    });
    expect(container.querySelector('.warning-banner')).toBeNull();
  });

  it('renders the idle warning when idle is below threshold and hard-cap is safe', () => {
    render(DestructionWarningBanner, {
      props: baseProps({ idleRemainingMs: WARN_MS }),
    });

    const banner = screen.getByTestId('destruction-warning-idle');
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveAttribute('role', 'alert');
    expect(banner.textContent).toMatch(/inactivity/i);
  });

  it('renders the hard-cap warning when hard-cap is below threshold and idle is safe', () => {
    render(DestructionWarningBanner, {
      props: baseProps({ hardCapRemainingMs: WARN_MS }),
    });

    const banner = screen.getByTestId('destruction-warning-hardcap');
    expect(banner).toBeInTheDocument();
    expect(banner.textContent).toMatch(/wall-clock cap/i);
  });

  it('renders only the hard-cap banner when both timers are in warning range', () => {
    render(DestructionWarningBanner, {
      props: baseProps({ idleRemainingMs: WARN_MS, hardCapRemainingMs: WARN_MS }),
    });

    // Hard-cap takes priority
    expect(screen.getByTestId('destruction-warning-hardcap')).toBeInTheDocument();
    expect(screen.queryByTestId('destruction-warning-idle')).toBeNull();
  });

  it('does not render more than one banner simultaneously', () => {
    render(DestructionWarningBanner, {
      props: baseProps({ idleRemainingMs: WARN_MS, hardCapRemainingMs: WARN_MS }),
    });

    const banners = document.querySelectorAll('.warning-banner');
    expect(banners).toHaveLength(1);
  });

  it('displays a formatted countdown timer in the idle banner', () => {
    // 4 min 47 sec = 287 seconds
    const idleRemainingMs = 287_000;
    render(DestructionWarningBanner, {
      props: baseProps({ idleRemainingMs }),
    });

    const timer = document.querySelector('.timer');
    expect(timer?.textContent).toBe('4:47');
  });

  it('displays a formatted countdown timer in the hard-cap banner', () => {
    const hardCapRemainingMs = 120_000; // 2 min 0 sec
    render(DestructionWarningBanner, {
      props: baseProps({ hardCapRemainingMs }),
    });

    const timer = document.querySelector('.timer');
    expect(timer?.textContent).toBe('2:00');
  });

  it('navigates to the finalize route when Finalize button is clicked (idle banner)', async () => {
    render(DestructionWarningBanner, {
      props: baseProps({ idleRemainingMs: WARN_MS }),
    });

    const btn = screen.getByRole('button', { name: /finalize now/i });
    await fireEvent.click(btn);

    expect(mockNavigate).toHaveBeenCalledWith(
      '/orgs/org_playground/sessions/sess-1/finalize',
    );
  });

  it('navigates to the finalize route when Finalize button is clicked (hard-cap banner)', async () => {
    render(DestructionWarningBanner, {
      props: baseProps({ hardCapRemainingMs: WARN_MS }),
    });

    const btn = screen.getByRole('button', { name: /finalize now/i });
    await fireEvent.click(btn);

    expect(mockNavigate).toHaveBeenCalledWith(
      '/orgs/org_playground/sessions/sess-1/finalize',
    );
  });

  it('calls onfinalize callback when provided', async () => {
    const onfinalize = vi.fn();
    render(DestructionWarningBanner, {
      props: { ...baseProps({ idleRemainingMs: WARN_MS }), onfinalize },
    });

    await fireEvent.click(screen.getByRole('button', { name: /finalize now/i }));
    expect(onfinalize).toHaveBeenCalledOnce();
  });

  it('idle banner has assertive aria-live for screen readers', () => {
    render(DestructionWarningBanner, {
      props: baseProps({ idleRemainingMs: WARN_MS }),
    });

    const banner = screen.getByTestId('destruction-warning-idle');
    expect(banner).toHaveAttribute('aria-live', 'assertive');
  });

  it('hard-cap banner has assertive aria-live for screen readers', () => {
    render(DestructionWarningBanner, {
      props: baseProps({ hardCapRemainingMs: WARN_MS }),
    });

    const banner = screen.getByTestId('destruction-warning-hardcap');
    expect(banner).toHaveAttribute('aria-live', 'assertive');
  });

  it('hard-cap banner mentions jamsesh finalize --local', () => {
    render(DestructionWarningBanner, {
      props: baseProps({ hardCapRemainingMs: WARN_MS }),
    });

    const banner = screen.getByTestId('destruction-warning-hardcap');
    expect(banner.textContent).toContain('jamsesh finalize --local');
  });

  it('transitions from no banner to idle banner when threshold is crossed', async () => {
    const { rerender } = render(DestructionWarningBanner, {
      props: baseProps({ idleRemainingMs: SAFE_MS }),
    });

    expect(document.querySelector('.warning-banner')).toBeNull();

    await rerender({ ...baseProps(), idleRemainingMs: WARN_MS, hardCapRemainingMs: SAFE_MS });
    expect(screen.getByTestId('destruction-warning-idle')).toBeInTheDocument();
  });

  it('transitions from idle banner to no banner when idle resets above threshold', async () => {
    const { rerender } = render(DestructionWarningBanner, {
      props: baseProps({ idleRemainingMs: WARN_MS }),
    });

    expect(screen.getByTestId('destruction-warning-idle')).toBeInTheDocument();

    await rerender({ ...baseProps(), idleRemainingMs: SAFE_MS, hardCapRemainingMs: SAFE_MS });
    expect(document.querySelector('.warning-banner')).toBeNull();
  });
});
