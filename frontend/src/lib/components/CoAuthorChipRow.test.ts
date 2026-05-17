import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import CoAuthorChipRow from './CoAuthorChipRow.svelte';

// Story 1 ships a placeholder component (empty markup) that just
// defines the prop interface. Story 2 fills in the chip rendering.

describe('CoAuthorChipRow (story-1 placeholder)', () => {
  it('renders without crashing when authors is empty', () => {
    const { container } = render(CoAuthorChipRow, { props: { authors: [] } });
    expect(container).toBeInTheDocument();
  });

  it('renders without crashing when authors prop is omitted (defaults to [])', () => {
    const { container } = render(CoAuthorChipRow, { props: {} });
    expect(container).toBeInTheDocument();
  });

  it('accepts a CoAuthor list prop without throwing (rendering deferred to story 2)', () => {
    const { container } = render(CoAuthorChipRow, {
      props: {
        authors: [
          { name: 'Alice', email: 'alice@example.com', account_id: 'user-1' },
          { name: 'Bob', email: 'bob@example.com', account_id: null },
        ],
      },
    });
    expect(container).toBeInTheDocument();
  });
});
