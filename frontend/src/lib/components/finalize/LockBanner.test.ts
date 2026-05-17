import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import LockBanner from './LockBanner.svelte';

describe('LockBanner', () => {
  it('1. renders nothing when all inputs are null/false (idle state)', () => {
    const { container } = render(LockBanner, {
      props: {
        lockConflict: null,
        lockError: null,
        lock: null,
        isCaller: false,
        sessionEnded: false,
      },
    });
    expect(container.querySelector('.conflict-banner')).not.toBeInTheDocument();
    expect(container.querySelector('.error-banner')).not.toBeInTheDocument();
    expect(container.querySelector('.lock-pill')).not.toBeInTheDocument();
  });

  it('2. renders conflict banner with holder name when lockConflict is set', () => {
    render(LockBanner, {
      props: {
        lockConflict: { holderAccountId: 'alice@example.com' },
        lockError: null,
        lock: null,
        isCaller: false,
        sessionEnded: false,
      },
    });
    expect(screen.getByRole('alert', { name: /another member is finalizing/i })).toBeInTheDocument();
    expect(screen.getByText(/alice@example.com/)).toBeInTheDocument();
  });

  it('2b. falls back to "Another member" when holderAccountId is empty', () => {
    render(LockBanner, {
      props: {
        lockConflict: { holderAccountId: '' },
        lockError: null,
        lock: null,
        isCaller: false,
        sessionEnded: false,
      },
    });
    expect(screen.getByText(/Another member/)).toBeInTheDocument();
  });

  it('3. clicking Override in conflict state fires onOverride callback', async () => {
    const onOverride = vi.fn();
    render(LockBanner, {
      props: {
        lockConflict: { holderAccountId: 'user-2' },
        lockError: null,
        lock: null,
        isCaller: false,
        sessionEnded: false,
        onOverride,
      },
    });
    await fireEvent.click(screen.getByRole('button', { name: /override/i }));
    expect(onOverride).toHaveBeenCalledOnce();
  });

  it('3b. clicking Wait in conflict state fires onWait callback', async () => {
    const onWait = vi.fn();
    render(LockBanner, {
      props: {
        lockConflict: { holderAccountId: 'user-2' },
        lockError: null,
        lock: null,
        isCaller: false,
        sessionEnded: false,
        onWait,
      },
    });
    await fireEvent.click(screen.getByRole('button', { name: /wait/i }));
    expect(onWait).toHaveBeenCalledOnce();
  });

  it('4. renders error banner when lockError is set', () => {
    render(LockBanner, {
      props: {
        lockConflict: null,
        lockError: 'Failed to acquire lock.',
        lock: null,
        isCaller: false,
        sessionEnded: false,
      },
    });
    const alert = screen.getByRole('alert');
    expect(alert).toBeInTheDocument();
    expect(alert).toHaveTextContent(/Failed to acquire lock/);
  });

  it('5. clicking Dismiss fires onDismissError callback', async () => {
    const onDismissError = vi.fn();
    render(LockBanner, {
      props: {
        lockConflict: null,
        lockError: 'Some error',
        lock: null,
        isCaller: false,
        sessionEnded: false,
        onDismissError,
      },
    });
    await fireEvent.click(screen.getByRole('button', { name: /dismiss/i }));
    expect(onDismissError).toHaveBeenCalledOnce();
  });

  it('6. renders "You hold the lock" pill when user holds lock and is caller and session not ended', () => {
    render(LockBanner, {
      props: {
        lockConflict: null,
        lockError: null,
        lock: { lock_id: 'lock-abc', is_caller: true },
        isCaller: true,
        sessionEnded: false,
      },
    });
    expect(screen.getByLabelText(/you hold the lock/i)).toBeInTheDocument();
  });

  it('6b. does NOT render lock pill when sessionEnded is true', () => {
    render(LockBanner, {
      props: {
        lockConflict: null,
        lockError: null,
        lock: { lock_id: 'lock-abc', is_caller: true },
        isCaller: true,
        sessionEnded: true,
      },
    });
    expect(screen.queryByLabelText(/you hold the lock/i)).not.toBeInTheDocument();
  });

  it('6c. does NOT render lock pill when isCaller is false', () => {
    render(LockBanner, {
      props: {
        lockConflict: null,
        lockError: null,
        lock: { lock_id: 'lock-abc', is_caller: false },
        isCaller: false,
        sessionEnded: false,
      },
    });
    expect(screen.queryByLabelText(/you hold the lock/i)).not.toBeInTheDocument();
  });

  it('conflict banner and error banner can both appear simultaneously', () => {
    render(LockBanner, {
      props: {
        lockConflict: { holderAccountId: 'user-2' },
        lockError: 'Network error',
        lock: null,
        isCaller: false,
        sessionEnded: false,
      },
    });
    expect(screen.getByRole('alert', { name: /another member is finalizing/i })).toBeInTheDocument();
    expect(screen.getByText(/Network error/)).toBeInTheDocument();
  });
});
