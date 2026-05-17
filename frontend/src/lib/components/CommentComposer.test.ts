import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import { fireEvent } from '@testing-library/svelte';
import CommentComposer from './CommentComposer.svelte';

// ── Module mocks ─────────────────────────────────────────────────────────────

const mockPOST = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: { POST: (...args: unknown[]) => mockPOST(...args) },
}));

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderComposer(
  overrides: {
    anchorFilePath?: string | null;
    anchorLineStart?: number | null;
    anchorLineEnd?: number | null;
    onsubmit?: (c: unknown) => void;
    oncancel?: () => void;
  } = {},
) {
  return render(CommentComposer, {
    props: {
      orgId: 'org-1',
      sessionId: 'sess-1',
      anchorCommitSha: 'abc1234567890',
      anchorFilePath: overrides.anchorFilePath ?? null,
      anchorLineStart: overrides.anchorLineStart ?? null,
      anchorLineEnd: overrides.anchorLineEnd ?? null,
      onsubmit: overrides.onsubmit,
      oncancel: overrides.oncancel,
    },
  });
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('CommentComposer', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders composer with Add comment title', () => {
    renderComposer();
    expect(screen.getByText('Add comment')).toBeInTheDocument();
  });

  it('shows truncated commit sha as anchor when no file path', () => {
    renderComposer();
    expect(screen.getByText('abc1234')).toBeInTheDocument();
  });

  it('shows file path as anchor when provided', () => {
    renderComposer({ anchorFilePath: 'src/foo.ts' });
    expect(screen.getByText(/src\/foo\.ts/)).toBeInTheDocument();
  });

  it('shows file path with line number', () => {
    renderComposer({ anchorFilePath: 'src/foo.ts', anchorLineStart: 5 });
    expect(screen.getByText(/src\/foo\.ts:5/)).toBeInTheDocument();
  });

  it('shows line range when start and end differ', () => {
    renderComposer({ anchorFilePath: 'src/foo.ts', anchorLineStart: 5, anchorLineEnd: 10 });
    expect(screen.getByText(/5/)).toBeInTheDocument();
    expect(screen.getByText(/10/)).toBeInTheDocument();
  });

  it('renders kind select with default question', () => {
    renderComposer();
    const select = screen.getByLabelText(/kind/i) as HTMLSelectElement;
    expect(select.value).toBe('question');
  });

  it('renders body textarea', () => {
    renderComposer();
    expect(screen.getByLabelText(/comment body/i)).toBeInTheDocument();
  });

  it('submit button is disabled when body is empty', () => {
    renderComposer();
    const btn = screen.getByRole('button', { name: /post comment/i });
    expect(btn).toBeDisabled();
  });

  it('calls oncancel when close button clicked', async () => {
    const oncancel = vi.fn();
    renderComposer({ oncancel });
    await fireEvent.click(screen.getByLabelText(/close comment composer/i));
    expect(oncancel).toHaveBeenCalledTimes(1);
  });

  it('calls oncancel when Cancel button clicked', async () => {
    const oncancel = vi.fn();
    renderComposer({ oncancel });
    await fireEvent.click(screen.getByRole('button', { name: /^cancel$/i }));
    expect(oncancel).toHaveBeenCalledTimes(1);
  });

  it('submits and calls onsubmit with returned comment', async () => {
    const fakeComment = { id: 'c-1', body: 'hello' };
    mockPOST.mockResolvedValue({ data: fakeComment, error: null });
    const onsubmit = vi.fn();

    renderComposer({ onsubmit });

    const textarea = screen.getByLabelText(/comment body/i);
    await fireEvent.input(textarea, { target: { value: 'hello' } });

    const btn = screen.getByRole('button', { name: /post comment/i });
    await fireEvent.click(btn);

    await waitFor(() => {
      expect(mockPOST).toHaveBeenCalledWith(
        '/api/orgs/{orgID}/sessions/{sessionID}/comments',
        expect.objectContaining({
          params: { path: { orgID: 'org-1', sessionID: 'sess-1' } },
          body: expect.objectContaining({ anchor_commit_sha: 'abc1234567890', body: 'hello', kind: 'question' }),
        }),
      );
      expect(onsubmit).toHaveBeenCalledWith(fakeComment);
    });
  });

  it('shows error when POST fails', async () => {
    mockPOST.mockResolvedValue({ data: null, error: { message: 'bad' } });
    renderComposer();

    const textarea = screen.getByLabelText(/comment body/i);
    await fireEvent.input(textarea, { target: { value: 'hello' } });
    await fireEvent.click(screen.getByRole('button', { name: /post comment/i }));

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
      expect(screen.getByRole('alert')).toHaveTextContent(/failed to post/i);
    });
  });

  it('includes anchor file path and line info in request body', async () => {
    const fakeComment = { id: 'c-2', body: 'note' };
    mockPOST.mockResolvedValue({ data: fakeComment, error: null });

    renderComposer({ anchorFilePath: 'src/bar.ts', anchorLineStart: 3, anchorLineEnd: 7 });

    const textarea = screen.getByLabelText(/comment body/i);
    await fireEvent.input(textarea, { target: { value: 'note' } });
    await fireEvent.click(screen.getByRole('button', { name: /post comment/i }));

    await waitFor(() => {
      expect(mockPOST).toHaveBeenCalledWith(
        '/api/orgs/{orgID}/sessions/{sessionID}/comments',
        expect.objectContaining({
          body: expect.objectContaining({
            anchor_file_path: 'src/bar.ts',
            anchor_line_start: 3,
            anchor_line_end: 7,
          }),
        }),
      );
    });
  });
});
