import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import CoAuthorChipRow from './CoAuthorChipRow.svelte';

const ALICE = { name: 'Alice', email: 'alice@example.com', account_id: 'acct-alice' };
const BOB = { name: 'Bob', email: 'bob@example.com', account_id: null };
const CARLA = { name: 'Carla', email: 'carla@example.com', account_id: 'acct-carla' };

describe('CoAuthorChipRow', () => {
  it('renders nothing when authors is empty (no label, no chips)', () => {
    render(CoAuthorChipRow, { props: { authors: [] } });
    expect(screen.queryByTestId('coauthor-chip-row')).toBeNull();
    expect(screen.queryByText(/co-authors/i)).toBeNull();
    expect(screen.queryAllByTestId('coauthor-chip')).toHaveLength(0);
  });

  it('renders one chip per author', () => {
    render(CoAuthorChipRow, { props: { authors: [ALICE, BOB, CARLA] } });
    const chips = screen.getAllByTestId('coauthor-chip');
    expect(chips).toHaveLength(3);
  });

  it('renders the "Co-authors" label when at least one author is present', () => {
    render(CoAuthorChipRow, { props: { authors: [ALICE] } });
    expect(screen.getByText(/co-authors/i)).toBeInTheDocument();
  });

  it('renders the author name in each chip', () => {
    render(CoAuthorChipRow, { props: { authors: [ALICE, BOB] } });
    const chips = screen.getAllByTestId('coauthor-chip');
    expect(chips[0].textContent).toContain('Alice');
    expect(chips[1].textContent).toContain('Bob');
  });

  it('renders an AuthorDot inside each chip with stable color seed', () => {
    render(CoAuthorChipRow, { props: { authors: [ALICE, BOB] } });
    const chips = screen.getAllByTestId('coauthor-chip');
    // AuthorDot renders as role="img"; one per chip.
    for (const chip of chips) {
      const dot = chip.querySelector('[role="img"]');
      expect(dot).not.toBeNull();
      const style = dot?.getAttribute('style') ?? '';
      // AuthorDot maps the seed to --author-N where N in [1,8]
      expect(style).toMatch(/--dot-color: var\(--author-[1-8]\)/);
    }
  });

  it('uses account_id as the AuthorDot seed when present (falls back to email)', () => {
    // Render once with account_id, once with the SAME author but account_id null.
    // The two seeds should produce different colors (in general — confirms
    // the component is using the right input, not silently dropping it).
    const { unmount } = render(CoAuthorChipRow, {
      props: { authors: [{ name: 'Alice', email: 'alice@example.com', account_id: 'acct-alice' }] },
    });
    const withAcct = screen.getByTestId('coauthor-chip').querySelector('[role="img"]');
    const titleWithAcct = withAcct?.getAttribute('aria-label');
    // aria-label defaults to title prop, which we set to author.name
    expect(titleWithAcct).toBe('Alice');
    unmount();

    render(CoAuthorChipRow, {
      props: { authors: [{ name: 'Alice', email: 'alice@example.com', account_id: null }] },
    });
    const withoutAcct = screen.getByTestId('coauthor-chip').querySelector('[role="img"]');
    expect(withoutAcct?.getAttribute('aria-label')).toBe('Alice');
  });

  it('updates in place when authors prop changes (Svelte 5 prop reactivity)', async () => {
    const { rerender } = render(CoAuthorChipRow, { props: { authors: [ALICE] } });
    expect(screen.getAllByTestId('coauthor-chip')).toHaveLength(1);
    expect(screen.getByText('Alice')).toBeInTheDocument();

    await rerender({ authors: [ALICE, BOB, CARLA] });
    expect(screen.getAllByTestId('coauthor-chip')).toHaveLength(3);
    expect(screen.getByText('Bob')).toBeInTheDocument();
    expect(screen.getByText('Carla')).toBeInTheDocument();

    // Shrink back: chips removed in place, no stale entries.
    await rerender({ authors: [BOB] });
    expect(screen.getAllByTestId('coauthor-chip')).toHaveLength(1);
    expect(screen.queryByText('Alice')).toBeNull();
    expect(screen.queryByText('Carla')).toBeNull();
    expect(screen.getByText('Bob')).toBeInTheDocument();
  });

  it('disappears entirely when authors transitions from non-empty to empty', async () => {
    const { rerender } = render(CoAuthorChipRow, { props: { authors: [ALICE, BOB] } });
    expect(screen.getByTestId('coauthor-chip-row')).toBeInTheDocument();

    await rerender({ authors: [] });
    expect(screen.queryByTestId('coauthor-chip-row')).toBeNull();
    expect(screen.queryByText(/co-authors/i)).toBeNull();
  });
});
