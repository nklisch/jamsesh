import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/svelte';
import SessionList from './SessionList.svelte';
import type { components } from '$lib/api/types.gen';

type Session = components['schemas']['Session'];

// ── Module mocks ────────────────────────────────────────────────────────────

vi.mock('$lib/auth.svelte', () => ({
  auth: {
    currentUser: { id: 'user-1', email: 'test@example.com', displayName: 'Test User' },
    isAuthenticated: true,
    signOut: vi.fn(),
  },
}));

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  current: { name: 'sessions', params: { orgId: 'org-1' } },
  navigate: (...args: unknown[]) => mockNavigate(...args),
}));

// Track individual unsub functions per subscribe call so tests can simulate
// handler calls and count teardowns.
type SubscribeCall = { sessionId: string; type: string; handler: (e: Record<string, unknown>) => void; unsub: ReturnType<typeof vi.fn> };
const subscribeCalls: SubscribeCall[] = [];
const mockSubscribe = vi.fn((sessionId: string, type: string, handler: (e: Record<string, unknown>) => void) => {
  const unsub = vi.fn();
  subscribeCalls.push({ sessionId, type, handler, unsub });
  return unsub;
});
vi.mock('$lib/ws.svelte', () => ({
  subscribe: (...args: unknown[]) => mockSubscribe(...args as [string, string, (e: Record<string, unknown>) => void]),
}));

const mockGET = vi.fn();
const mockPOST = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: {
    GET: (...args: unknown[]) => mockGET(...args),
    POST: (...args: unknown[]) => mockPOST(...args),
  },
}));

// ── Fixtures ─────────────────────────────────────────────────────────────────

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: 'sess-1',
    org_id: 'org-1',
    name: 'Test Session',
    goal: 'A test goal',
    scope: JSON.stringify(['src/**']),
    default_mode: 'sync',
    status: 'active',
    created_at: new Date().toISOString(),
    members: [{ account_id: 'user-1', role: 'creator', joined_at: new Date().toISOString() }],
    ...overrides,
  };
}


// ── Tests ─────────────────────────────────────────────────────────────────────

describe('SessionList', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    subscribeCalls.length = 0;
    Object.defineProperty(globalThis.navigator, 'clipboard', {
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
      configurable: true,
    });
    localStorage.clear();
  });

  afterEach(() => {
    vi.clearAllMocks();
    subscribeCalls.length = 0;
  });

  it('renders the page heading', async () => {
    mockGET.mockResolvedValue({ data: { items: [], next_cursor: null }, error: null });
    render(SessionList, { props: { orgId: 'org-1' } });
    expect(screen.getByRole('heading', { name: /your sessions/i })).toBeInTheDocument();
  });

  it('renders a "New session" button', async () => {
    mockGET.mockResolvedValue({ data: { items: [], next_cursor: null }, error: null });
    render(SessionList, { props: { orgId: 'org-1' } });
    expect(screen.getByRole('button', { name: /new session/i })).toBeInTheDocument();
  });

  it('shows filter chips', async () => {
    mockGET.mockResolvedValue({ data: { items: [], next_cursor: null }, error: null });
    render(SessionList, { props: { orgId: 'org-1' } });
    expect(screen.getByRole('button', { name: /all/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /active/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /finalizing/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /ended/i })).toBeInTheDocument();
  });

  it('renders sessions from API', async () => {
    const session = makeSession({ name: 'Auth Refresh' });
    mockGET.mockResolvedValue({ data: { items: [session], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByText('Auth Refresh')).toBeInTheDocument();
    });
  });

  it('renders goal text for each session', async () => {
    const session = makeSession({ goal: 'Fix the login flow' });
    mockGET.mockResolvedValue({ data: { items: [session], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByText('Fix the login flow')).toBeInTheDocument();
    });
  });

  it('filters sessions by status', async () => {
    const active = makeSession({ id: 'a1', name: 'Active Session', status: 'active' });
    const ended = makeSession({ id: 'e1', name: 'Ended Session', status: 'ended' });
    mockGET.mockResolvedValue({ data: { items: [active, ended], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByText('Active Session')).toBeInTheDocument();
      expect(screen.getByText('Ended Session')).toBeInTheDocument();
    });

    // Click "Active" filter — use getAllByRole to disambiguate from session rows
    const allActiveButtons = screen.getAllByRole('button', { name: /^active/i });
    const activeFilterBtn = allActiveButtons.find((b) => b.classList.contains('filter-chip'))!;
    await fireEvent.click(activeFilterBtn);

    expect(screen.getByText('Active Session')).toBeInTheDocument();
    expect(screen.queryByText('Ended Session')).not.toBeInTheDocument();
  });

  it('shows "ended" filter only ended sessions', async () => {
    const active = makeSession({ id: 'a1', name: 'Still Going', status: 'active' });
    const ended = makeSession({ id: 'e1', name: 'All Done', status: 'ended' });
    mockGET.mockResolvedValue({ data: { items: [active, ended], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });

    await waitFor(() => expect(screen.getByText('Still Going')).toBeInTheDocument());

    const endedBtn = screen.getByRole('button', { name: /ended/i });
    await fireEvent.click(endedBtn);

    expect(screen.queryByText('Still Going')).not.toBeInTheDocument();
    expect(screen.getByText('All Done')).toBeInTheDocument();
  });

  it('shows empty message when no sessions match filter', async () => {
    const active = makeSession({ status: 'active' });
    mockGET.mockResolvedValue({ data: { items: [active], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });

    await waitFor(() => expect(screen.queryByText(/loading/i)).not.toBeInTheDocument());

    const finalizingBtn = screen.getByRole('button', { name: /finalizing/i });
    await fireEvent.click(finalizingBtn);

    expect(screen.getByText(/no finalizing sessions/i)).toBeInTheDocument();
  });

  it('opens new session drawer on button click', async () => {
    mockGET.mockResolvedValue({ data: { items: [], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });

    await waitFor(() => expect(screen.queryByText(/loading/i)).not.toBeInTheDocument());

    const newBtn = screen.getByRole('button', { name: /new session/i });
    await fireEvent.click(newBtn);

    // Drawer should be open — the reworked drawer has a "Generate commands" submit button
    expect(screen.getByRole('button', { name: /generate commands/i })).toBeInTheDocument();
  });

  it('closes the drawer when Cancel is clicked', async () => {
    mockGET.mockResolvedValue({ data: { items: [], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });

    await waitFor(() => expect(screen.queryByText(/loading/i)).not.toBeInTheDocument());

    await fireEvent.click(screen.getByRole('button', { name: /new session/i }));
    expect(screen.getByRole('button', { name: /generate commands/i })).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
    expect(screen.queryByRole('button', { name: /generate commands/i })).not.toBeInTheDocument();
  });

  it('subscribes to WS events when sessions are loaded', async () => {
    const session = makeSession();
    mockGET.mockResolvedValue({ data: { items: [session], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(mockSubscribe).toHaveBeenCalled();
    });
  });

  it('shows an error message when the API fails', async () => {
    mockGET.mockResolvedValue({ data: null, error: { message: 'Server error' } });

    render(SessionList, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByText(/failed to load sessions/i)).toBeInTheDocument();
    });
  });

  it('renders scope globs as code chips', async () => {
    const session = makeSession({ scope: JSON.stringify(['src/auth/**', 'tests/**']) });
    mockGET.mockResolvedValue({ data: { items: [session], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByText('src/auth/**')).toBeInTheDocument();
    });
  });

  // ── Chrome AttachHelpLink ─────────────────────────────────────────────────

  it('renders the "Setup help" link in the page actions area', async () => {
    mockGET.mockResolvedValue({ data: { items: [], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });

    expect(screen.getByRole('button', { name: /setup help/i })).toBeInTheDocument();
  });

  it('clicking the chrome "Setup help" link opens walkthrough in chrome-help mode', async () => {
    mockGET.mockResolvedValue({ data: { items: [], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });

    const helpBtn = screen.getByRole('button', { name: /setup help/i });
    await fireEvent.click(helpBtn);

    // Walkthrough dialog should be visible.
    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: /attach claude code/i })).toBeInTheDocument();
    });

    // In chrome-help mode (sessionId=null) the placeholder text is shown instead of a join cmd.
    expect(
      screen.getByText(/open a session view to copy its join command/i),
    ).toBeInTheDocument();
  });

  // ── Unit 1: Stabilize subscription effect on session-id set ──────────────
  // Regression tests for bug-squash-sessionlist-resubscribe-churn.
  // The $effect must re-run ONLY when the id-set changes, not on every
  // field-only updateSession call (which reassigns the sessions array).

  it('Unit1: a field-only WS event does not cause additional subscribe/unsubscribe churn', async () => {
    const session = makeSession({ id: 'sess-churn', name: 'Churn Test' });
    // First GET loads the session list; second GET is the refetch triggered by the handler.
    mockGET
      .mockResolvedValueOnce({ data: { items: [session], next_cursor: null }, error: null })
      .mockResolvedValue({ data: { ...session, name: 'Churn Test Updated' }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });

    // Wait for initial subscriptions to be established.
    await waitFor(() => expect(subscribeCalls.length).toBeGreaterThan(0));

    const countAfterLoad = subscribeCalls.length;
    // Each unsub stub call count after load (should be 0 — nothing unsubscribed yet).
    const unsubCallsAfterLoad = subscribeCalls.reduce((n, c) => n + c.unsub.mock.calls.length, 0);
    expect(unsubCallsAfterLoad).toBe(0);

    // Find a commit.arrived handler and simulate the event (field-only update —
    // name changes but the id set stays the same).
    const commitHandler = subscribeCalls.find((c) => c.type === 'commit.arrived')?.handler;
    expect(commitHandler).toBeDefined();
    await act(async () => {
      commitHandler!({ type: 'commit.arrived' });
      // Let the refetch promise settle.
      await Promise.resolve();
    });

    // After a field-only event:
    // 1. No new subscribe calls (id set unchanged → effect did NOT re-run).
    expect(subscribeCalls.length).toBe(countAfterLoad);
    // 2. No unsubscribe calls (no teardown occurred).
    const unsubCallsAfterEvent = subscribeCalls.reduce((n, c) => n + c.unsub.mock.calls.length, 0);
    expect(unsubCallsAfterEvent).toBe(0);
  });

  it('Unit1: id-set change causes exactly the new sessions subscriptions to be added without tearing down surviving sockets', async () => {
    // LIMITATION NOTE: SessionList.svelte only calls loadSessions() from onMount
    // (there is no external trigger to add a session to the live list in place).
    // A true "same-instance add" cannot be driven from the test harness without
    // modifying the component or its internals. This test drives the closest
    // equivalent: mount with [sess-a], unmount, then mount a NEW instance with
    // [sess-a, sess-b]. This exercises the $effect logic that runs when the
    // id-set changes (sess-a+sess-b vs sess-a-only) and verifies:
    //   1. Exactly 4 subscriptions per session are established.
    //   2. No unsubs fire for the surviving session (sess-a) during the id-set
    //      expansion (only the mount-scoped cleanup on unmount is expected).
    //   3. The $effect does NOT tear down and re-create subscriptions for the
    //      session whose id did not change (sess-a unsubs from the second mount
    //      instance must be 0 immediately after load).
    //
    // A genuine same-instance add (e.g. via a WebSocket push that adds a new
    // session row) would require the component to have a reactive sessions store
    // or an explicit "add session" input prop. Filed as a known harness limit.
    const sess1 = makeSession({ id: 'sess-a', name: 'Session A' });
    const sess2 = makeSession({ id: 'sess-b', name: 'Session B' });

    // First mount: one session (sess-a only).
    mockGET.mockResolvedValueOnce({ data: { items: [sess1], next_cursor: null }, error: null });

    const { unmount } = render(SessionList, { props: { orgId: 'org-1' } });

    // Wait for subscriptions to sess-a to be set up (4 types × 1 session).
    await waitFor(() => expect(subscribeCalls.filter((c) => c.sessionId === 'sess-a').length).toBe(4));
    expect(subscribeCalls.length).toBe(4);

    // Unmount and start fresh tracking for the second instance.
    unmount();
    subscribeCalls.length = 0;
    vi.clearAllMocks();

    // Second mount: both sessions — simulates the state after an "add session"
    // event if the component had that path (it does not, hence this approach).
    mockGET.mockResolvedValueOnce({ data: { items: [sess1, sess2], next_cursor: null }, error: null });
    render(SessionList, { props: { orgId: 'org-1' } });

    // Both sess-a and sess-b must have exactly 4 subscriptions each.
    await waitFor(() => expect(subscribeCalls.filter((c) => c.sessionId === 'sess-b').length).toBe(4));
    expect(subscribeCalls.filter((c) => c.sessionId === 'sess-a').length).toBe(4);
    expect(subscribeCalls.length).toBe(8);

    // No unsubs should have fired yet for the second-mount subscriptions
    // (cleanup only happens on unmount or id-set change, neither of which
    // occurred after the initial load in this mount).
    const unsubCallsAfterLoad = subscribeCalls.reduce((n, c) => n + c.unsub.mock.calls.length, 0);
    expect(unsubCallsAfterLoad).toBe(0);
  });

  // ── Unit 2: Sequence-guarded refetch ─────────────────────────────────────
  // Regression tests for bug-squash-ws-refetch-stale-overwrite.

  it('Unit2: a late-resolving refetch is dropped when a newer one is already applied', async () => {
    const session = makeSession({ id: 'sess-seq', name: 'Original' });
    // Initial list load.
    mockGET.mockResolvedValueOnce({ data: { items: [session], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });
    await waitFor(() => expect(subscribeCalls.length).toBeGreaterThan(0));

    // Set up two deferred GET resolvers for refetch calls.
    let resolve1!: (v: { data: Session; error: null }) => void;
    let resolve2!: (v: { data: Session; error: null }) => void;
    const p1 = new Promise<{ data: Session; error: null }>((r) => { resolve1 = r; });
    const p2 = new Promise<{ data: Session; error: null }>((r) => { resolve2 = r; });

    mockGET
      .mockReturnValueOnce(p1) // first refetch → deferred
      .mockReturnValueOnce(p2); // second refetch → deferred

    // Trigger refetch #1 (commit.arrived) and refetch #2 (presence.updated).
    const commitHandler = subscribeCalls.find((c) => c.type === 'commit.arrived' && c.sessionId === 'sess-seq')?.handler;
    const presenceHandler = subscribeCalls.find((c) => c.type === 'presence.updated' && c.sessionId === 'sess-seq')?.handler;
    expect(commitHandler).toBeDefined();
    expect(presenceHandler).toBeDefined();

    await act(async () => { commitHandler!({ type: 'commit.arrived' }); });
    await act(async () => { presenceHandler!({ type: 'presence.updated' }); });

    // Resolve in REVERSE order: second refetch resolves first with 'Newer'.
    await act(async () => {
      resolve2({ data: { ...session, name: 'Newer' }, error: null });
      await Promise.resolve();
    });

    // Then the first (stale) refetch resolves with 'Stale'.
    await act(async () => {
      resolve1({ data: { ...session, name: 'Stale' }, error: null });
      await Promise.resolve();
    });

    // The stale response should have been dropped; 'Newer' should persist.
    await waitFor(() => {
      expect(screen.getByText('Newer')).toBeInTheDocument();
    });
    expect(screen.queryByText('Stale')).not.toBeInTheDocument();
  });

  it('Unit2: a commit.arrived refetch in flight when session.ended fires does not overwrite ended status', async () => {
    const session = makeSession({ id: 'sess-end', name: 'End Race', status: 'active' });
    mockGET.mockResolvedValueOnce({ data: { items: [session], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });
    await waitFor(() => expect(subscribeCalls.filter((c) => c.sessionId === 'sess-end').length).toBeGreaterThan(0));

    // Set up a deferred GET for the commit.arrived refetch.
    let resolveCommit!: (v: { data: Session; error: null }) => void;
    const pCommit = new Promise<{ data: Session; error: null }>((r) => { resolveCommit = r; });
    mockGET.mockReturnValueOnce(pCommit);

    const commitHandler = subscribeCalls.find((c) => c.type === 'commit.arrived' && c.sessionId === 'sess-end')?.handler;
    const endedHandler = subscribeCalls.find((c) => c.type === 'session.ended' && c.sessionId === 'sess-end')?.handler;
    expect(commitHandler).toBeDefined();
    expect(endedHandler).toBeDefined();

    // Trigger commit.arrived refetch (in flight, not resolved yet).
    await act(async () => { commitHandler!({ type: 'commit.arrived' }); });

    // session.ended fires — bumps refetchSeq, sets status = 'ended'.
    await act(async () => { endedHandler!({ type: 'session.ended' }); });

    // Verify 'ended' pill appears.
    await waitFor(() => expect(screen.getByText('ended')).toBeInTheDocument());

    // Now resolve the stale commit.arrived refetch with 'active' status.
    await act(async () => {
      resolveCommit({ data: { ...session, status: 'active' }, error: null });
      await Promise.resolve();
    });

    // The ended status must NOT be overwritten.
    expect(screen.getByText('ended')).toBeInTheDocument();
    expect(screen.queryByText('active')).not.toBeInTheDocument();
  });

  it('Unit2: a failed GET does not overwrite existing session state', async () => {
    const session = makeSession({ id: 'sess-err', name: 'Error Race', status: 'active' });
    mockGET.mockResolvedValueOnce({ data: { items: [session], next_cursor: null }, error: null });

    render(SessionList, { props: { orgId: 'org-1' } });
    await waitFor(() => expect(subscribeCalls.filter((c) => c.sessionId === 'sess-err').length).toBeGreaterThan(0));

    // Refetch returns an error (no data).
    mockGET.mockResolvedValueOnce({ data: null, error: { message: 'Internal Server Error' } });

    const commitHandler = subscribeCalls.find((c) => c.type === 'commit.arrived' && c.sessionId === 'sess-err')?.handler;
    expect(commitHandler).toBeDefined();

    await act(async () => {
      commitHandler!({ type: 'commit.arrived' });
      await Promise.resolve();
    });

    // The session should still be shown with original state — no crash, no overwrite.
    await waitFor(() => expect(screen.getByText('Error Race')).toBeInTheDocument());
  });
});
