// OAuthCallback.test.ts
// Verifies: code+state read from query params, params cleared, POST to oauth
// callback endpoint, auth tokens stored on success, error state on failure.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/svelte';
import OAuthCallback from './OAuthCallback.svelte';

// ── Module mocks ────────────────────────────────────────────────────────────

const mockPOST = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    POST: (...args: unknown[]) => mockPOST(...args),
  },
}));

const mockSetTokens = vi.fn();
const mockLoadCurrentUser = vi.fn().mockResolvedValue(undefined);
vi.mock('$lib/auth.svelte', () => ({
  auth: {
    setTokens: (...args: unknown[]) => mockSetTokens(...args),
    loadCurrentUser: () => mockLoadCurrentUser(),
    isAuthenticated: false,
  },
}));

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  navigate: (...args: unknown[]) => mockNavigate(...args),
}));

// ── Location helpers ──────────────────────────────────────────────────────────

function setSearch(search: string) {
  Object.defineProperty(window, 'location', {
    value: { ...window.location, pathname: '/auth/oauth/callback', search, hash: '' },
    writable: true,
    configurable: true,
  });
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('OAuthCallback', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setSearch('?code=abc&state=xyz');
    sessionStorage.setItem('oauth.provider', 'github');
    sessionStorage.setItem('oauth.return_to', '/orgs/foo/sessions');
  });

  afterEach(() => {
    cleanup();
    sessionStorage.clear();
  });

  // ── Loading / exchanging state ─────────────────────────────────────────────

  it('renders exchanging state on mount before POST resolves', () => {
    mockPOST.mockReturnValue(new Promise(() => {}));
    render(OAuthCallback);
    expect(screen.getByText(/signing you in/i)).toBeInTheDocument();
  });

  // ── Happy path ─────────────────────────────────────────────────────────────

  it('calls POST with provider, code, and state from sessionStorage and query params', async () => {
    mockPOST.mockResolvedValue({ data: null, error: { error: 'oauth.invalid_state' } });
    render(OAuthCallback);

    await waitFor(() => expect(mockPOST).toHaveBeenCalledOnce());
    expect(mockPOST).toHaveBeenCalledWith(
      '/api/auth/oauth/callback',
      expect.objectContaining({ body: { provider: 'github', code: 'abc', state: 'xyz' } }),
    );
  });

  it('stores tokens and navigates to return_to from sessionStorage on success', async () => {
    mockPOST.mockResolvedValue({
      data: {
        access_token: 'access-tok',
        refresh_token: 'refresh-tok',
        access_expires_at: new Date().toISOString(),
        refresh_expires_at: new Date().toISOString(),
      },
      error: null,
    });
    render(OAuthCallback);

    await waitFor(() => expect(mockSetTokens).toHaveBeenCalledWith('access-tok', 'refresh-tok'));
    expect(mockNavigate).toHaveBeenCalledWith('/orgs/foo/sessions');
  });

  it('clears sessionStorage entries after reading them', async () => {
    mockPOST.mockResolvedValue({
      data: {
        access_token: 'access-tok',
        refresh_token: 'refresh-tok',
        access_expires_at: new Date().toISOString(),
        refresh_expires_at: new Date().toISOString(),
      },
      error: null,
    });
    render(OAuthCallback);

    await waitFor(() => expect(mockNavigate).toHaveBeenCalled());
    expect(sessionStorage.getItem('oauth.provider')).toBeNull();
    expect(sessionStorage.getItem('oauth.return_to')).toBeNull();
  });

  // ── Happy path fallback (no return_to) ────────────────────────────────────

  it('navigates to / when no oauth.return_to in sessionStorage', async () => {
    sessionStorage.removeItem('oauth.return_to');
    mockPOST.mockResolvedValue({
      data: {
        access_token: 'access-tok',
        refresh_token: 'refresh-tok',
        access_expires_at: new Date().toISOString(),
        refresh_expires_at: new Date().toISOString(),
      },
      error: null,
    });
    render(OAuthCallback);

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/'));
  });

  // ── loadCurrentUser is awaited before navigate ────────────────────────────

  it('awaits loadCurrentUser before navigating on successful exchange', async () => {
    mockPOST.mockResolvedValue({
      data: {
        access_token: 'access-tok',
        refresh_token: 'refresh-tok',
        access_expires_at: new Date().toISOString(),
        refresh_expires_at: new Date().toISOString(),
      },
      error: null,
    });
    render(OAuthCallback);

    await waitFor(() => expect(mockNavigate).toHaveBeenCalled());
    // loadCurrentUser must be called, and it must have been invoked before navigate.
    expect(mockLoadCurrentUser).toHaveBeenCalledOnce();
    expect(mockLoadCurrentUser.mock.invocationCallOrder[0]).toBeLessThan(
      mockNavigate.mock.invocationCallOrder[0],
    );
  });

  // ── Provider fallback ─────────────────────────────────────────────────────

  it('uses github as provider fallback when oauth.provider not in sessionStorage', async () => {
    sessionStorage.removeItem('oauth.provider');
    mockPOST.mockResolvedValue({ data: null, error: { error: 'oauth.invalid_state' } });
    render(OAuthCallback);

    await waitFor(() => expect(mockPOST).toHaveBeenCalledOnce());
    expect(mockPOST).toHaveBeenCalledWith(
      '/api/auth/oauth/callback',
      expect.objectContaining({ body: expect.objectContaining({ provider: 'github' }) }),
    );
  });

  // ── Missing params ─────────────────────────────────────────────────────────

  it('shows error state without POST when code is missing from query', async () => {
    setSearch('?state=xyz');
    render(OAuthCallback);

    await waitFor(() => {
      expect(screen.getByText(/this sign-in link is no longer valid/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/missing_params/)).toBeInTheDocument();
    expect(mockPOST).not.toHaveBeenCalled();
  });

  it('shows error state without POST when state is missing from query', async () => {
    setSearch('?code=abc');
    render(OAuthCallback);

    await waitFor(() => {
      expect(screen.getByText(/this sign-in link is no longer valid/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/missing_params/)).toBeInTheDocument();
    expect(mockPOST).not.toHaveBeenCalled();
  });

  // ── Backend error ─────────────────────────────────────────────────────────

  it('shows error state with backend error code when POST returns error', async () => {
    mockPOST.mockResolvedValue({
      data: null,
      error: { error: 'oauth.invalid_state' },
    });
    render(OAuthCallback);

    await waitFor(() => {
      expect(screen.getByText(/this sign-in link is no longer valid/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/oauth\.invalid_state/)).toBeInTheDocument();
  });

  it('shows generic error code when POST returns error without code', async () => {
    mockPOST.mockResolvedValue({ data: null, error: {} });
    render(OAuthCallback);

    await waitFor(() => {
      expect(screen.getByText(/exchange_failed/)).toBeInTheDocument();
    });
  });

  // ── Fetch throw ───────────────────────────────────────────────────────────

  it('shows exchange_failed when POST throws a network error', async () => {
    mockPOST.mockRejectedValue(new TypeError('Failed to fetch'));
    render(OAuthCallback);

    await waitFor(() => {
      expect(screen.getByText(/exchange_failed/)).toBeInTheDocument();
    });
  });

  // ── Open-redirect protection ──────────────────────────────────────────────

  it('rejects protocol-relative return_to and falls back to /', async () => {
    sessionStorage.setItem('oauth.return_to', '//evil.com');
    mockPOST.mockResolvedValue({
      data: {
        access_token: 'access-tok',
        refresh_token: 'refresh-tok',
        access_expires_at: new Date().toISOString(),
        refresh_expires_at: new Date().toISOString(),
      },
      error: null,
    });
    render(OAuthCallback);

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/'));
  });

  // ── URL clearing ──────────────────────────────────────────────────────────

  it('clears code+state from URL via history.replaceState after reading', async () => {
    const replaceStateSpy = vi.spyOn(history, 'replaceState');
    mockPOST.mockReturnValue(new Promise(() => {})); // keep in-flight
    render(OAuthCallback);

    await waitFor(() => expect(replaceStateSpy).toHaveBeenCalled());
    expect(replaceStateSpy).toHaveBeenCalledWith(null, '', '/auth/oauth/callback');
  });

  // ── Back to sign-in ────────────────────────────────────────────────────────

  it('renders a back-to-sign-in button in error state', async () => {
    setSearch('');
    render(OAuthCallback);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /back to sign in/i })).toBeInTheDocument();
    });
  });
});
