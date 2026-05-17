import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import SquashMessageEditor from './SquashMessageEditor.svelte';

const ALICE = { name: 'Alice', email: 'alice@example.com', account_id: 'acct-alice' };
const BOB = { name: 'Bob', email: 'bob@example.com', account_id: null };

describe('SquashMessageEditor', () => {
  it('renders the message prop pre-populated in the textarea', () => {
    render(SquashMessageEditor, {
      props: {
        message: 'Auth design refresh: OAuth providers, session refresh',
        onmessagechange: () => {},
        coAuthors: [],
      },
    });
    const textarea = screen.getByLabelText(/squash commit message/i) as HTMLTextAreaElement;
    expect(textarea.value).toBe('Auth design refresh: OAuth providers, session refresh');
  });

  it('renders the hint row with the squash-mode pre-fill description', () => {
    render(SquashMessageEditor, {
      props: { message: '', onmessagechange: () => {}, coAuthors: [] },
    });
    // Mock copy: "Squash mode · pre-filled from session goal + commit subjects · editable"
    expect(screen.getByText(/squash mode/i)).toBeInTheDocument();
    expect(screen.getByText(/pre-filled from session goal/i)).toBeInTheDocument();
    expect(screen.getByText(/editable/i)).toBeInTheDocument();
  });

  it('renders the "Commit message" label associated with the textarea', () => {
    render(SquashMessageEditor, {
      props: { message: '', onmessagechange: () => {}, coAuthors: [] },
    });
    const label = screen.getByText('Commit message');
    expect(label).toBeInTheDocument();
    expect(label.tagName.toLowerCase()).toBe('label');
    expect(label.getAttribute('for')).toBe('squash-message');
  });

  it('fires onmessagechange(value) on every keystroke (oninput)', async () => {
    const onmessagechange = vi.fn();
    render(SquashMessageEditor, {
      props: { message: 'Initial', onmessagechange, coAuthors: [] },
    });
    const textarea = screen.getByLabelText(/squash commit message/i);
    await fireEvent.input(textarea, { target: { value: 'Updated subject' } });
    expect(onmessagechange).toHaveBeenCalledTimes(1);
    expect(onmessagechange).toHaveBeenCalledWith('Updated subject');

    await fireEvent.input(textarea, { target: { value: 'Updated subject!' } });
    expect(onmessagechange).toHaveBeenCalledTimes(2);
    expect(onmessagechange).toHaveBeenLastCalledWith('Updated subject!');
  });

  it('composes <CoAuthorChipRow/> with the coAuthors prop', () => {
    render(SquashMessageEditor, {
      props: {
        message: 'subject',
        onmessagechange: () => {},
        coAuthors: [ALICE, BOB],
      },
    });
    const row = screen.getByTestId('coauthor-chip-row');
    expect(row).toBeInTheDocument();
    const chips = screen.getAllByTestId('coauthor-chip');
    expect(chips).toHaveLength(2);
    expect(row.textContent).toContain('Alice');
    expect(row.textContent).toContain('Bob');
  });

  it('does not render a chip row when coAuthors is empty', () => {
    render(SquashMessageEditor, {
      props: { message: 'subject', onmessagechange: () => {}, coAuthors: [] },
    });
    expect(screen.queryByTestId('coauthor-chip-row')).toBeNull();
  });

  it('updates the textarea in place when message prop changes (no remount)', async () => {
    const { rerender } = render(SquashMessageEditor, {
      props: { message: 'first', onmessagechange: () => {}, coAuthors: [] },
    });
    const before = screen.getByLabelText(/squash commit message/i) as HTMLTextAreaElement;
    expect(before.value).toBe('first');

    await rerender({ message: 'second', onmessagechange: () => {}, coAuthors: [] });

    const after = screen.getByLabelText(/squash commit message/i) as HTMLTextAreaElement;
    expect(after.value).toBe('second');
    // Same DOM node — identity preserved across prop updates.
    expect(after).toBe(before);
  });
});
