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
    auth.setTokens('test-access', 'test-refresh');
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

  // --- orgs getter ---

  test('orgs starts null before loadCurrentUser', async () => {
    const { auth } = await import('$lib/auth.svelte');
    expect(auth.orgs).toBeNull();
  });

  test('loadCurrentUser populates orgs with empty array', async () => {
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
    auth.setTokens('test-access', 'test-refresh');
    await auth.loadCurrentUser();

    expect(auth.orgs).toEqual([]);
  });

  test('loadCurrentUser populates orgs with membership array', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: 'user-123',
          email: 'ada@example.com',
          display_name: 'Ada Lovelace',
          orgs: [
            { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
            { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
          ],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    );

    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-access', 'test-refresh');
    await auth.loadCurrentUser();

    expect(auth.orgs).toEqual([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
  });

  // --- idempotency ---

  test('loadCurrentUser is a no-op when already loaded', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
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
    auth.setTokens('test-access', 'test-refresh');
    await auth.loadCurrentUser();
    await auth.loadCurrentUser(); // second call — should not fetch again

    expect(fetchSpy).toHaveBeenCalledOnce();
  });

  test('concurrent loadCurrentUser calls result in exactly one fetch', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
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
    await Promise.all([auth.loadCurrentUser(), auth.loadCurrentUser()]);

    expect(fetchSpy.mock.calls.length).toBe(1);
  });

  // --- signOut clears orgs ---

  test('signOut clears orgs to null', async () => {
    vi.doMock('$lib/router.svelte', () => ({
      navigate: vi.fn(),
      current: { name: 'sessions', params: {} },
    }));
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: 'user-123',
          email: 'ada@example.com',
          display_name: 'Ada Lovelace',
          orgs: [{ id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' }],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    );

    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-access', 'test-refresh');
    await auth.loadCurrentUser();
    expect(auth.orgs).not.toBeNull();

    auth.signOut();

    expect(auth.orgs).toBeNull();
  });

  // --- addOrg ---

  test('addOrg initializes orgs array when currently null', async () => {
    const { auth } = await import('$lib/auth.svelte');
    expect(auth.orgs).toBeNull();

    auth.addOrg({ id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' });

    expect(auth.orgs).toEqual([{ id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' }]);
  });

  test('addOrg appends to existing orgs array via reassignment', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: 'user-123',
          email: 'ada@example.com',
          display_name: 'Ada Lovelace',
          orgs: [{ id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' }],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    );

    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-access', 'test-refresh');
    await auth.loadCurrentUser();

    const originalArray = auth.orgs;
    auth.addOrg({ id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' });

    // Array was reassigned (not mutated in-place)
    expect(auth.orgs).not.toBe(originalArray);
    expect(auth.orgs).toEqual([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
  });

  // --- _loadingMe reset on signOut ---

  test('signOut while a loadCurrentUser is in-flight allows a subsequent loadCurrentUser to fetch again', async () => {
    // Without _loadingMe = null in signOut(), the post-signOut call to
    // loadCurrentUser() would see _loadingMe still set to the in-flight
    // (but now abandoned) promise and return early as a no-op — silently
    // never fetching fresh data for the newly signed-in user.
    vi.doMock('$lib/router.svelte', () => ({
      navigate: vi.fn(),
      current: { name: 'sessions', params: {} },
    }));

    // A controllable fetch so we can resolve user A's response after signOut.
    let resolveUserA: (response: Response) => void;
    const fetchUserA = new Promise<Response>((resolve) => {
      resolveUserA = resolve;
    });
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
      .mockReturnValueOnce(fetchUserA as Promise<Response>)
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: 'user-b',
            email: 'userb@example.com',
            display_name: 'User B',
            orgs: [],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      );

    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('a', 'a');

    // Kick off the first load — it is in-flight and awaiting fetchUserA.
    const p1 = auth.loadCurrentUser();

    // signOut must reset _loadingMe to null so the next loadCurrentUser
    // doesn't just await the abandoned promise.
    auth.signOut();

    // Resolve the stale user A response now that the token has been cleared.
    // The token-at-start guard inside loadCurrentUser discards this data.
    resolveUserA!(
      new Response(
        JSON.stringify({
          id: 'user-a',
          email: 'usera@example.com',
          display_name: 'User A',
          orgs: [],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    );
    await p1;

    // State is clean after the stale response was discarded.
    expect(auth.currentUser).toBeNull();

    // Sign in as user B and load their profile — this MUST fire a second fetch.
    auth.setTokens('b', 'b');
    await auth.loadCurrentUser();

    expect(fetchSpy).toHaveBeenCalledTimes(2);
    expect(auth.currentUser).toEqual({
      id: 'user-b',
      email: 'userb@example.com',
      displayName: 'User B',
    });
  });

  // --- stale-write race guard ---

  test('discards stale /api/me response when signOut raced the in-flight call', async () => {
    // signOut calls navigate('/login') — mock it so the test doesn't error.
    vi.doMock('$lib/router.svelte', () => ({
      navigate: vi.fn(),
      current: { name: 'sessions', params: {} },
    }));

    // A controllable fetch we resolve manually after signOut runs.
    let resolveFetch: (response: Response) => void;
    const fetchPromise = new Promise<Response>((resolve) => {
      resolveFetch = resolve;
    });
    vi.spyOn(globalThis, 'fetch').mockReturnValueOnce(fetchPromise as Promise<Response>);

    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('user1-access', 'user1-refresh');

    // Start the load — it will be in-flight after this line.
    const loadPromise = auth.loadCurrentUser();
    expect(auth.orgs).toBeNull();
    expect(auth.currentUser).toBeNull();

    // signOut races the load.
    auth.signOut();
    expect(auth.token).toBeNull();
    expect(auth.orgs).toBeNull();

    // Now the in-flight /api/me response arrives, valid but stale.
    resolveFetch!(
      new Response(
        JSON.stringify({
          id: 'stale-user',
          email: 'stale@example.com',
          display_name: 'Stale User',
          orgs: [{ id: 'stale-org', name: 'stale', slug: 'stale', role: 'creator' }],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    );
    await loadPromise;

    // State must remain cleared — the stale response is discarded by
    // the token-at-start guard. Without the guard, this would leak the
    // previous user's identity onto whoever signs in next.
    expect(auth.currentUser).toBeNull();
    expect(auth.orgs).toBeNull();
  });
});
