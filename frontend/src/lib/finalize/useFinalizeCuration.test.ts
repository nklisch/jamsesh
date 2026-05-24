// Seam-contract tests for finalizeCuration (useFinalizeCuration).
//
// Documents the module-singleton facade's public API contract:
//   - defaults: empty selectedShas, empty groups, mode=squash, blank target/message
//   - canRun is false until selection + target + message are all present
//   - addCommit / removeCommit maintain ordered, deduplicated selection
//   - moveUp / moveDown reorder the selection array
//   - addAllInGroup adds all shas not yet in the selection
//   - setMode / setTargetBranch / setCommitMessage update the corresponding state
//   - adoptFromPlan only adopts fields that are currently empty (idempotent guard)
//   - reset() returns all state to defaults

import { describe, it, expect, beforeEach, vi } from 'vitest';

// Mock the API client before importing the module under test so the
// loadRefs method never makes real HTTP requests.
vi.mock('$lib/api/client', () => ({
  client: {
    GET: vi.fn(),
  },
}));

describe('finalizeCuration', () => {
  beforeEach(async () => {
    // Reset module between tests so module-level $state starts fresh.
    vi.resetModules();
  });

  it('starts with empty selection and squash mode', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    expect(finalizeCuration.selectedShas).toEqual([]);
    expect(finalizeCuration.availableGroups).toEqual([]);
    expect(finalizeCuration.mode).toBe('squash');
    expect(finalizeCuration.targetBranch).toBe('');
    expect(finalizeCuration.commitMessage).toBe('');
  });

  it('canRun is false when selection is empty', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.setTargetBranch('main');
    finalizeCuration.setCommitMessage('squash msg');
    expect(finalizeCuration.canRun).toBe(false);
  });

  it('canRun is false when targetBranch is blank', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.addCommit('sha-abc');
    finalizeCuration.setCommitMessage('msg');
    expect(finalizeCuration.canRun).toBe(false);
  });

  it('canRun is false in squash mode when commitMessage is blank', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.addCommit('sha-abc');
    finalizeCuration.setTargetBranch('main');
    expect(finalizeCuration.canRun).toBe(false);
  });

  it('canRun is true when all required fields are set in squash mode', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.addCommit('sha-abc');
    finalizeCuration.setTargetBranch('main');
    finalizeCuration.setCommitMessage('my message');
    expect(finalizeCuration.canRun).toBe(true);
  });

  it('canRun is true in preserve mode without a commitMessage', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.addCommit('sha-abc');
    finalizeCuration.setTargetBranch('main');
    finalizeCuration.setMode('preserve');
    // commitMessage still blank — preserve mode does not require it
    expect(finalizeCuration.canRun).toBe(true);
  });

  it('addCommit appends a sha and deduplicates', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.addCommit('sha-1');
    finalizeCuration.addCommit('sha-2');
    finalizeCuration.addCommit('sha-1'); // duplicate
    expect(finalizeCuration.selectedShas).toEqual(['sha-1', 'sha-2']);
  });

  it('removeCommit removes a sha from the selection', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.addCommit('sha-1');
    finalizeCuration.addCommit('sha-2');
    finalizeCuration.removeCommit('sha-1');
    expect(finalizeCuration.selectedShas).toEqual(['sha-2']);
  });

  it('moveUp swaps the item with its predecessor', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.addCommit('sha-1');
    finalizeCuration.addCommit('sha-2');
    finalizeCuration.addCommit('sha-3');
    finalizeCuration.moveUp(2); // move sha-3 up
    expect(finalizeCuration.selectedShas).toEqual(['sha-1', 'sha-3', 'sha-2']);
  });

  it('moveUp at index 0 is a no-op', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.addCommit('sha-1');
    finalizeCuration.addCommit('sha-2');
    finalizeCuration.moveUp(0);
    expect(finalizeCuration.selectedShas).toEqual(['sha-1', 'sha-2']);
  });

  it('moveDown swaps the item with its successor', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.addCommit('sha-1');
    finalizeCuration.addCommit('sha-2');
    finalizeCuration.addCommit('sha-3');
    finalizeCuration.moveDown(0); // move sha-1 down
    expect(finalizeCuration.selectedShas).toEqual(['sha-2', 'sha-1', 'sha-3']);
  });

  it('adoptFromPlan does not overwrite non-empty fields', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.setTargetBranch('existing-branch');
    finalizeCuration.adoptFromPlan({ targetBranch: 'plan-branch' });
    expect(finalizeCuration.targetBranch).toBe('existing-branch');
  });

  it('adoptFromPlan populates empty fields', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.adoptFromPlan({
      targetBranch: 'plan-branch',
      commitMessage: 'plan msg',
      selectedShas: ['sha-p1'],
    });
    expect(finalizeCuration.targetBranch).toBe('plan-branch');
    expect(finalizeCuration.commitMessage).toBe('plan msg');
    expect(finalizeCuration.selectedShas).toEqual(['sha-p1']);
  });

  it('reset() returns all state to defaults', async () => {
    const { finalizeCuration } = await import('./useFinalizeCuration.svelte');
    finalizeCuration.addCommit('sha-abc');
    finalizeCuration.setTargetBranch('main');
    finalizeCuration.setCommitMessage('msg');
    finalizeCuration.setIsCaller(true);
    finalizeCuration.reset();

    expect(finalizeCuration.selectedShas).toEqual([]);
    expect(finalizeCuration.targetBranch).toBe('');
    expect(finalizeCuration.commitMessage).toBe('');
    expect(finalizeCuration.isCaller).toBe(false);
  });
});
