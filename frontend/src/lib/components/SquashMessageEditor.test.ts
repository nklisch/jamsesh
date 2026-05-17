import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import SquashMessageEditor from './SquashMessageEditor.svelte';

// Story 1 ships a minimum-viable editor: textarea bound to `message`
// + `onmessagechange`. The full implementation lands in story 2.

describe('SquashMessageEditor (story-1 placeholder)', () => {
  it('renders the message prop in a textarea', () => {
    render(SquashMessageEditor, {
      props: { message: 'Hello squash', onmessagechange: () => {} },
    });
    const textarea = screen.getByLabelText(/squash commit message/i) as HTMLTextAreaElement;
    expect(textarea.value).toBe('Hello squash');
  });

  it('fires onmessagechange on input', async () => {
    const onmessagechange = vi.fn();
    render(SquashMessageEditor, {
      props: { message: 'Initial', onmessagechange },
    });
    const textarea = screen.getByLabelText(/squash commit message/i) as HTMLTextAreaElement;
    await fireEvent.input(textarea, { target: { value: 'Updated' } });
    expect(onmessagechange).toHaveBeenCalledWith('Updated');
  });

  it('renders without crashing when coAuthors is omitted (defaults to empty)', () => {
    render(SquashMessageEditor, {
      props: { message: '', onmessagechange: () => {} },
    });
    expect(screen.getByLabelText(/squash commit message/i)).toBeInTheDocument();
  });
});
