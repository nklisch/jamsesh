// Tests for the openapi-fetch typed client.
//
// Verifies that the Bearer middleware correctly attaches the
// Authorization header when auth.token is non-null, and omits it
// when auth.token is null.
//
// Strategy: vi.resetModules() between tests so each test gets a
// fresh module instance (fresh $state runes). Dynamic imports are
// used after resetModules() so the re-imported module is the new
// instance.

import { describe, test, expect, beforeEach, afterEach, vi } from 'vitest';

describe('client — Bearer middleware', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetModules();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  test('attaches Authorization: Bearer header when token is set', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-access-token', 'test-refresh-token');

    const { client } = await import('./client');

    let captured: Request | null = null;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      captured = input as Request;
      return new Response(
        JSON.stringify({
          access_token: 'a',
          refresh_token: 'b',
          access_expires_at: '',
          refresh_expires_at: '',
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });

    await client.POST('/api/auth/refresh', { body: { refresh_token: 'r' } });

    expect(captured).not.toBeNull();
    expect(captured!.headers.get('Authorization')).toBe('Bearer test-access-token');
  });

  test('omits Authorization header when token is null', async () => {
    // No tokens set — auth starts from empty localStorage.
    const { client } = await import('./client');

    let captured: Request | null = null;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      captured = input as Request;
      return new Response(
        JSON.stringify({
          access_token: 'a',
          refresh_token: 'b',
          access_expires_at: '',
          refresh_expires_at: '',
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });

    await client.POST('/api/auth/refresh', { body: { refresh_token: 'r' } });

    expect(captured).not.toBeNull();
    expect(captured!.headers.get('Authorization')).toBeNull();
  });

  test('updates the header when setTokens is called after client is created', async () => {
    const { auth } = await import('$lib/auth.svelte');
    const { client } = await import('./client');

    // First request — no token.
    let captured: Request | null = null;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      captured = input as Request;
      return new Response(
        JSON.stringify({ access_token: 'a', refresh_token: 'b', access_expires_at: '', refresh_expires_at: '' }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });

    await client.POST('/api/auth/refresh', { body: { refresh_token: 'r' } });
    expect(captured!.headers.get('Authorization')).toBeNull();

    // Set token, second request — should carry Bearer.
    auth.setTokens('after-set-token', 'rf');
    captured = null;

    await client.POST('/api/auth/refresh', { body: { refresh_token: 'r' } });
    expect(captured!.headers.get('Authorization')).toBe('Bearer after-set-token');
  });

  test('does not overwrite an explicit Authorization header', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('account-token', 'account-refresh');

    const { client } = await import('./client');

    let captured: Request | null = null;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      captured = input as Request;
      return new Response(JSON.stringify({ ticket: 't', expires_in_seconds: 60 }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });

    await client.POST('/api/auth/ws-ticket', {
      headers: { Authorization: 'Bearer explicit-token' },
    });

    expect(captured).not.toBeNull();
    expect(captured!.headers.get('Authorization')).toBe('Bearer explicit-token');
  });
});

describe('client — playground bearer selection', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetModules();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  function captureFetch(): { current: () => Request | null } {
    let captured: Request | null = null;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      captured = input as Request;
      return new Response(JSON.stringify({}), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });
    return { current: () => captured };
  }

  test('attaches the playground bearer for /api/playground/* requests', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setPlaygroundContext({
      sessionId: 's1',
      bearer: 'pg-bearer',
      nickname: 'n',
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    });

    const { client } = await import('./client');
    const cap = captureFetch();

    await client.GET('/api/playground/sessions/{id}', { params: { path: { id: 's1' } } });

    expect(cap.current()!.headers.get('Authorization')).toBe('Bearer pg-bearer');
  });

  test('attaches the playground bearer for /api/orgs/org_playground/* requests', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setPlaygroundContext({
      sessionId: 's1',
      bearer: 'pg-bearer',
      nickname: 'n',
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    });

    const { client } = await import('./client');
    const cap = captureFetch();

    await client.GET('/api/orgs/{orgID}/sessions/{sessionID}', {
      params: { path: { orgID: 'org_playground', sessionID: 's1' } },
    });

    expect(cap.current()!.headers.get('Authorization')).toBe('Bearer pg-bearer');
  });

  test('a signed-in account token does NOT shadow the playground bearer on playground requests', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('account-token', 'account-refresh');
    auth.setPlaygroundContext({
      sessionId: 's1',
      bearer: 'pg-bearer',
      nickname: 'n',
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    });

    const { client } = await import('./client');
    const cap = captureFetch();

    await client.GET('/api/playground/sessions/{id}', { params: { path: { id: 's1' } } });

    expect(cap.current()!.headers.get('Authorization')).toBe('Bearer pg-bearer');
  });

  test('non-playground requests still use the account token when a playground context exists', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('account-token', 'account-refresh');
    auth.setPlaygroundContext({
      sessionId: 's1',
      bearer: 'pg-bearer',
      nickname: 'n',
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    });

    const { client } = await import('./client');
    const cap = captureFetch();

    await client.GET('/api/me');

    expect(cap.current()!.headers.get('Authorization')).toBe('Bearer account-token');
  });

  test('org-scoped requests to a non-playground org use the account token, not the playground bearer', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('account-token', 'account-refresh');
    auth.setPlaygroundContext({
      sessionId: 's1',
      bearer: 'pg-bearer',
      nickname: 'n',
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    });

    const { client } = await import('./client');
    const cap = captureFetch();

    await client.GET('/api/orgs/{orgID}/sessions/{sessionID}', {
      params: { path: { orgID: 'org_real', sessionID: 's9' } },
    });

    expect(cap.current()!.headers.get('Authorization')).toBe('Bearer account-token');
  });
});

describe('client — 401 interceptor', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetModules();
    window.history.replaceState({}, '', '/orgs/x/sessions');
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  test('clears tokens and navigates to /login on 401 response', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('stale-token', 'stale-refresh');
    expect(auth.token).toBe('stale-token');

    const { client } = await import('./client');

    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      return new Response(
        JSON.stringify({ error: 'auth.invalid_token', message: 'token rejected' }),
        { status: 401, headers: { 'Content-Type': 'application/json' } },
      );
    });

    await client.GET('/api/me');

    expect(auth.token).toBeNull();
    expect(auth.refresh).toBeNull();
    expect(localStorage.getItem('jamsesh.token')).toBeNull();
    expect(localStorage.getItem('jamsesh.refresh')).toBeNull();
    expect(window.location.pathname).toBe('/login');
  });

  test('does NOT clear tokens on 200 response', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('good-token', 'good-refresh');

    const { client } = await import('./client');

    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      return new Response(
        JSON.stringify({ id: 'u1', email: 'a@b.test', display_name: 'A' }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });

    await client.GET('/api/me');

    expect(auth.token).toBe('good-token');
    expect(auth.refresh).toBe('good-refresh');
    expect(window.location.pathname).not.toBe('/login');
  });

  test('does NOT clear tokens on 500 response', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('good-token', 'good-refresh');

    const { client } = await import('./client');

    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      return new Response('internal error', { status: 500 });
    });

    await client.GET('/api/me');

    expect(auth.token).toBe('good-token');
    expect(auth.refresh).toBe('good-refresh');
    expect(window.location.pathname).not.toBe('/login');
  });

  test('multiple parallel 401s are idempotent', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('stale-token', 'stale-refresh');

    const { client } = await import('./client');

    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      return new Response(
        JSON.stringify({ error: 'auth.invalid_token', message: '' }),
        { status: 401, headers: { 'Content-Type': 'application/json' } },
      );
    });

    await Promise.all([
      client.GET('/api/me'),
      client.GET('/api/me'),
      client.GET('/api/me'),
    ]);

    expect(auth.token).toBeNull();
    expect(window.location.pathname).toBe('/login');
  });

  test('non-auth 401 (error prefix not "auth.") does NOT trigger signOut', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('valid-token', 'valid-refresh');

    const { client } = await import('./client');

    const signOutSpy = vi.spyOn(auth, 'signOut');

    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      return new Response(
        JSON.stringify({ error: 'org.scope_invalid', message: 'insufficient org scope' }),
        { status: 401, headers: { 'Content-Type': 'application/json' } },
      );
    });

    await client.GET('/api/me');

    expect(signOutSpy).not.toHaveBeenCalled();
    expect(auth.token).toBe('valid-token');
    expect(window.location.pathname).not.toBe('/login');
  });

  test('opaque 401 (non-JSON body) does NOT trigger signOut', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('valid-token', 'valid-refresh');

    const { client } = await import('./client');

    const signOutSpy = vi.spyOn(auth, 'signOut');

    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      return new Response('Unauthorized', {
        status: 401,
        headers: { 'Content-Type': 'text/plain' },
      });
    });

    await client.GET('/api/me');

    expect(signOutSpy).not.toHaveBeenCalled();
    expect(auth.token).toBe('valid-token');
    expect(window.location.pathname).not.toBe('/login');
  });

  test('auth.* subcode other than invalid_token triggers signOut', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('stale-token', 'stale-refresh');

    const { client } = await import('./client');

    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      return new Response(
        JSON.stringify({ error: 'auth.token_expired', message: 'token has expired' }),
        { status: 401, headers: { 'Content-Type': 'application/json' } },
      );
    });

    await client.GET('/api/me');

    expect(auth.token).toBeNull();
    expect(auth.refresh).toBeNull();
    expect(window.location.pathname).toBe('/login');
  });

  test('auth.* 401 on playground request clears playground context but not durable auth', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('account-token', 'account-refresh');
    auth.setPlaygroundContext({
      sessionId: 's1',
      bearer: 'pg-bearer',
      nickname: 'n',
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    });

    const { client } = await import('./client');

    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      return new Response(
        JSON.stringify({ error: 'auth.invalid_token', message: 'token rejected' }),
        { status: 401, headers: { 'Content-Type': 'application/json' } },
      );
    });

    await client.GET('/api/playground/sessions/{id}', { params: { path: { id: 's1' } } });

    expect(auth.token).toBe('account-token');
    expect(auth.refresh).toBe('account-refresh');
    expect(auth.playgroundContext).toBeNull();
    expect(localStorage.getItem('jamsesh.playground.s1')).toBeNull();
    expect(window.location.pathname).not.toBe('/login');
  });

  test('auth.* error on non-401 response (e.g. 403) does NOT trigger signOut', async () => {
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('valid-token', 'valid-refresh');

    const { client } = await import('./client');

    const signOutSpy = vi.spyOn(auth, 'signOut');

    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      return new Response(
        JSON.stringify({ error: 'auth.invalid_token', message: 'theoretical 403' }),
        { status: 403, headers: { 'Content-Type': 'application/json' } },
      );
    });

    await client.GET('/api/me');

    expect(signOutSpy).not.toHaveBeenCalled();
    expect(auth.token).toBe('valid-token');
    expect(window.location.pathname).not.toBe('/login');
  });
});
