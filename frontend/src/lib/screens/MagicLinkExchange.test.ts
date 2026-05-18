// MagicLinkExchange.test.ts
// Verifies: token read from hash, hash cleared, POST to exchange endpoint,
// auth tokens stored on success, error state on failure.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/svelte';
import MagicLinkExchange from './MagicLinkExchange.svelte';

// ── Module mocks ────────────────────────────────────────────────────────────

const mockPOST = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    POST: (...args: unknown[]) => mockPOST(...args),
  },
}));

const mockSetTokens = vi.fn();
vi.mock('$lib/auth.svelte', () => ({
  auth: {
    setTokens: (...args: unknown[]) => mockSetTokens(...args),
    isAuthenticated: false,
  },
}));

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  current: { name: 'magic-link', params: {} },
  navigate: (...args: unknown[]) => mockNavigate(...args),
}));

// ── Hash helpers ─────────────────────────────────────────────────────────────

function setHash(hash: string) {
  Object.defineProperty(window, 'location', {
    value: { ...window.location, hash, pathname: '/auth/magic-link', search: '' },
    writable: true,
    configurable: true,
  });
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('MagicLinkExchange', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setHash('#token=abc123def456');
  });

  afterEach(() => {
    cleanup();
  });

  // ── Loading / exchanging state ─────────────────────────────────────────────

  it('renders exchanging state on mount before POST resolves', () => {
    mockPOST.mockReturnValue(new Promise(() => {}));
    render(MagicLinkExchange);
    expect(screen.getByText(/signing you in/i)).toBeInTheDocument();
  });

  // ── Token read from hash ───────────────────────────────────────────────────

  it('calls POST with the token extracted from the hash', async () => {
    mockPOST.mockResolvedValue({ data: null, error: { error: 'auth.invalid_token' } });
    render(MagicLinkExchange);

    await waitFor(() => expect(mockPOST).toHaveBeenCalledOnce());
    expect(mockPOST).toHaveBeenCalledWith(
      '/api/auth/magic-link/exchange',
      expect.objectContaining({ body: { token: 'abc123def456' } }),
    );
  });

  // ── Hash clearing ──────────────────────────────────────────────────────────

  it('clears the hash via history.replaceState after reading the token', async () => {
    const replaceStateSpy = vi.spyOn(history, 'replaceState');
    mockPOST.mockReturnValue(new Promise(() => {})); // keep in-flight
    render(MagicLinkExchange);

    await waitFor(() => expect(replaceStateSpy).toHaveBeenCalled());
    expect(replaceStateSpy).toHaveBeenCalledWith(null, '', '/auth/magic-link');
  });

  // ── Missing token ──────────────────────────────────────────────────────────

  it('shows error state when hash contains no token', async () => {
    setHash('');
    render(MagicLinkExchange);

    await waitFor(() => {
      expect(screen.getByText(/this link is no longer valid/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/missing_token/)).toBeInTheDocument();
    // POST should NOT be called without a token
    expect(mockPOST).not.toHaveBeenCalled();
  });

  // ── Happy path ─────────────────────────────────────────────────────────────

  it('stores tokens and navigates to /login (fallback) on successful exchange', async () => {
    mockPOST.mockResolvedValue({
      data: {
        access_token: 'access-tok',
        refresh_token: 'refresh-tok',
        access_expires_at: new Date().toISOString(),
        refresh_expires_at: new Date().toISOString(),
      },
      error: null,
    });
    render(MagicLinkExchange);

    await waitFor(() => expect(mockSetTokens).toHaveBeenCalledWith('access-tok', 'refresh-tok'));
    expect(mockNavigate).toHaveBeenCalledWith('/login');
  });

  // ── Error states ───────────────────────────────────────────────────────────

  it('shows error state with code when POST returns 401 invalid token', async () => {
    mockPOST.mockResolvedValue({
      data: null,
      error: { error: 'auth.invalid_token', message: 'invalid or expired token' },
    });
    render(MagicLinkExchange);

    await waitFor(() => {
      expect(screen.getByText(/this link is no longer valid/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/auth\.invalid_token/)).toBeInTheDocument();
  });

  it('shows error state with code when POST returns 401 expired token', async () => {
    mockPOST.mockResolvedValue({
      data: null,
      error: { error: 'auth.expired_token', message: 'magic link has expired' },
    });
    render(MagicLinkExchange);

    await waitFor(() => {
      expect(screen.getByText(/auth\.expired_token/)).toBeInTheDocument();
    });
  });

  it('shows generic error code when POST returns error with no code', async () => {
    mockPOST.mockResolvedValue({ data: null, error: {} });
    render(MagicLinkExchange);

    await waitFor(() => {
      expect(screen.getByText(/exchange_failed/)).toBeInTheDocument();
    });
  });

  // ── Back to sign-in ────────────────────────────────────────────────────────

  it('renders a back-to-sign-in button in error state', async () => {
    setHash('');
    render(MagicLinkExchange);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /back to sign in/i })).toBeInTheDocument();
    });
  });
});
