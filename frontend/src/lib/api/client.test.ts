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
});
