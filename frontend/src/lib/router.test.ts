// Tests for the History-API router.
// Vitest runs with jsdom (globals: true) so window and history are available.

import { describe, test, expect } from 'vitest';

// The router module uses module-level $state / $derived runes which means
// we need to import it as an ES module and work with the exported values.
// Because Vitest resets modules per test file but not per test, we manage
// window.location manually via history.pushState / popstate events.

describe('router — pattern matching', () => {
  // We test the matching logic by importing the module once.
  // Each navigate() call mutates the shared `path` state and updates `current`.

  test('matches / as home (first-match wins)', async () => {
    const { navigate, current } = await import('./router.svelte');
    navigate('/');
    expect(current.name).toBe('home');
    expect(current.params).toEqual({});
  });

  test('matches /login', async () => {
    const { navigate, current } = await import('./router.svelte');
    navigate('/login');
    expect(current.name).toBe('login');
    expect(current.params).toEqual({});
  });

  test('matches /orgs/:orgId/sessions', async () => {
    const { navigate, current } = await import('./router.svelte');
    navigate('/orgs/acme/sessions');
    expect(current.name).toBe('sessions');
    expect(current.params).toEqual({ orgId: 'acme' });
  });

  test('matches /orgs/:orgId/sessions/:sessionId', async () => {
    const { navigate, current } = await import('./router.svelte');
    navigate('/orgs/acme/sessions/sess-42');
    expect(current.name).toBe('session-view');
    expect(current.params).toEqual({ orgId: 'acme', sessionId: 'sess-42' });
  });

  test('matches /orgs/:orgId/sessions/:sessionId/finalize (more specific than session-view)', async () => {
    const { navigate, current } = await import('./router.svelte');
    navigate('/orgs/acme/sessions/sess-42/finalize');
    expect(current.name).toBe('finalize');
    expect(current.params).toEqual({ orgId: 'acme', sessionId: 'sess-42' });
  });

  test('decodes percent-encoded params', async () => {
    const { navigate, current } = await import('./router.svelte');
    navigate('/orgs/my%20org/sessions');
    expect(current.params.orgId).toBe('my org');
  });

  test('returns not-found for unknown path', async () => {
    const { navigate, current } = await import('./router.svelte');
    navigate('/does/not/exist');
    expect(current.name).toBe('not-found');
    expect(current.params).toEqual({});
  });
});

describe('router — navigate()', () => {
  test('navigate() updates current synchronously', async () => {
    const { navigate, current } = await import('./router.svelte');
    navigate('/login');
    expect(current.name).toBe('login');
    navigate('/orgs/x/sessions');
    expect(current.name).toBe('sessions');
  });

  test('navigate() pushes to history', async () => {
    const { navigate } = await import('./router.svelte');
    const before = window.history.length;
    navigate('/login');
    expect(window.history.length).toBeGreaterThanOrEqual(before);
    expect(window.location.pathname).toBe('/login');
  });
});

describe('router — popstate', () => {
  test('popstate event updates current', async () => {
    const { navigate, current } = await import('./router.svelte');

    // Navigate forward so there is history to go back to.
    navigate('/login');
    navigate('/orgs/acme/sessions');
    expect(current.name).toBe('sessions');

    // Simulate browser back by mutating location and firing popstate.
    window.history.back();
    // jsdom doesn't auto-fire popstate on history.back(); fire it manually
    // with the path we expect to land on.
    Object.defineProperty(window, 'location', {
      value: { ...window.location, pathname: '/login' },
      writable: true,
      configurable: true,
    });
    window.dispatchEvent(new PopStateEvent('popstate', {}));
    expect(current.name).toBe('login');
  });
});

describe('router — requiresAuth flag', () => {
  // Declarative auth flag: each route in the registry declares whether it
  // requires authentication. The auth gate in App.svelte reads this flag
  // rather than maintaining a separate name-based allowlist.

  test('public routes expose requiresAuth: false', async () => {
    const { navigate, current } = await import('./router.svelte');

    navigate('/login');
    expect(current.requiresAuth).toBe(false);

    navigate('/auth/magic-link');
    expect(current.requiresAuth).toBe(false);

    navigate('/auth/oauth/callback');
    expect(current.requiresAuth).toBe(false);

    navigate('/playground/s/sess-pg-1/resume');
    expect(current.requiresAuth).toBe(false);

    navigate('/orgs/acme/sessions/sess-1/resume');
    expect(current.requiresAuth).toBe(false);
  });

  test('resume routes are public and win over broader session routes', async () => {
    const { navigate, current } = await import('./router.svelte');

    navigate('/playground/s/sess-pg-1/resume');
    expect(current.name).toBe('playground-resume');
    expect(current.params).toEqual({ sessionId: 'sess-pg-1' });
    expect(current.requiresAuth).toBe(false);

    navigate('/orgs/acme/sessions/sess-1/resume');
    expect(current.name).toBe('session-resume');
    expect(current.params).toEqual({ orgId: 'acme', sessionId: 'sess-1' });
    expect(current.requiresAuth).toBe(false);
  });

  test('protected routes expose requiresAuth: true', async () => {
    const { navigate, current } = await import('./router.svelte');

    navigate('/');
    expect(current.requiresAuth).toBe(true);

    navigate('/orgs/acme/sessions');
    expect(current.requiresAuth).toBe(true);

    navigate('/orgs/acme/sessions/sess-1');
    expect(current.requiresAuth).toBe(true);

    navigate('/orgs/acme/sessions/sess-1/finalize');
    expect(current.requiresAuth).toBe(true);

    navigate('/orgs/acme/sessions/sess-1/invites/inv-1/accept');
    expect(current.requiresAuth).toBe(true);

    navigate('/orgs/acme/settings');
    expect(current.requiresAuth).toBe(true);
  });

  test('not-found routes default to requiresAuth: true (unknown surfaces are protected)', async () => {
    const { navigate, current } = await import('./router.svelte');
    navigate('/does/not/exist');
    expect(current.requiresAuth).toBe(true);
  });
});
