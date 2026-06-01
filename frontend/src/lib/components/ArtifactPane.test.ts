import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import { fireEvent } from '@testing-library/svelte';
import ArtifactPane from './ArtifactPane.svelte';

// ── Module mocks ─────────────────────────────────────────────────────────────

const mockGET = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: { GET: (...args: unknown[]) => mockGET(...args) },
}));

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderPane(
  overrides: {
    selectedSha?: string | null;
    selectedPath?: string | null;
    onrangeselect?: (r: { start: number; end: number } | null) => void;
  } = {},
) {
  return render(ArtifactPane, {
    props: {
      orgId: 'org-1',
      sessionId: 'sess-1',
      selectedSha: overrides.selectedSha ?? null,
      selectedPath: overrides.selectedPath ?? null,
      onrangeselect: overrides.onrangeselect,
    },
  });
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('ArtifactPane', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('shows empty state when no sha/path selected', () => {
    renderPane();
    expect(screen.getByText(/Select a commit and file/i)).toBeInTheDocument();
  });

  it('shows loading state while fetching', () => {
    mockGET.mockReturnValue(new Promise(() => {})); // never resolves
    renderPane({ selectedSha: 'abc123', selectedPath: 'src/foo.ts' });
    expect(screen.getByText(/loading file/i)).toBeInTheDocument();
  });

  it('renders file content with line numbers', async () => {
    mockGET.mockResolvedValue({
      data: { content: 'line one\nline two\nline three', mime: 'text/plain', is_binary: false },
      error: null,
      response: new Response(null, { status: 200 }),
    });

    renderPane({ selectedSha: 'abc123', selectedPath: 'src/foo.ts' });

    await waitFor(() => {
      expect(screen.getByText('line one')).toBeInTheDocument();
      expect(screen.getByText('line two')).toBeInTheDocument();
    });
    expect(mockGET).toHaveBeenCalledWith(
      '/api/orgs/{orgID}/sessions/{sessionID}/files',
      expect.objectContaining({
        params: {
          path: { orgID: 'org-1', sessionID: 'sess-1' },
          query: { commit: 'abc123', path: 'src/foo.ts' },
        },
      }),
    );
  });

  it('renders line numbers', async () => {
    mockGET.mockResolvedValue({
      data: { content: 'first\nsecond', mime: 'text/plain', is_binary: false },
      error: null,
      response: new Response(null, { status: 200 }),
    });

    renderPane({ selectedSha: 'abc123', selectedPath: 'src/foo.ts' });

    await waitFor(() => {
      expect(screen.getByLabelText('Line 1')).toBeInTheDocument();
      expect(screen.getByLabelText('Line 2')).toBeInTheDocument();
    });
  });

  it('shows binary placeholder for binary files', async () => {
    mockGET.mockResolvedValue({
      data: { content: '', mime: 'application/octet-stream', is_binary: true },
      error: null,
      response: new Response(null, { status: 200 }),
    });

    renderPane({ selectedSha: 'abc123', selectedPath: 'image.png' });

    await waitFor(() => {
      expect(screen.getByText(/binary file/i)).toBeInTheDocument();
      expect(screen.getByText(/application\/octet-stream/i)).toBeInTheDocument();
    });
  });

  it('shows error when fetch fails', async () => {
    mockGET.mockResolvedValue({
      data: null,
      error: { message: 'HTTP 404' },
      response: new Response(null, { status: 404 }),
    });

    renderPane({ selectedSha: 'abc123', selectedPath: 'missing.ts' });

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });
  });

  it('shows error when fetch throws', async () => {
    mockGET.mockRejectedValue(new Error('network error'));

    renderPane({ selectedSha: 'abc123', selectedPath: 'foo.ts' });

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });
  });

  it('selects a line on click and calls onrangeselect', async () => {
    mockGET.mockResolvedValue({
      data: { content: 'alpha\nbeta\ngamma', mime: 'text/plain', is_binary: false },
      error: null,
      response: new Response(null, { status: 200 }),
    });

    const onrangeselect = vi.fn();
    renderPane({ selectedSha: 'abc123', selectedPath: 'foo.ts', onrangeselect });

    await waitFor(() => {
      expect(screen.getByLabelText('Line 1')).toBeInTheDocument();
    });

    const line1 = screen.getByLabelText('Line 1');
    await fireEvent.click(line1);

    expect(onrangeselect).toHaveBeenCalledWith({ start: 1, end: 1 });
  });

  it('shows sha badge in file header', async () => {
    mockGET.mockResolvedValue({
      data: { content: 'hello', mime: 'text/plain', is_binary: false },
      error: null,
      response: new Response(null, { status: 200 }),
    });

    renderPane({ selectedSha: 'abc1234567890', selectedPath: 'foo.ts' });

    await waitFor(() => {
      expect(screen.getByText('abc1234')).toBeInTheDocument();
    });
  });

  it('displays file path in header', async () => {
    mockGET.mockResolvedValue({
      data: { content: 'code', mime: 'text/plain', is_binary: false },
      error: null,
      response: new Response(null, { status: 200 }),
    });

    renderPane({ selectedSha: 'abc123', selectedPath: 'src/components/Foo.svelte' });

    await waitFor(() => {
      expect(screen.getByTitle('src/components/Foo.svelte')).toBeInTheDocument();
    });
  });

  // ── Regression: stale-fetch abort guard (Unit 2) ──────────────────────────

  it('stale-response is not written: selecting B while A is in-flight shows B content', async () => {
    // Deferred promise for file A (slow response).
    let resolveA!: (r: unknown) => void;
    const fetchA = new Promise<unknown>((res) => { resolveA = res; });

    // File B resolves immediately.
    const fetchB = Promise.resolve({
      data: { content: 'content-of-B', mime: 'text/plain', is_binary: false },
      error: null,
      response: new Response(null, { status: 200 }),
    });

    mockGET
      .mockReturnValueOnce(fetchA)   // first call: file A
      .mockReturnValueOnce(fetchB);  // second call: file B

    const { rerender } = renderPane({ selectedSha: 'sha-a', selectedPath: 'a.ts' });

    // Kick off fetch B by changing the selected path — this causes the $effect
    // to re-run and abort the A request.
    await rerender({
      orgId: 'org-1',
      sessionId: 'sess-1',
      selectedSha: 'sha-b',
      selectedPath: 'b.ts',
    });

    // Wait for B to render.
    await waitFor(() => {
      expect(screen.getByText('content-of-B')).toBeInTheDocument();
    });

    // Now resolve A's deferred promise (stale). Because the signal was aborted,
    // neither content nor error should be written.
    resolveA({
      data: { content: 'content-of-A', mime: 'text/plain', is_binary: false },
      error: null,
      response: new Response(null, { status: 200 }),
    });

    // Give microtasks a tick to flush; A's then-callback should short-circuit.
    await new Promise((r) => setTimeout(r, 0));

    // B's content should still be visible; A's content must not have overwritten it.
    expect(screen.queryByText('content-of-A')).not.toBeInTheDocument();
    expect(screen.getByText('content-of-B')).toBeInTheDocument();
  });
});
