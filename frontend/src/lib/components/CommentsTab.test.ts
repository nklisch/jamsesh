import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import CommentsTab from './CommentsTab.svelte';
import type { components } from '$lib/api/types.gen';

type Comment = components['schemas']['Comment'];

// ── Module mocks ─────────────────────────────────────────────────────────────

const mockGET = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: { GET: (...args: unknown[]) => mockGET(...args) },
}));

vi.mock('$lib/auth.svelte', () => ({
  auth: { token: 'test-token', isAuthenticated: true, currentUser: null },
}));

const handlersByType = new Map<string, ((env: unknown) => void)[]>();
const mockSubscribe = vi.fn((sessionId: string, type: string, handler: (env: unknown) => void) => {
  if (!handlersByType.has(type)) handlersByType.set(type, []);
  handlersByType.get(type)!.push(handler);
  return () => {
    const arr = handlersByType.get(type);
    if (arr) handlersByType.set(type, arr.filter((h) => h !== handler));
  };
});

vi.mock('$lib/ws.svelte', () => ({
  subscribe: (...args: Parameters<typeof mockSubscribe>) => mockSubscribe(...args),
}));

// ── Fixtures ──────────────────────────────────────────────────────────────────

function makeComment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 'comment-1',
    session_id: 'sess-1',
    author_id: 'alice',
    author_kind: 'human',
    anchor: { commit_sha: 'abc1234', file_path: 'src/auth.ts', line_range: { start: 10, end: 10 } },
    body: 'This looks good but we should document the error path.',
    kind: 'question',
    created_at: new Date().toISOString(),
    ...overrides,
  };
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('CommentsTab', () => {
  beforeEach(() => {
    handlersByType.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('shows loading state on mount', () => {
    mockGET.mockReturnValue(new Promise(() => {})); // never resolves
    render(CommentsTab, { props: { orgId: 'org-1', sessionId: 'sess-1' } });
    expect(screen.getByText(/loading comments/i)).toBeInTheDocument();
  });

  it('shows empty state when no comments', async () => {
    mockGET.mockResolvedValue({ data: { items: [], next_cursor: null }, error: null });
    render(CommentsTab, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(screen.getByText(/no comments yet/i)).toBeInTheDocument();
    });
  });

  it('renders comments from API', async () => {
    const comment = makeComment({ body: 'Should this also clear the refresh token?' });
    mockGET.mockResolvedValue({ data: { items: [comment], next_cursor: null }, error: null });

    render(CommentsTab, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(screen.getByText('Should this also clear the refresh token?')).toBeInTheDocument();
    });
  });

  it('renders author for each comment', async () => {
    const comment = makeComment({ author_id: 'bob' });
    mockGET.mockResolvedValue({ data: { items: [comment], next_cursor: null }, error: null });

    render(CommentsTab, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(screen.getByText('@bob')).toBeInTheDocument();
    });
  });

  it('renders kind badge for each comment', async () => {
    const comment = makeComment({ kind: 'suggestion' });
    mockGET.mockResolvedValue({ data: { items: [comment], next_cursor: null }, error: null });

    render(CommentsTab, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(screen.getByText('suggestion')).toBeInTheDocument();
    });
  });

  it('renders file anchor for comments', async () => {
    const comment = makeComment({
      anchor: { commit_sha: 'abc123', file_path: 'docs/auth.md', line_range: { start: 16, end: 16 } },
    });
    mockGET.mockResolvedValue({ data: { items: [comment], next_cursor: null }, error: null });

    render(CommentsTab, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(screen.getByText('docs/auth.md:16')).toBeInTheDocument();
    });
  });

  it('marks resolved comments', async () => {
    const comment = makeComment({ resolved_at: new Date().toISOString(), resolved_by: 'alice' });
    mockGET.mockResolvedValue({ data: { items: [comment], next_cursor: null }, error: null });

    render(CommentsTab, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(document.querySelector('.comment-card.resolved')).toBeInTheDocument();
    });
  });

  it('subscribes to comment.added and comment.resolved', () => {
    mockGET.mockResolvedValue({ data: { items: [], next_cursor: null }, error: null });
    render(CommentsTab, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    const subscribedTypes = mockSubscribe.mock.calls.map((c) => c[1]);
    expect(subscribedTypes).toContain('comment.added');
    expect(subscribedTypes).toContain('comment.resolved');
  });

  it('shows error when API fails', async () => {
    mockGET.mockResolvedValue({ data: null, error: { message: 'Forbidden' } });
    render(CommentsTab, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(/failed to load comments/i);
    });
  });

  it('renders multiple comments', async () => {
    const comments = [
      makeComment({ id: 'c1', body: 'First comment', author_id: 'alice' }),
      makeComment({ id: 'c2', body: 'Second comment', author_id: 'bob' }),
    ];
    mockGET.mockResolvedValue({ data: { items: comments, next_cursor: null }, error: null });

    render(CommentsTab, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(screen.getByText('First comment')).toBeInTheDocument();
      expect(screen.getByText('Second comment')).toBeInTheDocument();
    });
  });
});
