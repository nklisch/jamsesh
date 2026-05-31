// Seam-contract tests for createFinalizeLock (useFinalizeLock).
//
// Each test calls createFinalizeLock() to get a fresh per-instance facade,
// asserting isolation between instances. Documents the public API contract:
//   - starts loading=true, status/conflict/error all null
//   - acquire() on 200 sets status and clears conflict
//   - acquire() on 409 sets conflict.holderAccountId and clears status
//   - acquire() on other error sets lockError message
//   - dismissError() clears the error string
//   - reset() returns all state to defaults
//   - two instances are isolated (state on A does not bleed to B)

import { describe, it, expect, beforeEach, vi } from 'vitest';

const mockPOST = vi.fn();
const mockDELETE = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    POST: (...args: unknown[]) => mockPOST(...args),
    DELETE: (...args: unknown[]) => mockDELETE(...args),
  },
}));

describe('createFinalizeLock', () => {
  beforeEach(async () => {
    vi.resetModules();
    mockPOST.mockReset();
    mockDELETE.mockReset();
  });

  it('starts with loading=true and all nullable fields null', async () => {
    const { createFinalizeLock } = await import('./useFinalizeLock.svelte');
    const lock = createFinalizeLock();
    expect(lock.loading).toBe(true);
    expect(lock.status).toBeNull();
    expect(lock.conflict).toBeNull();
    expect(lock.error).toBeNull();
  });

  it('acquire() on 200 sets status and sets loading=false', async () => {
    const lockData = { lock_id: 'lock-1', session_id: 'sess-1', held_by_account_id: 'user-1' };
    mockPOST.mockResolvedValueOnce({ data: lockData, error: null, response: { status: 200 } });

    const { createFinalizeLock } = await import('./useFinalizeLock.svelte');
    const lock = createFinalizeLock();
    await lock.acquire('org-1', 'sess-1');

    expect(lock.loading).toBe(false);
    expect(lock.status).toEqual(lockData);
    expect(lock.conflict).toBeNull();
    expect(lock.error).toBeNull();
  });

  it('acquire() on 409 sets conflict.holderAccountId', async () => {
    mockPOST.mockResolvedValueOnce({
      data: null,
      error: { details: { held_by_account_id: 'user-other' } },
      response: { status: 409 },
    });

    const { createFinalizeLock } = await import('./useFinalizeLock.svelte');
    const lock = createFinalizeLock();
    await lock.acquire('org-1', 'sess-1');

    expect(lock.loading).toBe(false);
    expect(lock.status).toBeNull();
    expect(lock.conflict).toEqual({ holderAccountId: 'user-other' });
    expect(lock.error).toBeNull();
  });

  it('acquire() on non-409 error sets lockError', async () => {
    mockPOST.mockResolvedValueOnce({
      data: null,
      error: { message: 'Internal server error' },
      response: { status: 500 },
    });

    const { createFinalizeLock } = await import('./useFinalizeLock.svelte');
    const lock = createFinalizeLock();
    await lock.acquire('org-1', 'sess-1');

    expect(lock.error).toBe('Internal server error');
    expect(lock.status).toBeNull();
    expect(lock.conflict).toBeNull();
  });

  it('dismissError() clears the error field', async () => {
    mockPOST.mockResolvedValueOnce({
      data: null,
      error: { message: 'Oops' },
      response: { status: 500 },
    });

    const { createFinalizeLock } = await import('./useFinalizeLock.svelte');
    const lock = createFinalizeLock();
    await lock.acquire('org-1', 'sess-1');
    expect(lock.error).not.toBeNull();

    lock.dismissError();
    expect(lock.error).toBeNull();
  });

  it('reset() returns all state to defaults', async () => {
    const lockData = { lock_id: 'lock-1', session_id: 'sess-1', held_by_account_id: 'user-1' };
    mockPOST.mockResolvedValueOnce({ data: lockData, error: null, response: { status: 200 } });

    const { createFinalizeLock } = await import('./useFinalizeLock.svelte');
    const lock = createFinalizeLock();
    await lock.acquire('org-1', 'sess-1');
    expect(lock.status).not.toBeNull();

    lock.reset();
    expect(lock.status).toBeNull();
    expect(lock.conflict).toBeNull();
    expect(lock.error).toBeNull();
    expect(lock.loading).toBe(true);
  });

  it('two instances are isolated — state on A does not bleed to B', async () => {
    const lockDataA = { lock_id: 'lock-A', session_id: 'sess-A', held_by_account_id: 'user-1', is_caller: true };
    mockPOST.mockResolvedValueOnce({ data: lockDataA, error: null, response: { status: 200 } });

    const { createFinalizeLock } = await import('./useFinalizeLock.svelte');
    const lockA = createFinalizeLock();
    const lockB = createFinalizeLock();

    // Acquire on A only
    await lockA.acquire('org-1', 'sess-A');

    expect(lockA.status).toEqual(lockDataA);
    // B must remain untouched
    expect(lockB.status).toBeNull();
    expect(lockB.loading).toBe(true);
  });

  it('destroy-during-acquire: alive guard — late-acquired lock is released and startSubscriptions skipped', async () => {
    // This test simulates the FinalizeView onMount alive-guard pattern:
    // we create a lock, start an acquire that we control, then "destroy"
    // (set alive=false) before resolving — verify release is called and
    // the caller knows not to start subscriptions.
    let resolveAcquire!: (val: { data: unknown; error: null; response: { status: number } }) => void;
    const acquirePromise = new Promise<{ data: unknown; error: null; response: { status: number } }>((res) => {
      resolveAcquire = res;
    });
    mockPOST.mockReturnValueOnce(acquirePromise);
    mockDELETE.mockResolvedValue({ data: null, error: null });

    const { createFinalizeLock } = await import('./useFinalizeLock.svelte');
    const lock = createFinalizeLock();

    let alive = true;
    let startSubscriptionsCalled = false;

    // Simulate onMount async IIFE
    const mountPromise = (async () => {
      await lock.acquire('org-1', 'sess-1');
      if (!alive) {
        void lock.release('org-1', 'sess-1');
        return; // skip startSubscriptions
      }
      startSubscriptionsCalled = true;
    })();

    // Simulate onDestroy firing before the acquire resolves
    alive = false;

    // Now resolve the acquire with a valid lock
    const lockData = { lock_id: 'lock-late', session_id: 'sess-1', held_by_account_id: 'user-1', is_caller: true };
    resolveAcquire({ data: lockData, error: null, response: { status: 201 } });

    await mountPromise;

    // release() should have been called (fire-and-forget via DELETE)
    expect(mockDELETE).toHaveBeenCalledOnce();
    // startSubscriptions must NOT have run
    expect(startSubscriptionsCalled).toBe(false);
  });
});
