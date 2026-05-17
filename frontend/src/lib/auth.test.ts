// Tests for the auth rune store.
//
// Verifies token/refresh persistence to localStorage, rune-derived
// state (isAuthenticated, token, refresh), and signOut clearing both
// state and storage then navigating to /login.

import { describe, test, expect, beforeEach, vi, afterEach } from 'vitest';

describe('auth store', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetModules();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  test('initialises token/refresh from localStorage', async () => {
    localStorage.setItem('jamsesh.token', 'stored-access');
    localStorage.setItem('jamsesh.refresh', 'stored-refresh');

    const { auth } = await import('$lib/auth.svelte');

    expect(auth.token).toBe('stored-access');
    expect(auth.refresh).toBe('stored-refresh');
    expect(auth.isAuthenticated).toBe(true);
  });

  test('starts unauthenticated when localStorage is empty', async () => {
    const { auth } = await import('$lib/auth.svelte');

    expect(auth.token).toBeNull();
    expect(auth.refresh).toBeNull();
    expect(auth.isAuthenticated).toBe(false);
  });

  test('setTokens updates rune state and persists to localStorage', async () => {
    const { auth } = await import('$lib/auth.svelte');

    auth.setTokens('new-access', 'new-refresh');

    expect(auth.token).toBe('new-access');
    expect(auth.refresh).toBe('new-refresh');
    expect(auth.isAuthenticated).toBe(true);
    expect(localStorage.getItem('jamsesh.token')).toBe('new-access');
    expect(localStorage.getItem('jamsesh.refresh')).toBe('new-refresh');
  });

  test('signOut clears rune state and localStorage', async () => {
    // Mock navigate so we don't trigger actual navigation.
    vi.doMock('$lib/router.svelte', () => ({
      navigate: vi.fn(),
      current: { name: 'login', params: {} },
    }));

    const { auth } = await import('$lib/auth.svelte');

    auth.setTokens('access', 'refresh');
    expect(auth.isAuthenticated).toBe(true);

    auth.signOut();

    expect(auth.token).toBeNull();
    expect(auth.refresh).toBeNull();
    expect(auth.isAuthenticated).toBe(false);
    expect(localStorage.getItem('jamsesh.token')).toBeNull();
    expect(localStorage.getItem('jamsesh.refresh')).toBeNull();
  });

  test('signOut navigates to /login', async () => {
    const mockNavigate = vi.fn();
    vi.doMock('$lib/router.svelte', () => ({
      navigate: mockNavigate,
      current: { name: 'sessions', params: {} },
    }));

    const { auth } = await import('$lib/auth.svelte');

    auth.setTokens('a', 'r');
    auth.signOut();

    expect(mockNavigate).toHaveBeenCalledWith('/login');
  });

  test('currentUser starts null', async () => {
    const { auth } = await import('$lib/auth.svelte');
    expect(auth.currentUser).toBeNull();
  });

  test('loadCurrentUser populates currentUser on success', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: 'user-123',
          email: 'ada@example.com',
          display_name: 'Ada Lovelace',
          orgs: [],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    );

    const { auth } = await import('$lib/auth.svelte');
    await auth.loadCurrentUser();

    expect(auth.currentUser).toEqual({
      id: 'user-123',
      email: 'ada@example.com',
      displayName: 'Ada Lovelace',
    });
  });

  test('loadCurrentUser resolves without throwing on network failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValueOnce(new Error('Network error'));

    const { auth } = await import('$lib/auth.svelte');
    await expect(auth.loadCurrentUser()).resolves.toBeUndefined();
    expect(auth.currentUser).toBeNull();
  });

  test('loadCurrentUser calls GET /api/me', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: 'user-456',
          email: 'charles@example.com',
          display_name: 'Charles Babbage',
          orgs: [],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    );

    const { auth } = await import('$lib/auth.svelte');
    await auth.loadCurrentUser();

    expect(fetchSpy).toHaveBeenCalledOnce();
    const calledUrl = (fetchSpy.mock.calls[0][0] as Request).url;
    expect(calledUrl).toContain('/api/me');
  });
});
