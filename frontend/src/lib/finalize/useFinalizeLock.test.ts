// Seam-contract tests for finalizeLock (useFinalizeLock).
//
// Documents the module-singleton facade's public API contract:
//   - starts loading=true, status/conflict/error all null
//   - acquire() on 200 sets status and clears conflict
//   - acquire() on 409 sets conflict.holderAccountId and clears status
//   - acquire() on other error sets lockError message
//   - dismissError() clears the error string
//   - reset() returns all state to defaults

import { describe, it, expect, beforeEach, vi } from 'vitest';

const mockPOST = vi.fn();
const mockDELETE = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    POST: (...args: unknown[]) => mockPOST(...args),
    DELETE: (...args: unknown[]) => mockDELETE(...args),
  },
}));

describe('finalizeLock', () => {
  beforeEach(async () => {
    vi.resetModules();
    mockPOST.mockReset();
    mockDELETE.mockReset();
  });

  it('starts with loading=true and all nullable fields null', async () => {
    const { finalizeLock } = await import('./useFinalizeLock.svelte');
    expect(finalizeLock.loading).toBe(true);
    expect(finalizeLock.status).toBeNull();
    expect(finalizeLock.conflict).toBeNull();
    expect(finalizeLock.error).toBeNull();
  });

  it('acquire() on 200 sets status and sets loading=false', async () => {
    const lockData = { lock_id: 'lock-1', session_id: 'sess-1', held_by_account_id: 'user-1' };
    mockPOST.mockResolvedValueOnce({ data: lockData, error: null, response: { status: 200 } });

    const { finalizeLock } = await import('./useFinalizeLock.svelte');
    await finalizeLock.acquire('org-1', 'sess-1');

    expect(finalizeLock.loading).toBe(false);
    expect(finalizeLock.status).toEqual(lockData);
    expect(finalizeLock.conflict).toBeNull();
    expect(finalizeLock.error).toBeNull();
  });

  it('acquire() on 409 sets conflict.holderAccountId', async () => {
    mockPOST.mockResolvedValueOnce({
      data: null,
      error: { details: { held_by_account_id: 'user-other' } },
      response: { status: 409 },
    });

    const { finalizeLock } = await import('./useFinalizeLock.svelte');
    await finalizeLock.acquire('org-1', 'sess-1');

    expect(finalizeLock.loading).toBe(false);
    expect(finalizeLock.status).toBeNull();
    expect(finalizeLock.conflict).toEqual({ holderAccountId: 'user-other' });
    expect(finalizeLock.error).toBeNull();
  });

  it('acquire() on non-409 error sets lockError', async () => {
    mockPOST.mockResolvedValueOnce({
      data: null,
      error: { message: 'Internal server error' },
      response: { status: 500 },
    });

    const { finalizeLock } = await import('./useFinalizeLock.svelte');
    await finalizeLock.acquire('org-1', 'sess-1');

    expect(finalizeLock.error).toBe('Internal server error');
    expect(finalizeLock.status).toBeNull();
    expect(finalizeLock.conflict).toBeNull();
  });

  it('dismissError() clears the error field', async () => {
    mockPOST.mockResolvedValueOnce({
      data: null,
      error: { message: 'Oops' },
      response: { status: 500 },
    });

    const { finalizeLock } = await import('./useFinalizeLock.svelte');
    await finalizeLock.acquire('org-1', 'sess-1');
    expect(finalizeLock.error).not.toBeNull();

    finalizeLock.dismissError();
    expect(finalizeLock.error).toBeNull();
  });

  it('reset() returns all state to defaults', async () => {
    const lockData = { lock_id: 'lock-1', session_id: 'sess-1', held_by_account_id: 'user-1' };
    mockPOST.mockResolvedValueOnce({ data: lockData, error: null, response: { status: 200 } });

    const { finalizeLock } = await import('./useFinalizeLock.svelte');
    await finalizeLock.acquire('org-1', 'sess-1');
    expect(finalizeLock.status).not.toBeNull();

    finalizeLock.reset();
    expect(finalizeLock.status).toBeNull();
    expect(finalizeLock.conflict).toBeNull();
    expect(finalizeLock.error).toBeNull();
    expect(finalizeLock.loading).toBe(true);
  });
});
