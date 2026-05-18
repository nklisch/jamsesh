import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/svelte';
import InviteAccept from './InviteAccept.svelte';
import type { components } from '$lib/api/types.gen';

type SessionInviteDetails = components['schemas']['SessionInviteDetails'];

// ── Module mocks ────────────────────────────────────────────────────────────

const mockGET = vi.fn();
const mockPOST = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    GET: (...args: unknown[]) => mockGET(...args),
    POST: (...args: unknown[]) => mockPOST(...args),
  },
}));

vi.mock('$lib/auth.svelte', () => ({
  auth: {
    currentUser: { id: 'user-1', email: 'test@example.com', displayName: 'Test User' },
    isAuthenticated: true,
  },
}));

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  current: { name: 'invite-accept', params: { orgId: 'org-1', sessionId: 'sess-1', inviteId: 'inv-1' } },
  navigate: (...args: unknown[]) => mockNavigate(...args),
}));

// ── Query-string helpers ─────────────────────────────────────────────────────
//
// jsdom's window.location is readonly in some versions — assign via Object.defineProperty.

function setSearch(search: string) {
  Object.defineProperty(window, 'location', {
    value: { ...window.location, search },
    writable: true,
    configurable: true,
  });
}

// ── Fixtures ──────────────────────────────────────────────────────────────────

function makeDetails(overrides: Partial<SessionInviteDetails> = {}): SessionInviteDetails {
  return {
    invite_id: 'inv-1',
    session_id: 'sess-1',
    session_name: 'spec refinement',
    session_goal: 'Refine the backlog',
    org_name: 'Acme Corp',
    invited_by_name: 'Marcus Chen',
    expires_at: new Date(Date.now() + 48 * 60 * 60 * 1000).toISOString(),
    your_role_on_accept: 'member',
    ...overrides,
  };
}

const DEFAULT_PROPS = { orgId: 'org-1', sessionId: 'sess-1', inviteId: 'inv-1' };

// ── Tests ──────────────────────────────────────────────────────────────────────

describe('InviteAccept', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setSearch('?token=tok-abc');
  });

  afterEach(() => {
    cleanup();
  });

  // ── Loading state ──────────────────────────────────────────────────────────

  it('renders loading state on mount before GET resolves', () => {
    // GET never resolves during this assertion
    mockGET.mockReturnValue(new Promise(() => {}));
    render(InviteAccept, { props: DEFAULT_PROPS });
    expect(screen.getByText(/checking invite/i)).toBeInTheDocument();
  });

  // ── Token missing ──────────────────────────────────────────────────────────

  it('shows error state when token is missing from query string', async () => {
    setSearch('');
    render(InviteAccept, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByText(/this invite is no longer valid/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/missing_token/)).toBeInTheDocument();
    // GET should NOT be called without a token
    expect(mockGET).not.toHaveBeenCalled();
  });

  // ── Happy path: GET 200 ────────────────────────────────────────────────────

  it('renders ready state with invite details when GET returns 200', async () => {
    mockGET.mockResolvedValue({ data: makeDetails(), error: null });
    render(InviteAccept, { props: DEFAULT_PROPS });

    await waitFor(() => {
      expect(screen.getByText(/spec refinement/)).toBeInTheDocument();
    });

    // Inviter pill
    expect(screen.getByText(/Marcus Chen/)).toBeInTheDocument();
    // Org name in lead
    expect(screen.getByText(/Acme Corp/)).toBeInTheDocument();
    // Accept CTA
    expect(screen.getByRole('button', { name: /accept/i })).toBeInTheDocument();
    // Decline link
    expect(screen.getByRole('button', { name: /decline/i })).toBeInTheDocument();
    // Explainer card
    expect(screen.getByText(/what happens when you accept/i)).toBeInTheDocument();
  });

  it('calls GET with correct path and token query param', async () => {
    mockGET.mockResolvedValue({ data: makeDetails(), error: null });
    render(InviteAccept, { props: DEFAULT_PROPS });

    await waitFor(() => expect(mockGET).toHaveBeenCalledOnce());
    expect(mockGET).toHaveBeenCalledWith(
      '/api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}',
      expect.objectContaining({
        params: {
          path: { orgID: 'org-1', sessionID: 'sess-1', inviteID: 'inv-1' },
          query: { token: 'tok-abc' },
        },
      }),
    );
  });

  // ── Error states from GET ──────────────────────────────────────────────────

  it('shows error state when GET returns 401', async () => {
    mockGET.mockResolvedValue({ data: null, error: { error: 'auth.invalid_token', message: 'bad token' } });
    render(InviteAccept, { props: DEFAULT_PROPS });

    await waitFor(() => {
      expect(screen.getByText(/this invite is no longer valid/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/auth\.invalid_token/)).toBeInTheDocument();
  });

  it('shows error state when GET returns 409 (already accepted)', async () => {
    mockGET.mockResolvedValue({ data: null, error: { error: 'invite.already_accepted', message: 'already accepted' } });
    render(InviteAccept, { props: DEFAULT_PROPS });

    await waitFor(() => {
      expect(screen.getByText(/this invite is no longer valid/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/invite\.already_accepted/)).toBeInTheDocument();
  });

  it('shows error state when GET has a network error (no error code)', async () => {
    mockGET.mockResolvedValue({ data: null, error: {} });
    render(InviteAccept, { props: DEFAULT_PROPS });

    await waitFor(() => {
      expect(screen.getByText(/this invite is no longer valid/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/network_error/)).toBeInTheDocument();
  });

  // ── Accept flow ────────────────────────────────────────────────────────────

  it('POSTs with the token in the body and navigates to session on 200', async () => {
    mockGET.mockResolvedValue({ data: makeDetails(), error: null });
    mockPOST.mockResolvedValue({
      data: { id: 'sess-1', name: 'spec refinement' },
      error: null,
      response: { status: 200 },
    });
    render(InviteAccept, { props: DEFAULT_PROPS });

    await waitFor(() => expect(screen.getByRole('button', { name: /accept/i })).toBeInTheDocument());

    await fireEvent.click(screen.getByRole('button', { name: /accept/i }));

    await waitFor(() => {
      expect(mockPOST).toHaveBeenCalledWith(
        '/api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept',
        expect.objectContaining({
          params: { path: { orgID: 'org-1', sessionID: 'sess-1', inviteID: 'inv-1' } },
          body: { token: 'tok-abc' },
        }),
      );
    });

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/orgs/org-1/sessions/sess-1');
    });
  });

  it('disables Accept button while POST is in flight', async () => {
    mockGET.mockResolvedValue({ data: makeDetails(), error: null });
    // POST never resolves — keeps component in 'accepting' state
    mockPOST.mockReturnValue(new Promise(() => {}));
    render(InviteAccept, { props: DEFAULT_PROPS });

    await waitFor(() => expect(screen.getByRole('button', { name: /accept/i })).toBeInTheDocument());

    const acceptBtn = screen.getByRole('button', { name: /accept/i });
    await fireEvent.click(acceptBtn);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /joining/i })).toBeDisabled();
    });
  });

  it('shows rejection state when POST returns 403 + auth.org_membership_required', async () => {
    mockGET.mockResolvedValue({ data: makeDetails(), error: null });
    mockPOST.mockResolvedValue({
      data: null,
      error: { error: 'auth.org_membership_required', message: 'not an org member' },
      response: { status: 403 },
    });
    render(InviteAccept, { props: DEFAULT_PROPS });

    await waitFor(() => expect(screen.getByRole('button', { name: /accept/i })).toBeInTheDocument());

    await fireEvent.click(screen.getByRole('button', { name: /accept/i }));

    await waitFor(() => {
      expect(screen.getByText(/members only/i)).toBeInTheDocument();
    });
    // Warning alert with org name
    expect(screen.getByText(/ask an admin/i)).toBeInTheDocument();
    // No longer navigated away
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('shows error state when POST returns any other error', async () => {
    mockGET.mockResolvedValue({ data: makeDetails(), error: null });
    mockPOST.mockResolvedValue({
      data: null,
      error: { error: 'invite.expired', message: 'invite has expired' },
      response: { status: 410 },
    });
    render(InviteAccept, { props: DEFAULT_PROPS });

    await waitFor(() => expect(screen.getByRole('button', { name: /accept/i })).toBeInTheDocument());

    await fireEvent.click(screen.getByRole('button', { name: /accept/i }));

    await waitFor(() => {
      expect(screen.getByText(/this invite is no longer valid/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/invite\.expired/)).toBeInTheDocument();
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  // ── Decline flow ───────────────────────────────────────────────────────────

  it('navigates to /orgs/:orgId/sessions when authenticated user clicks Decline', async () => {
    mockGET.mockResolvedValue({ data: makeDetails(), error: null });
    render(InviteAccept, { props: DEFAULT_PROPS });

    await waitFor(() => expect(screen.getByRole('button', { name: /decline/i })).toBeInTheDocument());

    await fireEvent.click(screen.getByRole('button', { name: /decline/i }));

    expect(mockNavigate).toHaveBeenCalledWith('/orgs/org-1/sessions');
  });
});

// ── Route pattern test ────────────────────────────────────────────────────────

describe('InviteAccept — route pattern', () => {
  it('matches invite-accept route and extracts all three params', async () => {
    vi.resetModules();
    vi.doUnmock('$lib/router.svelte');
    const router = await import('$lib/router.svelte');
    router.navigate('/orgs/my-org/sessions/sess-42/invites/inv-7/accept');
    expect(router.current.name).toBe('invite-accept');
    expect(router.current.params).toEqual({
      orgId: 'my-org',
      sessionId: 'sess-42',
      inviteId: 'inv-7',
    });
  });

  it('does not match session-view route for invite-accept path', async () => {
    vi.resetModules();
    vi.doUnmock('$lib/router.svelte');
    const router = await import('$lib/router.svelte');
    router.navigate('/orgs/my-org/sessions/sess-42');
    expect(router.current.name).toBe('session-view');
    // The invite path is longer and should not match session-view
    router.navigate('/orgs/my-org/sessions/sess-42/invites/inv-7/accept');
    expect(router.current.name).toBe('invite-accept');
  });
});
