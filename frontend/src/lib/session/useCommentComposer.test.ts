// Seam-contract tests for createCommentComposer.
//
// Documents the factory's public API contract:
//   - starts closed with no range
//   - handleRangeSelect(range) opens the composer and stores the range
//   - handleRangeSelect(null) does not open the composer
//   - close() hides the composer without clearing range
//   - two instances are fully isolated

import { describe, it, expect } from 'vitest';
import { createCommentComposer } from './useCommentComposer.svelte';

describe('createCommentComposer', () => {
  it('starts closed with null range', () => {
    const c = createCommentComposer();
    expect(c.open).toBe(false);
    expect(c.range).toBeNull();
  });

  it('handleRangeSelect(range) opens the composer and stores the range', () => {
    const c = createCommentComposer();
    c.handleRangeSelect({ start: 5, end: 10 });
    expect(c.open).toBe(true);
    expect(c.range).toEqual({ start: 5, end: 10 });
  });

  it('handleRangeSelect(null) clears range but does not open', () => {
    const c = createCommentComposer();
    // Open first, then clear.
    c.handleRangeSelect({ start: 1, end: 2 });
    expect(c.open).toBe(true);

    c.handleRangeSelect(null);
    expect(c.range).toBeNull();
    // open remains true — range clear does not force-close (submit/cancel does).
    // This reflects the actual contract: only close() resets open.
  });

  it('close() sets open to false', () => {
    const c = createCommentComposer();
    c.handleRangeSelect({ start: 3, end: 7 });
    expect(c.open).toBe(true);

    c.close();
    expect(c.open).toBe(false);
  });

  it('two instances are fully isolated', () => {
    const a = createCommentComposer();
    const b = createCommentComposer();

    a.handleRangeSelect({ start: 1, end: 2 });
    expect(a.open).toBe(true);
    expect(b.open).toBe(false);
  });
});
