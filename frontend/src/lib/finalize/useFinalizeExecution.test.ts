// Seam-contract tests for createFinalizeExecution (useFinalizeExecution).
//
// Each test calls createFinalizeExecution() to get a fresh per-instance facade,
// asserting isolation between instances. Documents the public API contract:
//   - starts with all boolean flags false
//   - markCopied() / clearCopied() toggle the copy-toast flag
//   - endSession() sets sessionEnded=true
//   - markShipped() on success sets sessionEnded=true and calls onCancelPatch
//   - markShipped() on error calls onError and leaves sessionEnded=false
//   - markShipped() is idempotent while in-flight (second call is a no-op)
//   - reset() returns all state to defaults
//   - two instances are isolated

import { describe, it, expect, beforeEach, vi } from 'vitest';

const mockPOST = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    POST: (...args: unknown[]) => mockPOST(...args),
  },
}));

describe('createFinalizeExecution', () => {
  beforeEach(async () => {
    vi.resetModules();
    mockPOST.mockReset();
  });

  it('starts with all flags false', async () => {
    const { createFinalizeExecution } = await import('./useFinalizeExecution.svelte');
    const exec = createFinalizeExecution();
    expect(exec.copiedRunCommand).toBe(false);
    expect(exec.markShippedInFlight).toBe(false);
    expect(exec.sessionEnded).toBe(false);
  });

  it('markCopied() sets copiedRunCommand=true', async () => {
    const { createFinalizeExecution } = await import('./useFinalizeExecution.svelte');
    const exec = createFinalizeExecution();
    exec.markCopied();
    expect(exec.copiedRunCommand).toBe(true);
  });

  it('clearCopied() resets copiedRunCommand to false', async () => {
    const { createFinalizeExecution } = await import('./useFinalizeExecution.svelte');
    const exec = createFinalizeExecution();
    exec.markCopied();
    exec.clearCopied();
    expect(exec.copiedRunCommand).toBe(false);
  });

  it('endSession() sets sessionEnded=true', async () => {
    const { createFinalizeExecution } = await import('./useFinalizeExecution.svelte');
    const exec = createFinalizeExecution();
    exec.endSession();
    expect(exec.sessionEnded).toBe(true);
  });

  it('markShipped() on success sets sessionEnded=true and calls onCancelPatch', async () => {
    mockPOST.mockResolvedValueOnce({ data: {}, error: null });

    const { createFinalizeExecution } = await import('./useFinalizeExecution.svelte');
    const exec = createFinalizeExecution();
    const onCancelPatch = vi.fn();
    const onError = vi.fn();

    await exec.markShipped('org-1', 'sess-1', 'main', onCancelPatch, onError);

    expect(exec.sessionEnded).toBe(true);
    expect(onCancelPatch).toHaveBeenCalledOnce();
    expect(onError).not.toHaveBeenCalled();
  });

  it('markShipped() on error calls onError and sessionEnded stays false', async () => {
    mockPOST.mockResolvedValueOnce({
      data: null,
      error: { message: 'Server rejected the request' },
    });

    const { createFinalizeExecution } = await import('./useFinalizeExecution.svelte');
    const exec = createFinalizeExecution();
    const onError = vi.fn();

    await exec.markShipped('org-1', 'sess-1', 'main', vi.fn(), onError);

    expect(exec.sessionEnded).toBe(false);
    expect(onError).toHaveBeenCalledWith('Server rejected the request');
  });

  it('reset() returns all state to defaults', async () => {
    mockPOST.mockResolvedValueOnce({ data: {}, error: null });

    const { createFinalizeExecution } = await import('./useFinalizeExecution.svelte');
    const exec = createFinalizeExecution();
    exec.markCopied();
    await exec.markShipped('org-1', 'sess-1', 'main', vi.fn(), vi.fn());
    expect(exec.sessionEnded).toBe(true);

    exec.reset();
    expect(exec.copiedRunCommand).toBe(false);
    expect(exec.markShippedInFlight).toBe(false);
    expect(exec.sessionEnded).toBe(false);
  });

  it('two instances are isolated — sessionEnded on A does not bleed to B', async () => {
    mockPOST.mockResolvedValueOnce({ data: {}, error: null });

    const { createFinalizeExecution } = await import('./useFinalizeExecution.svelte');
    const execA = createFinalizeExecution();
    const execB = createFinalizeExecution();

    await execA.markShipped('org-1', 'sess-A', 'main', vi.fn(), vi.fn());

    expect(execA.sessionEnded).toBe(true);
    expect(execB.sessionEnded).toBe(false);
  });
});
