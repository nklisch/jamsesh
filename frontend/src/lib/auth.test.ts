// Tests for the auth rune store.
//
// Verifies access-token persistence to localStorage, memory-only refresh
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

  test('initialises token from localStorage and clears legacy refresh storage', async () => {
    localStorage.setItem('jamsesh.token', 'stored-access');
    localStorage.setItem('jamsesh.refresh', 'stored-refresh');

    const { auth } = await import('$lib/auth.svelte');

    expect(auth.token).toBe('stored-access');
    expect(auth.refresh).toBeNull();
    expect(auth.isAuthenticated).toBe(true);
    expect(localStorage.getItem('jamsesh.refresh')).toBeNull();
  });

  test('starts unauthenticated when localStorage is empty', async () => {
    const { auth } = await import('$lib/auth.svelte');

    expect(auth.token).toBeNull();
    expect(auth.refresh).toBeNull();
    expect(auth.isAuthenticated).toBe(false);
  });

  test('setTokens updates rune state and persists only access to localStorage', async () => {
    const { auth } = await import('$lib/auth.svelte');

    auth.setTokens('new-access', 'new-refresh');

    expect(auth.token).toBe('new-access');
    expect(auth.refresh).toBe('new-refresh');
    expect(auth.isAuthenticated).toBe(true);
    expect(localStorage.getItem('jamsesh.token')).toBe('new-access');
    expect(localStorage.getItem('jamsesh.refresh')).toBeNull();
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

    await auth.signOut();

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
    await auth.signOut();

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

  test('loadCurrentUser discards response when _token is null at completion', async () => {
    // Call loadCurrentUser WITHOUT setTokens — _token is null, so the guard
    // `_token !== null && _token === tokenAtStart` is false (null !== null is
    // false). The 200 response must not write state.
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify({ id: 'u', email: 'x', display_name: 'X', orgs: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    const { auth } = await import('$lib/auth.svelte');
    await auth.loadCurrentUser();
    expect(auth.currentUser).toBeNull();
    expect(auth.orgs).toBeNull();
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

    await auth.signOut();

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
      // signOut now fires a best-effort POST /api/auth/logout before
      // clearing local state (feature-auth-signout-backend-revoke-frontend).
      // We don't care about its response body; 204 is enough.
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
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
    await auth.signOut();

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

    // 3 fetches: user-A in-flight + signOut's POST /api/auth/logout + user-B load.
    expect(fetchSpy).toHaveBeenCalledTimes(3);
    expect(auth.currentUser).toEqual({
      id: 'user-b',
      email: 'userb@example.com',
      displayName: 'User B',
    });
  });

  // --- setAccessOnly ---

  test('setAccessOnly sets token and persists to localStorage', async () => {
    const { auth } = await import('$lib/auth.svelte');

    auth.setAccessOnly('access-only-token');

    expect(auth.token).toBe('access-only-token');
    expect(auth.isAuthenticated).toBe(true);
    expect(localStorage.getItem('jamsesh.token')).toBe('access-only-token');
  });

  test('setAccessOnly clears refresh and removes jamsesh.refresh from localStorage', async () => {
    const { auth } = await import('$lib/auth.svelte');

    // Prime the store with both tokens first.
    auth.setTokens('old-access', 'old-refresh');
    expect(auth.refresh).toBe('old-refresh');
    expect(localStorage.getItem('jamsesh.refresh')).toBeNull();

    auth.setAccessOnly('new-access-only');

    expect(auth.refresh).toBeNull();
    expect(localStorage.getItem('jamsesh.refresh')).toBeNull();
  });

  test('setAccessOnly clears cached currentUser and orgs', async () => {
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
    auth.setTokens('prior-access', 'prior-refresh');
    await auth.loadCurrentUser();

    expect(auth.currentUser).not.toBeNull();
    expect(auth.orgs).not.toBeNull();

    auth.setAccessOnly('resume-access-token');

    expect(auth.currentUser).toBeNull();
    expect(auth.orgs).toBeNull();
  });

  test('setAccessOnly resets _loadingMe so next loadCurrentUser fetches fresh', async () => {
    // After setAccessOnly, a subsequent loadCurrentUser() must NOT be a no-op
    // caused by a stale _loadingMe promise from a prior in-flight request.
    vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: 'old-user',
            email: 'old@example.com',
            display_name: 'Old User',
            orgs: [],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: 'new-user',
            email: 'new@example.com',
            display_name: 'New User',
            orgs: [],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      );

    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('old-access', 'old-refresh');
    await auth.loadCurrentUser();

    expect(auth.currentUser?.id).toBe('old-user');

    // Adopt a new access-only session — must clear the loaded cache.
    auth.setAccessOnly('resume-access-token');
    expect(auth.currentUser).toBeNull();

    // Next loadCurrentUser should fetch fresh data for the new account.
    await auth.loadCurrentUser();

    expect(auth.currentUser?.id).toBe('new-user');
  });

  test('setAccessOnly when no prior refresh leaves localStorage clean', async () => {
    const { auth } = await import('$lib/auth.svelte');

    // No refresh was ever set.
    expect(localStorage.getItem('jamsesh.refresh')).toBeNull();

    auth.setAccessOnly('access-only-fresh');

    // removeItem on an absent key is a no-op — key stays absent.
    expect(localStorage.getItem('jamsesh.refresh')).toBeNull();
    expect(auth.token).toBe('access-only-fresh');
    expect(auth.refresh).toBeNull();
  });

  // --- playgroundContext ---

  test('playgroundContext starts null', async () => {
    const { auth } = await import('$lib/auth.svelte');
    expect(auth.playgroundContext).toBeNull();
  });

  test('setPlaygroundContext populates playgroundContext', async () => {
    const { auth } = await import('$lib/auth.svelte');
    const expiresAt = new Date(Date.now() + 60_000).toISOString();

    auth.setPlaygroundContext({
      sessionId: 'sess-pg-1',
      bearer: 'anon-bearer-abc',
      nickname: 'swift-fox',
      expiresAt,
    });

    expect(auth.playgroundContext).toEqual({
      sessionId: 'sess-pg-1',
      bearer: 'anon-bearer-abc',
      nickname: 'swift-fox',
      expiresAt,
    });
    expect(localStorage.getItem('jamsesh.playground.sess-pg-1')).toContain('anon-bearer-abc');
  });

  test('setPlaygroundContext(null) clears the context', async () => {
    const { auth } = await import('$lib/auth.svelte');
    const expiresAt = new Date(Date.now() + 60_000).toISOString();

    auth.setPlaygroundContext({
      sessionId: 'sess-pg-2',
      bearer: 'anon-bearer-xyz',
      nickname: 'bold-hawk',
      expiresAt,
    });
    expect(auth.playgroundContext).not.toBeNull();

    auth.setPlaygroundContext(null);
    expect(auth.playgroundContext).toBeNull();
    expect(localStorage.getItem('jamsesh.playground.sess-pg-2')).toBeNull();
  });

  test('restorePlaygroundContext loads a live browser-scoped context', async () => {
    const { auth } = await import('$lib/auth.svelte');
    const expiresAt = new Date(Date.now() + 60_000).toISOString();
    localStorage.setItem('jamsesh.playground.sess-pg-live', JSON.stringify({
      sessionId: 'sess-pg-live',
      bearer: 'anon-bearer-live',
      nickname: 'live-nick',
      expiresAt,
    }));

    expect(auth.restorePlaygroundContext('sess-pg-live')).toBe(true);
    expect(auth.playgroundContext).toEqual({
      sessionId: 'sess-pg-live',
      bearer: 'anon-bearer-live',
      nickname: 'live-nick',
      expiresAt,
    });
  });

  test('restorePlaygroundContext rejects expired context and clears storage', async () => {
    const { auth } = await import('$lib/auth.svelte');
    localStorage.setItem('jamsesh.playground.sess-pg-expired', JSON.stringify({
      sessionId: 'sess-pg-expired',
      bearer: 'anon-bearer-expired',
      nickname: 'expired-nick',
      expiresAt: new Date(Date.now() - 60_000).toISOString(),
    }));

    expect(auth.restorePlaygroundContext('sess-pg-expired')).toBe(false);
    expect(auth.playgroundContext).toBeNull();
    expect(localStorage.getItem('jamsesh.playground.sess-pg-expired')).toBeNull();
  });

  test('setting playgroundContext does not affect isAuthenticated (orthogonal states)', async () => {
    const { auth } = await import('$lib/auth.svelte');
    const expiresAt = new Date(Date.now() + 60_000).toISOString();

    // Start unauthenticated.
    expect(auth.isAuthenticated).toBe(false);

    auth.setPlaygroundContext({
      sessionId: 'sess-pg-3',
      bearer: 'anon-bearer-456',
      nickname: 'calm-river',
      expiresAt,
    });

    // playgroundContext populated — isAuthenticated must remain false.
    expect(auth.playgroundContext).not.toBeNull();
    expect(auth.isAuthenticated).toBe(false);
  });

  test('isAuthenticated true and playgroundContext non-null can coexist', async () => {
    // A signed-in user clicking a playground share link: both states are set.
    const { auth } = await import('$lib/auth.svelte');
    const expiresAt = new Date(Date.now() + 60_000).toISOString();

    auth.setTokens('real-access-token', 'real-refresh-token');
    expect(auth.isAuthenticated).toBe(true);

    auth.setPlaygroundContext({
      sessionId: 'sess-pg-4',
      bearer: 'anon-bearer-789',
      nickname: 'quick-storm',
      expiresAt,
    });

    expect(auth.isAuthenticated).toBe(true);
    expect(auth.playgroundContext).not.toBeNull();
    expect(auth.playgroundContext?.sessionId).toBe('sess-pg-4');
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
    vi.spyOn(globalThis, 'fetch')
      .mockReturnValueOnce(fetchPromise as Promise<Response>)
      // signOut best-effort POST /api/auth/logout
      // (feature-auth-signout-backend-revoke-frontend).
      .mockResolvedValueOnce(new Response(null, { status: 204 }));

    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('user1-access', 'user1-refresh');

    // Start the load — it will be in-flight after this line.
    const loadPromise = auth.loadCurrentUser();
    expect(auth.orgs).toBeNull();
    expect(auth.currentUser).toBeNull();

    // signOut races the load.
    await auth.signOut();
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

  // --- best-effort backend logout (feature-auth-signout-backend-revoke-frontend) ---

  test('signOut calls POST /api/auth/logout before clearing local state', async () => {
    vi.doMock('$lib/router.svelte', () => ({
      navigate: vi.fn(),
      current: { name: 'sessions', params: {} },
    }));
    const fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(null, { status: 204 }));

    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('a-token', 'r-token');

    await auth.signOut();

    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const req = fetchSpy.mock.calls[0][0] as Request;
    expect(req.method).toBe('POST');
    expect(req.url).toMatch(/\/api\/auth\/logout$/);
    // Token cleared post-call.
    expect(auth.token).toBeNull();
  });

  test('signOut when unauthenticated does not call POST /api/auth/logout', async () => {
    vi.doMock('$lib/router.svelte', () => ({
      navigate: vi.fn(),
      current: { name: 'login', params: {} },
    }));
    const fetchSpy = vi.spyOn(globalThis, 'fetch');

    const { auth } = await import('$lib/auth.svelte');
    // No setTokens — _token is null.
    await auth.signOut();

    expect(fetchSpy).not.toHaveBeenCalled();
  });

  test('signOut clears local state even when POST /api/auth/logout throws', async () => {
    vi.doMock('$lib/router.svelte', () => ({
      navigate: vi.fn(),
      current: { name: 'sessions', params: {} },
    }));
    vi.spyOn(globalThis, 'fetch').mockRejectedValueOnce(new Error('network down'));

    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('a', 'r');

    await auth.signOut();

    // Local state cleared regardless of the rejected POST.
    expect(auth.token).toBeNull();
    expect(auth.refresh).toBeNull();
    expect(auth.isAuthenticated).toBe(false);
  });

  test('signOut clears local state even when POST /api/auth/logout returns 401', async () => {
    vi.doMock('$lib/router.svelte', () => ({
      navigate: vi.fn(),
      current: { name: 'sessions', params: {} },
    }));
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify({ error: 'auth.invalid_token', message: 'invalid' }), {
        status: 401,
        headers: { 'Content-Type': 'application/json' },
      }),
    );

    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('a', 'r');

    await auth.signOut();

    expect(auth.token).toBeNull();
    expect(auth.isAuthenticated).toBe(false);
  });
});
