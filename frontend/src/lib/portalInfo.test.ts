// Tests for the portalInfo rune store.
// Verifies: successful fetch sets values and marks loaded; failed fetch falls
// back to login-safe defaults and marks loaded; second init() is a no-op.

import { describe, test, expect, beforeEach, vi, afterEach } from 'vitest';

// ── Client mock ──────────────────────────────────────────────────────────────
// Must be declared before vi.mock (hoisted) so the factory can close over it.

const mockGET = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    GET: (...args: unknown[]) => mockGET(...args),
  },
}));

// portalInfo imports client at module load time, so we must import it AFTER
// vi.mock is set up. Use dynamic import + vi.resetModules() to get a fresh
// module per test (the store's private $state vars persist across tests
// otherwise).

describe('portalInfo store', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  test('successful fetch: sets playgroundEnabled, landingVariant, and loaded=true', async () => {
    mockGET.mockResolvedValueOnce({
      data: { playground_enabled: true, landing_variant: 'project' },
      error: undefined,
    });

    const { portalInfo } = await import('$lib/portalInfo.svelte');

    expect(portalInfo.loaded).toBe(false);

    await portalInfo.init();

    expect(portalInfo.loaded).toBe(true);
    expect(portalInfo.playgroundEnabled).toBe(true);
    expect(portalInfo.landingVariant).toBe('project');
    expect(mockGET).toHaveBeenCalledTimes(1);
    expect(mockGET).toHaveBeenCalledWith('/api/portal/info');
  });

  test('successful fetch with auto variant and playground disabled', async () => {
    mockGET.mockResolvedValueOnce({
      data: { playground_enabled: false, landing_variant: 'auto' },
      error: undefined,
    });

    const { portalInfo } = await import('$lib/portalInfo.svelte');

    await portalInfo.init();

    expect(portalInfo.loaded).toBe(true);
    expect(portalInfo.playgroundEnabled).toBe(false);
    expect(portalInfo.landingVariant).toBe('auto');
  });

  test('fetch returns no data: falls back to {playgroundEnabled:false, landingVariant:"login"}, loaded=true, logs warning', async () => {
    mockGET.mockResolvedValueOnce({ data: undefined, error: { message: 'server error' } });
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

    const { portalInfo } = await import('$lib/portalInfo.svelte');

    await portalInfo.init();

    expect(portalInfo.loaded).toBe(true);
    expect(portalInfo.playgroundEnabled).toBe(false);
    expect(portalInfo.landingVariant).toBe('login');
    expect(warnSpy).toHaveBeenCalledOnce();
    expect(warnSpy.mock.calls[0][0]).toContain('[portalInfo]');
  });

  test('fetch throws (network error): falls back to login-safe defaults, loaded=true, logs warning', async () => {
    mockGET.mockRejectedValueOnce(new Error('Network failure'));
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

    const { portalInfo } = await import('$lib/portalInfo.svelte');

    await portalInfo.init();

    expect(portalInfo.loaded).toBe(true);
    expect(portalInfo.playgroundEnabled).toBe(false);
    expect(portalInfo.landingVariant).toBe('login');
    expect(warnSpy).toHaveBeenCalledOnce();
    expect(warnSpy.mock.calls[0][0]).toContain('[portalInfo]');
  });

  test('second init() call is a no-op (does not refetch)', async () => {
    mockGET.mockResolvedValue({
      data: { playground_enabled: true, landing_variant: 'project' },
      error: undefined,
    });

    const { portalInfo } = await import('$lib/portalInfo.svelte');

    await portalInfo.init();
    await portalInfo.init(); // second call

    expect(mockGET).toHaveBeenCalledTimes(1);
    expect(portalInfo.loaded).toBe(true);
  });

  test('concurrent init() calls share the same in-flight promise (not duplicate fetches)', async () => {
    let resolveFirst!: (v: unknown) => void;
    const firstFetch = new Promise((r) => { resolveFirst = r; });
    mockGET.mockReturnValueOnce(firstFetch);

    const { portalInfo } = await import('$lib/portalInfo.svelte');

    // Fire two concurrent inits before the first resolves.
    const p1 = portalInfo.init();
    const p2 = portalInfo.init();

    resolveFirst({ data: { playground_enabled: false, landing_variant: 'login' }, error: undefined });

    await Promise.all([p1, p2]);

    expect(mockGET).toHaveBeenCalledTimes(1);
    expect(portalInfo.loaded).toBe(true);
  });
});
