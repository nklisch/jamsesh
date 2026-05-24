import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
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

const mockSubscribe = vi.fn().mockReturnValue(() => {});
vi.mock('$lib/ws.svelte', () => ({
  subscribe: (...args: unknown[]) => mockSubscribe(...args),
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
    Object.defineProperty(globalThis.navigator, 'clipboard', {
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
      configurable: true,
    });
    localStorage.clear();
  });

  afterEach(() => {
    vi.clearAllMocks();
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
});
