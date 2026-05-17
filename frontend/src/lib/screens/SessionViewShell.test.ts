import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import SessionViewShell from './SessionViewShell.svelte';
import type { components } from '$lib/api/types.gen';

type Session = components['schemas']['Session'];

// ── Module mocks ─────────────────────────────────────────────────────────────

const mockGET = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: { GET: (...args: unknown[]) => mockGET(...args) },
}));

vi.mock('$lib/auth.svelte', () => ({
  auth: {
    currentUser: { id: 'user-1', email: 'test@example.com', displayName: 'Test User' },
    isAuthenticated: true,
    token: 'test-token',
  },
}));

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  current: { name: 'session-view', params: { orgId: 'org-1', sessionId: 'sess-1' } },
  navigate: (...args: unknown[]) => mockNavigate(...args),
}));

vi.mock('$lib/ws.svelte', () => ({
  subscribe: vi.fn().mockReturnValue(() => {}),
  // WsStatusBanner (mounted inside SessionViewShell) reads from this
  // rune store; the test doesn't exercise reconnect behavior, so a
  // constant `null` (no active subscription) keeps the banner absent.
  wsStatus: { for: () => null },
}));

// ── Fixtures ──────────────────────────────────────────────────────────────────

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: 'sess-1',
    org_id: 'org-1',
    name: 'Auth design refresh',
    goal: 'Tighten the session-token flow',
    scope: JSON.stringify(['docs/auth/**']),
    default_mode: 'sync',
    status: 'active',
    created_at: new Date().toISOString(),
    members: [
      { account_id: 'user-1', role: 'creator', joined_at: new Date().toISOString() },
      { account_id: 'user-2', role: 'member', joined_at: new Date().toISOString() },
    ],
    ...overrides,
  };
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('SessionViewShell', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default GET mock: session data
    mockGET.mockImplementation((path: string) => {
      if (path.includes('/refs')) {
        return Promise.resolve({ data: { refs: [] }, error: null });
      }
      if (path.includes('/comments')) {
        return Promise.resolve({ data: { items: [], next_cursor: null }, error: null });
      }
      return Promise.resolve({ data: makeSession(), error: null });
    });
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders the session name', async () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Auth design refresh' })).toBeInTheDocument();
    });
  });

  it('renders the session goal', async () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(screen.getByText('Tighten the session-token flow')).toBeInTheDocument();
    });
  });

  it('renders the wordmark in app chrome', () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });
    expect(screen.getByLabelText('jamsesh')).toBeInTheDocument();
  });

  it('renders a breadcrumb with org and session links', async () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(screen.getByText('Auth design refresh', { selector: '.here' })).toBeInTheDocument();
    });
  });

  it('cycles tree state on ⇔ button click', async () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(document.querySelector('.top')).toBeInTheDocument();
    });

    const top = document.querySelector('.top')!;
    expect(top).toHaveClass('tree-collapsed');

    const cycleBtn = screen.getByRole('button', { name: /cycle tree width/i });
    await fireEvent.click(cycleBtn);

    expect(top).toHaveClass('tree-expanded');

    await fireEvent.click(cycleBtn);
    expect(top).toHaveClass('tree-wide');

    await fireEvent.click(cycleBtn);
    expect(top).toHaveClass('tree-collapsed');
  });

  it('expands bottom panel when tab is clicked', async () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => expect(screen.queryByText(/tighten/i)).toBeInTheDocument());

    const bottom = document.querySelector('.bottom')!;
    expect(bottom).not.toHaveClass('expanded');

    const activityTab = screen.getByRole('tab', { name: /activity/i });
    await fireEvent.click(activityTab);

    expect(bottom).toHaveClass('expanded');
  });

  it('switches between Activity and Comments tabs', async () => {
    mockGET.mockImplementation((path: string) => {
      if (path.includes('/refs')) {
        return Promise.resolve({ data: { refs: [] }, error: null });
      }
      if (path.includes('/comments')) {
        return Promise.resolve({ data: { items: [], next_cursor: null }, error: null });
      }
      return Promise.resolve({ data: makeSession(), error: null });
    });

    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => expect(screen.queryByText(/tighten/i)).toBeInTheDocument());

    const activityTab = screen.getByRole('tab', { name: /activity/i });
    const commentsTab = screen.getByRole('tab', { name: /comments/i });

    await fireEvent.click(activityTab);
    expect(activityTab).toHaveAttribute('aria-selected', 'true');
    expect(commentsTab).toHaveAttribute('aria-selected', 'false');

    await fireEvent.click(commentsTab);
    expect(commentsTab).toHaveAttribute('aria-selected', 'true');
    expect(activityTab).toHaveAttribute('aria-selected', 'false');
  });

  it('renders the artifact slot with data-selected-sha', async () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      const slot = document.querySelector('[data-selected-sha]');
      expect(slot).toBeInTheDocument();
    });
  });

  it('navigates to the finalize route when the Finalize header button is clicked', async () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      const btn = screen.getByRole('button', { name: /finalize session/i });
      expect(btn).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole('button', { name: /finalize session/i }));
    expect(mockNavigate).toHaveBeenCalledWith('/orgs/org-1/sessions/sess-1/finalize');
  });

  it('navigates to sessions list when breadcrumb org link is clicked', async () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      const orgLink = screen.getByRole('button', { name: 'org-1' });
      expect(orgLink).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole('button', { name: 'org-1' }));
    expect(mockNavigate).toHaveBeenCalledWith('/orgs/org-1/sessions');
  });

  it('shows error when session fails to load', async () => {
    mockGET.mockResolvedValue({ data: null, error: { message: 'Not found' } });

    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'bad-id' } });

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(/failed to load session/i);
    });
  });

  it('shows loading state initially', () => {
    mockGET.mockReturnValue(new Promise(() => {})); // never resolves
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });
    expect(screen.getByText(/loading session/i)).toBeInTheDocument();
  });
});
