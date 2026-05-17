import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import AuthorDot from './AuthorDot.svelte';

// Re-export the hash function for direct unit testing
function authorColorIndex(id: string): number {
  let h = 0;
  for (let i = 0; i < id.length; i++) {
    h = ((h << 5) - h) + id.charCodeAt(i);
    h |= 0;
  }
  return (Math.abs(h) % 8) + 1;
}

describe('authorColorIndex (hash function)', () => {
  it('returns a value in [1, 8]', () => {
    const ids = ['alice', 'bob', 'charlie', 'diana', 'eve', 'frank', 'grace', 'hank'];
    for (const id of ids) {
      const idx = authorColorIndex(id);
      expect(idx).toBeGreaterThanOrEqual(1);
      expect(idx).toBeLessThanOrEqual(8);
    }
  });

  it('is deterministic — same input produces same output', () => {
    expect(authorColorIndex('alice')).toBe(authorColorIndex('alice'));
    expect(authorColorIndex('user-uuid-1234')).toBe(authorColorIndex('user-uuid-1234'));
  });

  it('produces different indices for different IDs (distribution check)', () => {
    const ids = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'];
    const indices = ids.map(authorColorIndex);
    const unique = new Set(indices);
    // With 8 distinct IDs, expect at least 2 distinct color indices
    expect(unique.size).toBeGreaterThan(1);
  });

  it('handles empty string without throwing', () => {
    expect(() => authorColorIndex('')).not.toThrow();
    const idx = authorColorIndex('');
    expect(idx).toBeGreaterThanOrEqual(1);
    expect(idx).toBeLessThanOrEqual(8);
  });
});

describe('AuthorDot component', () => {
  it('renders with role=img and aria-label defaulting to authorId', () => {
    render(AuthorDot, { props: { authorId: 'alice' } });
    expect(screen.getByRole('img', { name: 'author alice' })).toBeInTheDocument();
  });

  it('uses custom title for aria-label when provided', () => {
    render(AuthorDot, { props: { authorId: 'alice', title: 'Alice Cooper' } });
    expect(screen.getByRole('img', { name: 'Alice Cooper' })).toBeInTheDocument();
  });

  it('does not render pulse indicator when online=false', () => {
    render(AuthorDot, { props: { authorId: 'alice', online: false } });
    expect(document.querySelector('.pulse')).toBeNull();
  });

  it('renders pulse indicator when online=true', () => {
    render(AuthorDot, { props: { authorId: 'alice', online: true } });
    expect(document.querySelector('.pulse')).toBeInTheDocument();
  });

  it('sets --dot-size CSS variable based on size prop', () => {
    render(AuthorDot, { props: { authorId: 'alice', size: 24 } });
    const dot = screen.getByRole('img');
    expect(dot.getAttribute('style')).toContain('--dot-size: 24px');
  });

  it('sets --dot-color CSS variable using --author-N pattern', () => {
    render(AuthorDot, { props: { authorId: 'alice' } });
    const dot = screen.getByRole('img');
    const style = dot.getAttribute('style') ?? '';
    expect(style).toMatch(/--dot-color: var\(--author-[1-8]\)/);
  });

  it('produces same color index for same authorId across renders', () => {
    const { unmount } = render(AuthorDot, { props: { authorId: 'alice' } });
    const style1 = screen.getByRole('img').getAttribute('style') ?? '';
    unmount();

    render(AuthorDot, { props: { authorId: 'alice' } });
    const style2 = screen.getByRole('img').getAttribute('style') ?? '';
    expect(style1).toBe(style2);
  });
});
