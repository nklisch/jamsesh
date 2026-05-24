import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import SessionViewShell from './SessionViewShell.svelte';
import type { components } from '$lib/api/types.gen';

type Session = components['schemas']['Session'];
type PlaygroundSessionSummary = components['schemas']['PlaygroundSessionSummary'];

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
    playgroundContext: null,
  },
}));

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  current: { name: 'session-view', params: { orgId: 'org-1', sessionId: 'sess-1' } },
  navigate: (...args: unknown[]) => mockNavigate(...args),
}));

// WS mock: captures (sessionId, type) → handler so tests can fire events.
// subscribe returns an unsubscribe no-op consistent with the real API.
type WsHandler = (env: { type: string; [key: string]: unknown }) => void;
const wsHandlers = new Map<string, WsHandler>();
const mockSubscribe = vi.fn().mockImplementation(
  (sessionId: string, type: string, handler: WsHandler) => {
    wsHandlers.set(`${sessionId}:${type}`, handler);
    return () => wsHandlers.delete(`${sessionId}:${type}`);
  },
);
vi.mock('$lib/ws.svelte', () => ({
  subscribe: (...args: unknown[]) => mockSubscribe(...args),
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

function makePlaygroundSession(
  overrides: Partial<PlaygroundSessionSummary> = {},
): PlaygroundSessionSummary {
  const now = new Date();
  return {
    id: 'pg-sess-1',
    org_id: 'org_playground',
    name: 'playground-pg01',
    goal: 'Quick prototype',
    scope: JSON.stringify(['**']),
    status: 'active',
    created_at: now.toISOString(),
    hard_cap_at: new Date(now.getTime() + 24 * 60 * 60 * 1000).toISOString(), // 24h
    idle_timeout_at: new Date(now.getTime() + 30 * 60 * 1000).toISOString(),  // 30m
    members_count: 1,
    ...overrides,
  };
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('SessionViewShell', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    wsHandlers.clear();
    localStorage.clear();
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
      writable: true,
      configurable: true,
    });
    // Default GET mock: durable session data
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

  it('renders AttachHelpLink in the SessionViewShell chrome', () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });
    expect(screen.getByRole('button', { name: /setup help/i })).toBeInTheDocument();
  });

  it('forwards the sessionId to AttachHelpLink and the walkthrough dialog displays it', async () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-42' } });

    const helpBtn = screen.getByRole('button', { name: /setup help/i });
    await fireEvent.click(helpBtn);

    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: /attach claude code/i })).toBeInTheDocument();
    });

    // The walkthrough derives joinCmd = `/jamsesh:join <sessionId>` and renders it in the CC pane.
    expect(screen.getByText('/jamsesh:join sess-42')).toBeInTheDocument();
  });

  // ── Playground branch tests ─────────────────────────────────────────────────

  describe('playground branch (orgId === org_playground)', () => {
    const pgSessionId = 'pg-sess-1';

    beforeEach(() => {
      // Playground GET mock: return PlaygroundSessionSummary for the playground endpoint.
      mockGET.mockImplementation((path: string) => {
        if (path.includes('/refs')) {
          return Promise.resolve({ data: { refs: [] }, error: null });
        }
        if (path.includes('/comments')) {
          return Promise.resolve({ data: { items: [], next_cursor: null }, error: null });
        }
        if (path === '/api/playground/sessions/{id}') {
          return Promise.resolve({ data: makePlaygroundSession(), error: null });
        }
        return Promise.resolve({ data: null, error: { message: 'unexpected endpoint' } });
      });
    });

    it('calls GET /api/playground/sessions/{id} (not the orgs endpoint) for playground sessions', async () => {
      render(SessionViewShell, { props: { orgId: 'org_playground', sessionId: pgSessionId } });

      await waitFor(() => {
        expect(mockGET).toHaveBeenCalledWith(
          '/api/playground/sessions/{id}',
          expect.objectContaining({ params: { path: { id: pgSessionId } } }),
        );
      });
      // The org-scoped endpoint should NOT be called for a playground session.
      expect(mockGET).not.toHaveBeenCalledWith(
        '/api/orgs/{orgID}/sessions/{sessionID}',
        expect.anything(),
      );
    });

    it('renders PlaygroundChip when orgId is org_playground', async () => {
      render(SessionViewShell, { props: { orgId: 'org_playground', sessionId: pgSessionId } });

      await waitFor(() => {
        expect(screen.getByLabelText('Playground session')).toBeInTheDocument();
      });
    });

    it('subscribes to playground.destruction_warning and session.ended WS events', async () => {
      render(SessionViewShell, { props: { orgId: 'org_playground', sessionId: pgSessionId } });

      await waitFor(() => {
        expect(screen.getByLabelText('Playground session')).toBeInTheDocument();
      });

      // mountSubscriptions() calls subscribe() twice — once per event type.
      const subscribedTypes = mockSubscribe.mock.calls
        .filter((call) => (call[0] as string) === pgSessionId)
        .map((call) => call[1] as string);

      expect(subscribedTypes).toContain('playground.destruction_warning');
      expect(subscribedTypes).toContain('session.ended');
      // Must NOT subscribe to the incorrect legacy event names.
      expect(subscribedTypes).not.toContain('playground.activity_reset');
      expect(subscribedTypes).not.toContain('session.destroyed');
    });

    it('navigates to /playground/s/:id/ended when session.ended WS event fires', async () => {
      render(SessionViewShell, { props: { orgId: 'org_playground', sessionId: pgSessionId } });

      await waitFor(() => {
        expect(screen.getByLabelText('Playground session')).toBeInTheDocument();
      });

      // Fire the session.ended WS event via the captured handler.
      const endedHandler = wsHandlers.get(`${pgSessionId}:session.ended`);
      expect(endedHandler).toBeDefined();
      endedHandler!({ type: 'session.ended', reason: 'timeout' });

      expect(mockNavigate).toHaveBeenCalledWith(`/playground/s/${pgSessionId}/ended`);
    });

    it('updates idle timer when playground.destruction_warning fires with reason=idle_timeout', async () => {
      render(SessionViewShell, { props: { orgId: 'org_playground', sessionId: pgSessionId } });

      await waitFor(() => {
        expect(screen.getByLabelText('Playground session')).toBeInTheDocument();
      });

      // Confirm the countdown badge is present (means hard_cap_at / idle_timeout_at were seeded).
      await waitFor(() => {
        expect(screen.getByTestId('countdown-badge')).toBeInTheDocument();
      });

      // Fire a destruction_warning for idle_timeout with 3 minutes remaining.
      const warningHandler = wsHandlers.get(`${pgSessionId}:playground.destruction_warning`);
      expect(warningHandler).toBeDefined();

      const threeMinutesFromNow = new Date(Date.now() + 3 * 60 * 1000);
      warningHandler!({
        type: 'playground.destruction_warning',
        reason: 'idle_timeout',
        ends_at: threeMinutesFromNow.toISOString(),
        remaining_seconds: 180,
        session_id: pgSessionId,
      });

      // After the idle warning fires with < 5 min, the countdown badge should
      // go urgent (CountdownBadge's isUrgent fires when idleRemainingMs < 5 min).
      await waitFor(() => {
        const badge = screen.getByTestId('countdown-badge');
        expect(badge).toHaveClass('urgent');
      });
    });

    it('updates hard-cap timer when playground.destruction_warning fires with reason=hard_cap', async () => {
      render(SessionViewShell, { props: { orgId: 'org_playground', sessionId: pgSessionId } });

      await waitFor(() => {
        expect(screen.getByLabelText('Playground session')).toBeInTheDocument();
      });

      // Confirm the countdown badge is present (hard_cap_at / idle_timeout_at were seeded).
      await waitFor(() => {
        expect(screen.getByTestId('countdown-badge')).toBeInTheDocument();
      });

      const warningHandler = wsHandlers.get(`${pgSessionId}:playground.destruction_warning`);
      expect(warningHandler).toBeDefined();

      // Fire a destruction_warning with reason=hard_cap and a deadline 3 minutes out.
      // This is the imminent-hard-cap case: the server fires a warning because the
      // session will be destroyed by the hard-cap timer, not the idle timer.
      const threeMinutesFromNow = new Date(Date.now() + 3 * 60 * 1000);
      warningHandler!({
        type: 'playground.destruction_warning',
        reason: 'hard_cap',
        ends_at: threeMinutesFromNow.toISOString(),
        remaining_seconds: 180,
        session_id: pgSessionId,
      });

      // The countdown badge becomes urgent because hardCapRemainingMs < WARN_THRESHOLD_MS.
      // The idle timer is unchanged (still 24h from fixture); only the hard-cap branch fires.
      await waitFor(() => {
        const badge = screen.getByTestId('countdown-badge');
        expect(badge).toHaveClass('urgent');
      });
    });

    it('renders the playground session name in the breadcrumb', async () => {
      render(SessionViewShell, { props: { orgId: 'org_playground', sessionId: pgSessionId } });

      await waitFor(() => {
        expect(screen.getByText('playground-pg01', { selector: '.here' })).toBeInTheDocument();
      });
    });

  });

  // ── Regression: durable-session path is unchanged by playground additions ───

  it('does NOT render PlaygroundChip for durable sessions', async () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Auth design refresh' })).toBeInTheDocument();
    });
    expect(screen.queryByLabelText('Playground session')).toBeNull();
  });

  it('durable session still renders org breadcrumb link', async () => {
    render(SessionViewShell, { props: { orgId: 'org-1', sessionId: 'sess-1' } });

    await waitFor(() => {
      // The org name should appear as a clickable breadcrumb for durable sessions.
      expect(screen.getByRole('button', { name: 'org-1' })).toBeInTheDocument();
    });
  });
});
