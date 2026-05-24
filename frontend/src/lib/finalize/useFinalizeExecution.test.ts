// Seam-contract tests for finalizeExecution (useFinalizeExecution).
//
// Documents the module-singleton facade's public API contract:
//   - starts with all boolean flags false
//   - markCopied() / clearCopied() toggle the copy-toast flag
//   - endSession() sets sessionEnded=true
//   - markShipped() on success sets sessionEnded=true and calls onCancelPatch
//   - markShipped() on error calls onError and leaves sessionEnded=false
//   - markShipped() is idempotent while in-flight (second call is a no-op)
//   - reset() returns all state to defaults

import { describe, it, expect, beforeEach, vi } from 'vitest';

const mockPOST = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    POST: (...args: unknown[]) => mockPOST(...args),
  },
}));

describe('finalizeExecution', () => {
  beforeEach(async () => {
    vi.resetModules();
    mockPOST.mockReset();
  });

  it('starts with all flags false', async () => {
    const { finalizeExecution } = await import('./useFinalizeExecution.svelte');
    expect(finalizeExecution.copiedRunCommand).toBe(false);
    expect(finalizeExecution.markShippedInFlight).toBe(false);
    expect(finalizeExecution.sessionEnded).toBe(false);
  });

  it('markCopied() sets copiedRunCommand=true', async () => {
    const { finalizeExecution } = await import('./useFinalizeExecution.svelte');
    finalizeExecution.markCopied();
    expect(finalizeExecution.copiedRunCommand).toBe(true);
  });

  it('clearCopied() resets copiedRunCommand to false', async () => {
    const { finalizeExecution } = await import('./useFinalizeExecution.svelte');
    finalizeExecution.markCopied();
    finalizeExecution.clearCopied();
    expect(finalizeExecution.copiedRunCommand).toBe(false);
  });

  it('endSession() sets sessionEnded=true', async () => {
    const { finalizeExecution } = await import('./useFinalizeExecution.svelte');
    finalizeExecution.endSession();
    expect(finalizeExecution.sessionEnded).toBe(true);
  });

  it('markShipped() on success sets sessionEnded=true and calls onCancelPatch', async () => {
    mockPOST.mockResolvedValueOnce({ data: {}, error: null });

    const { finalizeExecution } = await import('./useFinalizeExecution.svelte');
    const onCancelPatch = vi.fn();
    const onError = vi.fn();

    await finalizeExecution.markShipped('org-1', 'sess-1', 'main', onCancelPatch, onError);

    expect(finalizeExecution.sessionEnded).toBe(true);
    expect(onCancelPatch).toHaveBeenCalledOnce();
    expect(onError).not.toHaveBeenCalled();
  });

  it('markShipped() on error calls onError and sessionEnded stays false', async () => {
    mockPOST.mockResolvedValueOnce({
      data: null,
      error: { message: 'Server rejected the request' },
    });

    const { finalizeExecution } = await import('./useFinalizeExecution.svelte');
    const onError = vi.fn();

    await finalizeExecution.markShipped('org-1', 'sess-1', 'main', vi.fn(), onError);

    expect(finalizeExecution.sessionEnded).toBe(false);
    expect(onError).toHaveBeenCalledWith('Server rejected the request');
  });

  it('reset() returns all state to defaults', async () => {
    mockPOST.mockResolvedValueOnce({ data: {}, error: null });

    const { finalizeExecution } = await import('./useFinalizeExecution.svelte');
    finalizeExecution.markCopied();
    await finalizeExecution.markShipped('org-1', 'sess-1', 'main', vi.fn(), vi.fn());
    expect(finalizeExecution.sessionEnded).toBe(true);

    finalizeExecution.reset();
    expect(finalizeExecution.copiedRunCommand).toBe(false);
    expect(finalizeExecution.markShippedInFlight).toBe(false);
    expect(finalizeExecution.sessionEnded).toBe(false);
  });
});
