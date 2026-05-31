// Seam-contract tests for createFinalizeCuration (useFinalizeCuration).
//
// Each test calls createFinalizeCuration() to get a fresh per-instance facade,
// asserting isolation between instances. Documents the public API contract:
//   - defaults: empty selectedShas, empty groups, mode=squash, blank target/message
//   - canRun is false until selection + target + message are all present
//   - addCommit / removeCommit maintain ordered, deduplicated selection
//   - moveUp / moveDown reorder the selection array
//   - addAllInGroup adds all shas not yet in the selection
//   - setMode / setTargetBranch / setCommitMessage update the corresponding state
//   - adoptFromPlan only adopts fields that are currently empty (idempotent guard)
//   - reset() returns all state to defaults
//   - two instances are isolated

import { describe, it, expect, beforeEach, vi } from 'vitest';

// Mock the API client before importing the module under test so the
// loadRefs method never makes real HTTP requests.
vi.mock('$lib/api/client', () => ({
  client: {
    GET: vi.fn(),
  },
}));

describe('createFinalizeCuration', () => {
  beforeEach(async () => {
    // Reset module between tests so any module-level code starts fresh.
    vi.resetModules();
  });

  it('starts with empty selection and squash mode', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    expect(curation.selectedShas).toEqual([]);
    expect(curation.availableGroups).toEqual([]);
    expect(curation.mode).toBe('squash');
    expect(curation.targetBranch).toBe('');
    expect(curation.commitMessage).toBe('');
  });

  it('canRun is false when selection is empty', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.setTargetBranch('main');
    curation.setCommitMessage('squash msg');
    expect(curation.canRun).toBe(false);
  });

  it('canRun is false when targetBranch is blank', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.addCommit('sha-abc');
    curation.setCommitMessage('msg');
    expect(curation.canRun).toBe(false);
  });

  it('canRun is false in squash mode when commitMessage is blank', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.addCommit('sha-abc');
    curation.setTargetBranch('main');
    expect(curation.canRun).toBe(false);
  });

  it('canRun is true when all required fields are set in squash mode', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.addCommit('sha-abc');
    curation.setTargetBranch('main');
    curation.setCommitMessage('my message');
    expect(curation.canRun).toBe(true);
  });

  it('canRun is true in preserve mode without a commitMessage', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.addCommit('sha-abc');
    curation.setTargetBranch('main');
    curation.setMode('preserve');
    // commitMessage still blank — preserve mode does not require it
    expect(curation.canRun).toBe(true);
  });

  it('addCommit appends a sha and deduplicates', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.addCommit('sha-1');
    curation.addCommit('sha-2');
    curation.addCommit('sha-1'); // duplicate
    expect(curation.selectedShas).toEqual(['sha-1', 'sha-2']);
  });

  it('removeCommit removes a sha from the selection', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.addCommit('sha-1');
    curation.addCommit('sha-2');
    curation.removeCommit('sha-1');
    expect(curation.selectedShas).toEqual(['sha-2']);
  });

  it('moveUp swaps the item with its predecessor', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.addCommit('sha-1');
    curation.addCommit('sha-2');
    curation.addCommit('sha-3');
    curation.moveUp(2); // move sha-3 up
    expect(curation.selectedShas).toEqual(['sha-1', 'sha-3', 'sha-2']);
  });

  it('moveUp at index 0 is a no-op', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.addCommit('sha-1');
    curation.addCommit('sha-2');
    curation.moveUp(0);
    expect(curation.selectedShas).toEqual(['sha-1', 'sha-2']);
  });

  it('moveDown swaps the item with its successor', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.addCommit('sha-1');
    curation.addCommit('sha-2');
    curation.addCommit('sha-3');
    curation.moveDown(0); // move sha-1 down
    expect(curation.selectedShas).toEqual(['sha-2', 'sha-1', 'sha-3']);
  });

  it('adoptFromPlan does not overwrite non-empty fields', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.setTargetBranch('existing-branch');
    curation.adoptFromPlan({ targetBranch: 'plan-branch' });
    expect(curation.targetBranch).toBe('existing-branch');
  });

  it('adoptFromPlan populates empty fields', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.adoptFromPlan({
      targetBranch: 'plan-branch',
      commitMessage: 'plan msg',
      selectedShas: ['sha-p1'],
    });
    expect(curation.targetBranch).toBe('plan-branch');
    expect(curation.commitMessage).toBe('plan msg');
    expect(curation.selectedShas).toEqual(['sha-p1']);
  });

  it('reset() returns all state to defaults', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curation = createFinalizeCuration();
    curation.addCommit('sha-abc');
    curation.setTargetBranch('main');
    curation.setCommitMessage('msg');
    curation.setIsCaller(true);
    curation.reset();

    expect(curation.selectedShas).toEqual([]);
    expect(curation.targetBranch).toBe('');
    expect(curation.commitMessage).toBe('');
    expect(curation.isCaller).toBe(false);
  });

  it('two instances are isolated — commits on A do not bleed to B', async () => {
    const { createFinalizeCuration } = await import('./useFinalizeCuration.svelte');
    const curationA = createFinalizeCuration();
    const curationB = createFinalizeCuration();

    curationA.addCommit('sha-A');
    curationA.setTargetBranch('branch-A');

    expect(curationA.selectedShas).toEqual(['sha-A']);
    expect(curationB.selectedShas).toEqual([]);
    expect(curationB.targetBranch).toBe('');
  });
});
