import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import { fireEvent } from '@testing-library/svelte';
import ArtifactPane from './ArtifactPane.svelte';

// ── Module mocks ─────────────────────────────────────────────────────────────

vi.mock('$lib/auth.svelte', () => ({
  auth: { token: 'test-token', isAuthenticated: true, currentUser: null },
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
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it('shows empty state when no sha/path selected', () => {
    renderPane();
    expect(screen.getByText(/Select a commit and file/i)).toBeInTheDocument();
  });

  it('shows loading state while fetching', () => {
    vi.mocked(fetch).mockReturnValue(new Promise(() => {})); // never resolves
    renderPane({ selectedSha: 'abc123', selectedPath: 'src/foo.ts' });
    expect(screen.getByText(/loading file/i)).toBeInTheDocument();
  });

  it('renders file content with line numbers', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({ content: 'line one\nline two\nline three', mime: 'text/plain', is_binary: false }),
    } as Response);

    renderPane({ selectedSha: 'abc123', selectedPath: 'src/foo.ts' });

    await waitFor(() => {
      expect(screen.getByText('line one')).toBeInTheDocument();
      expect(screen.getByText('line two')).toBeInTheDocument();
    });
  });

  it('renders line numbers', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({ content: 'first\nsecond', mime: 'text/plain', is_binary: false }),
    } as Response);

    renderPane({ selectedSha: 'abc123', selectedPath: 'src/foo.ts' });

    await waitFor(() => {
      expect(screen.getByLabelText('Line 1')).toBeInTheDocument();
      expect(screen.getByLabelText('Line 2')).toBeInTheDocument();
    });
  });

  it('shows binary placeholder for binary files', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({ content: '', mime: 'application/octet-stream', is_binary: true }),
    } as Response);

    renderPane({ selectedSha: 'abc123', selectedPath: 'image.png' });

    await waitFor(() => {
      expect(screen.getByText(/binary file/i)).toBeInTheDocument();
      expect(screen.getByText(/application\/octet-stream/i)).toBeInTheDocument();
    });
  });

  it('shows error when fetch fails', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: false,
      status: 404,
    } as Response);

    renderPane({ selectedSha: 'abc123', selectedPath: 'missing.ts' });

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });
  });

  it('shows error when fetch throws', async () => {
    vi.mocked(fetch).mockRejectedValue(new Error('network error'));

    renderPane({ selectedSha: 'abc123', selectedPath: 'foo.ts' });

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });
  });

  it('selects a line on click and calls onrangeselect', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({ content: 'alpha\nbeta\ngamma', mime: 'text/plain', is_binary: false }),
    } as Response);

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
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({ content: 'hello', mime: 'text/plain', is_binary: false }),
    } as Response);

    renderPane({ selectedSha: 'abc1234567890', selectedPath: 'foo.ts' });

    await waitFor(() => {
      expect(screen.getByText('abc1234')).toBeInTheDocument();
    });
  });

  it('displays file path in header', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({ content: 'code', mime: 'text/plain', is_binary: false }),
    } as Response);

    renderPane({ selectedSha: 'abc123', selectedPath: 'src/components/Foo.svelte' });

    await waitFor(() => {
      expect(screen.getByTitle('src/components/Foo.svelte')).toBeInTheDocument();
    });
  });
});
