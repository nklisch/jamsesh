// Seam-contract tests for createFinalizePlan (useFinalizePlan).
//
// Each test calls createFinalizePlan() to get a fresh per-instance facade,
// asserting isolation between instances. Documents the public API contract:
//   - starts plan=null, loading=false, planId=''
//   - refetch() on success sets plan and returns it; onError not called
//   - refetch() on error calls onError callback and returns null
//   - schedulePatch fires the PATCH after the debounce delay
//   - cancelPendingPatch prevents a scheduled PATCH from firing
//   - reset() clears plan state and cancels any pending patch
//   - two instances are isolated

import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';

const mockGET = vi.fn();
const mockPATCH = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    GET: (...args: unknown[]) => mockGET(...args),
    PATCH: (...args: unknown[]) => mockPATCH(...args),
  },
}));

describe('createFinalizePlan', () => {
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
    const { createFinalizePlan } = await import('./useFinalizePlan.svelte');
    const plan = createFinalizePlan();
    expect(plan.plan).toBeNull();
    expect(plan.loading).toBe(false);
    expect(plan.planId).toBe('');
  });

  it('refetch() on success sets plan and returns it', async () => {
    const planData = { plan_id: 'plan-1', commits: [], base_sha: 'abc', target_branch: 'main' };
    mockGET.mockResolvedValueOnce({ data: planData, error: null });

    const { createFinalizePlan } = await import('./useFinalizePlan.svelte');
    const plan = createFinalizePlan();
    const onError = vi.fn();
    const result = await plan.refetch('org-1', 'sess-1', 'lock-1', onError);

    expect(result).toEqual(planData);
    expect(plan.plan).toEqual(planData);
    expect(plan.planId).toBe('plan-1');
    expect(onError).not.toHaveBeenCalled();
  });

  it('refetch() on error calls onError and returns null', async () => {
    mockGET.mockResolvedValueOnce({ data: null, error: { message: 'Plan not found' } });

    const { createFinalizePlan } = await import('./useFinalizePlan.svelte');
    const plan = createFinalizePlan();
    const onError = vi.fn();
    const result = await plan.refetch('org-1', 'sess-1', 'lock-1', onError);

    expect(result).toBeNull();
    expect(plan.plan).toBeNull();
    expect(onError).toHaveBeenCalledWith('Plan not found');
  });

  it('schedulePatch fires PATCH after debounce delay', async () => {
    mockPATCH.mockResolvedValueOnce({ data: {}, error: null });

    const { createFinalizePlan } = await import('./useFinalizePlan.svelte');
    const plan = createFinalizePlan();
    const onError = vi.fn();
    const onSuccess = vi.fn();

    plan.schedulePatch({
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
    const { createFinalizePlan } = await import('./useFinalizePlan.svelte');
    const plan = createFinalizePlan();
    const onSuccess = vi.fn();

    plan.schedulePatch({
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

    plan.cancelPendingPatch();
    await vi.runAllTimersAsync();

    expect(mockPATCH).not.toHaveBeenCalled();
    expect(onSuccess).not.toHaveBeenCalled();
  });

  it('reset() clears plan and cancels any pending patch', async () => {
    const planData = { plan_id: 'plan-1', commits: [], base_sha: 'abc', target_branch: 'main' };
    mockGET.mockResolvedValueOnce({ data: planData, error: null });

    const { createFinalizePlan } = await import('./useFinalizePlan.svelte');
    const plan = createFinalizePlan();
    await plan.refetch('org-1', 'sess-1', 'lock-1', vi.fn());
    expect(plan.plan).not.toBeNull();

    plan.schedulePatch({
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

    plan.reset();
    await vi.runAllTimersAsync();

    expect(plan.plan).toBeNull();
    expect(plan.planId).toBe('');
    // The scheduled PATCH was cancelled by reset() — no HTTP call.
    expect(mockPATCH).not.toHaveBeenCalled();
  });

  it('two instances are isolated — plan on A does not bleed to B', async () => {
    const planDataA = { plan_id: 'plan-A', commits: [], base_sha: 'abc', target_branch: 'main' };
    mockGET.mockResolvedValueOnce({ data: planDataA, error: null });

    const { createFinalizePlan } = await import('./useFinalizePlan.svelte');
    const planA = createFinalizePlan();
    const planB = createFinalizePlan();

    await planA.refetch('org-1', 'sess-A', 'lock-A', vi.fn());

    expect(planA.planId).toBe('plan-A');
    expect(planB.plan).toBeNull();
    expect(planB.planId).toBe('');
  });
});
