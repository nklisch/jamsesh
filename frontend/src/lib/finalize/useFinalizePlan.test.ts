// Seam-contract tests for finalizePlan (useFinalizePlan).
//
// Documents the module-singleton facade's public API contract:
//   - starts plan=null, loading=false, planId=''
//   - refetch() on success sets plan and returns it; onError not called
//   - refetch() on error calls onError callback and returns null
//   - schedulePatch fires the PATCH after the debounce delay
//   - cancelPendingPatch prevents a scheduled PATCH from firing
//   - reset() clears plan state and cancels any pending patch

import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';

const mockGET = vi.fn();
const mockPATCH = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    GET: (...args: unknown[]) => mockGET(...args),
    PATCH: (...args: unknown[]) => mockPATCH(...args),
  },
}));

describe('finalizePlan', () => {
  beforeEach(async () => {
    vi.resetModules();
    vi.useFakeTimers();
    mockGET.mockReset();
    mockPATCH.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('starts with plan=null, loading=false', async () => {
    const { finalizePlan } = await import('./useFinalizePlan.svelte');
    expect(finalizePlan.plan).toBeNull();
    expect(finalizePlan.loading).toBe(false);
    expect(finalizePlan.planId).toBe('');
  });

  it('refetch() on success sets plan and returns it', async () => {
    const planData = { plan_id: 'plan-1', commits: [], base_sha: 'abc', target_branch: 'main' };
    mockGET.mockResolvedValueOnce({ data: planData, error: null });

    const { finalizePlan } = await import('./useFinalizePlan.svelte');
    const onError = vi.fn();
    const result = await finalizePlan.refetch('org-1', 'sess-1', 'lock-1', onError);

    expect(result).toEqual(planData);
    expect(finalizePlan.plan).toEqual(planData);
    expect(finalizePlan.planId).toBe('plan-1');
    expect(onError).not.toHaveBeenCalled();
  });

  it('refetch() on error calls onError and returns null', async () => {
    mockGET.mockResolvedValueOnce({ data: null, error: { message: 'Plan not found' } });

    const { finalizePlan } = await import('./useFinalizePlan.svelte');
    const onError = vi.fn();
    const result = await finalizePlan.refetch('org-1', 'sess-1', 'lock-1', onError);

    expect(result).toBeNull();
    expect(finalizePlan.plan).toBeNull();
    expect(onError).toHaveBeenCalledWith('Plan not found');
  });

  it('schedulePatch fires PATCH after debounce delay', async () => {
    mockPATCH.mockResolvedValueOnce({ data: {}, error: null });

    const { finalizePlan } = await import('./useFinalizePlan.svelte');
    const onError = vi.fn();
    const onSuccess = vi.fn();

    finalizePlan.schedulePatch({
      orgId: 'org-1',
      sessionId: 'sess-1',
      lockId: 'lock-1',
      selectedShas: ['sha-a'],
      targetBranch: 'main',
      baseSha: 'base-sha',
      mode: 'squash',
      commitMessage: 'msg',
      onError,
      onSuccess,
    });

    expect(mockPATCH).not.toHaveBeenCalled();
    await vi.runAllTimersAsync();
    expect(mockPATCH).toHaveBeenCalledOnce();
    expect(onSuccess).toHaveBeenCalledOnce();
  });

  it('cancelPendingPatch prevents the PATCH from firing', async () => {
    const { finalizePlan } = await import('./useFinalizePlan.svelte');
    const onSuccess = vi.fn();

    finalizePlan.schedulePatch({
      orgId: 'org-1',
      sessionId: 'sess-1',
      lockId: 'lock-1',
      selectedShas: ['sha-a'],
      targetBranch: 'main',
      baseSha: 'base-sha',
      mode: 'squash',
      commitMessage: 'msg',
      onError: vi.fn(),
      onSuccess,
    });

    finalizePlan.cancelPendingPatch();
    await vi.runAllTimersAsync();

    expect(mockPATCH).not.toHaveBeenCalled();
    expect(onSuccess).not.toHaveBeenCalled();
  });

  it('reset() clears plan and cancels any pending patch', async () => {
    const planData = { plan_id: 'plan-1', commits: [], base_sha: 'abc', target_branch: 'main' };
    mockGET.mockResolvedValueOnce({ data: planData, error: null });

    const { finalizePlan } = await import('./useFinalizePlan.svelte');
    await finalizePlan.refetch('org-1', 'sess-1', 'lock-1', vi.fn());
    expect(finalizePlan.plan).not.toBeNull();

    finalizePlan.schedulePatch({
      orgId: 'org-1',
      sessionId: 'sess-1',
      lockId: 'lock-1',
      selectedShas: [],
      targetBranch: '',
      baseSha: '',
      mode: 'squash',
      commitMessage: '',
      onError: vi.fn(),
      onSuccess: vi.fn(),
    });

    finalizePlan.reset();
    await vi.runAllTimersAsync();

    expect(finalizePlan.plan).toBeNull();
    expect(finalizePlan.planId).toBe('');
    // The scheduled PATCH was cancelled by reset() — no HTTP call.
    expect(mockPATCH).not.toHaveBeenCalled();
  });
});
