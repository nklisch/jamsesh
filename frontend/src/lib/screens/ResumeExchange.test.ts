// ResumeExchange.test.ts
// Verifies:
//  - bare fetch sends NO Authorization header
//  - #rt stripped via history.replaceState before any other nav/asset
//  - success adopt+navigate for playground AND durable
//  - confirm accept (adopts) / decline (no bearer persisted + retry hint)
//  - generic error on bad/expired/missing token

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, cleanup, fireEvent } from '@testing-library/svelte';
import ResumeExchange from './ResumeExchange.svelte';

// ── Module mocks ─────────────────────────────────────────────────────────────

const mockSetPlaygroundContext = vi.fn();
const mockSetAccessOnly = vi.fn();
let mockToken: string | null = null;
let mockCurrentUser: { id: string; email: string; displayName: string } | null = null;
let mockPlaygroundContext: { sessionId: string; bearer: string; nickname: string } | null = null;

vi.mock('$lib/auth.svelte', () => ({
  auth: {
    get token() { return mockToken; },
    get currentUser() { return mockCurrentUser; },
    get playgroundContext() { return mockPlaygroundContext; },
    setPlaygroundContext: (...args: unknown[]) => mockSetPlaygroundContext(...args),
    setAccessOnly: (...args: unknown[]) => mockSetAccessOnly(...args),
  },
}));

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  current: { name: 'session-resume', params: { orgId: 'org-1', sessionId: 'sess-1' } },
  navigate: (...args: unknown[]) => mockNavigate(...args),
}));

// ── Hash helpers ──────────────────────────────────────────────────────────────

function setHash(hash: string) {
  Object.defineProperty(window, 'location', {
    value: {
      ...window.location,
      hash,
      pathname: '/orgs/org-1/sessions/sess-1/resume',
      search: '',
      origin: 'http://localhost:3000',
    },
    writable: true,
    configurable: true,
  });
}

// ── Exchange response factory ─────────────────────────────────────────────────

function makeExchangeResponse(overrides: Partial<{
  bearer: string;
  session_id: string;
  org_id: string;
  kind: 'playground' | 'durable';
  account_id: string;
  display_name: string;
  expires_at: string;
}> = {}) {
  return {
    bearer: 'test-bearer-token',
    session_id: 'sess-abc',
    org_id: 'org-xyz',
    kind: 'durable' as const,
    account_id: 'user-123',
    display_name: 'Test User',
    expires_at: new Date(Date.now() + 3600_000).toISOString(),
    ...overrides,
  };
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('ResumeExchange', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockToken = null;
    mockCurrentUser = null;
    mockPlaygroundContext = null;
    setHash('#rt=test-resume-token');

    // Default: successful durable exchange
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(makeExchangeResponse()),
    }));
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  // ── Loading state ─────────────────────────────────────────────────────────

  it('renders "resuming" state on mount before fetch resolves', () => {
    vi.stubGlobal('fetch', vi.fn().mockReturnValue(new Promise(() => {})));
    render(ResumeExchange);
    expect(screen.getByText(/resuming your session/i)).toBeInTheDocument();
  });

  // ── Security: bare fetch sends NO Authorization header ─────────────────────

  it('bare fetch sends no Authorization header (does not attach any bearer)', async () => {
    // Give the caller a valid token to make sure we don't send it
    mockToken = 'existing-bearer-should-not-be-sent';
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(makeExchangeResponse()),
    });
    vi.stubGlobal('fetch', mockFetch);

    render(ResumeExchange);
    await waitFor(() => expect(mockFetch).toHaveBeenCalled());

    const [, fetchInit] = mockFetch.mock.calls[0] as [string, RequestInit];
    const headers = fetchInit.headers as Record<string, string> | undefined;
    // Must not contain any Authorization key (case-insensitive)
    if (headers) {
      const headerKeys = Object.keys(headers).map((k) => k.toLowerCase());
      expect(headerKeys).not.toContain('authorization');
    }
    expect(fetchInit.credentials).toBe('omit');
  });

  it('fetch is called with the exchange endpoint and resume_token in body', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(makeExchangeResponse()),
    });
    vi.stubGlobal('fetch', mockFetch);

    render(ResumeExchange);
    await waitFor(() => expect(mockFetch).toHaveBeenCalledOnce());

    const [url, init] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(url).toMatch(/\/api\/session-resumes\/exchange$/);
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ resume_token: 'test-resume-token' }));
  });

  // ── Security: #rt stripped immediately ────────────────────────────────────

  it('calls history.replaceState to strip #rt before exchange resolves', async () => {
    const replaceStateSpy = vi.spyOn(history, 'replaceState');
    // Keep fetch in-flight so we can assert replaceState was called before
    // the exchange even resolves.
    vi.stubGlobal('fetch', vi.fn().mockReturnValue(new Promise(() => {})));

    render(ResumeExchange);

    await waitFor(() => expect(replaceStateSpy).toHaveBeenCalled());
    expect(replaceStateSpy).toHaveBeenCalledWith(
      null,
      '',
      '/orgs/org-1/sessions/sess-1/resume',
    );
  });

  // ── Missing token ─────────────────────────────────────────────────────────

  it('shows error state immediately when hash contains no #rt', async () => {
    setHash('');
    render(ResumeExchange);

    await waitFor(() => {
      expect(screen.getByText(/this resume link has expired/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/run the command again from your terminal/i)).toBeInTheDocument();
    // Fetch must NOT be called with no token
    expect(vi.mocked(globalThis.fetch)).not.toHaveBeenCalled();
  });

  it('shows generic retry hint (no oracle / no raw error code) on missing token', async () => {
    setHash('');
    render(ResumeExchange);

    await waitFor(() => {
      expect(screen.getByText(/run the command again/i)).toBeInTheDocument();
    });
    // Confirm there is no raw error code rendered
    expect(screen.queryByText(/resume\.token/i)).not.toBeInTheDocument();
  });

  // ── Success: durable adopt + navigate ─────────────────────────────────────

  it('durable: calls setAccessOnly and navigates to org session on success', async () => {
    const resp = makeExchangeResponse({ kind: 'durable', org_id: 'org-xyz', session_id: 'sess-abc' });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(resp),
    }));

    render(ResumeExchange);

    await waitFor(() => expect(mockSetAccessOnly).toHaveBeenCalledWith('test-bearer-token'));
    expect(mockSetPlaygroundContext).not.toHaveBeenCalled();
    expect(mockNavigate).toHaveBeenCalledWith('/orgs/org-xyz/sessions/sess-abc');
  });

  it('durable: encodes URI segments in the navigation path', async () => {
    const resp = makeExchangeResponse({
      kind: 'durable',
      org_id: 'org with spaces',
      session_id: 'sess/slash',
    });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(resp),
    }));

    render(ResumeExchange);

    await waitFor(() => expect(mockNavigate).toHaveBeenCalled());
    expect(mockNavigate).toHaveBeenCalledWith(
      `/orgs/${encodeURIComponent('org with spaces')}/sessions/${encodeURIComponent('sess/slash')}`,
    );
  });

  // ── Success: playground adopt + navigate ─────────────────────────────────

  it('playground: calls setPlaygroundContext (mirrors JoinerPicker) and navigates', async () => {
    const resp = makeExchangeResponse({
      kind: 'playground',
      session_id: 'play-sess-1',
      display_name: 'GreenFox',
    });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(resp),
    }));

    render(ResumeExchange);

    await waitFor(() => expect(mockSetPlaygroundContext).toHaveBeenCalledOnce());
    const [ctx] = mockSetPlaygroundContext.mock.calls[0] as [{ sessionId: string; bearer: string; nickname: string }][];
    expect((ctx as unknown as { sessionId: string }).sessionId).toBe('play-sess-1');
    expect((ctx as unknown as { bearer: string }).bearer).toBe('test-bearer-token');
    expect(mockSetAccessOnly).not.toHaveBeenCalled();
    expect(mockNavigate).toHaveBeenCalledWith('/orgs/org_playground/sessions/play-sess-1');
  });

  it('playground: never calls setAccessOnly', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(makeExchangeResponse({ kind: 'playground' })),
    }));

    render(ResumeExchange);

    await waitFor(() => expect(mockSetPlaygroundContext).toHaveBeenCalled());
    expect(mockSetAccessOnly).not.toHaveBeenCalled();
  });

  // ── Confirm flow: differing existing identity ─────────────────────────────

  it('shows confirming state when currentUser id differs from account_id', async () => {
    mockToken = 'existing-token';
    mockCurrentUser = { id: 'different-user', email: 'other@example.com', displayName: 'Other User' };
    const resp = makeExchangeResponse({ account_id: 'user-123', display_name: 'CLI User' });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(resp),
    }));

    render(ResumeExchange);

    await waitFor(() => {
      expect(screen.getByText(/resume as a different account/i)).toBeInTheDocument();
    });
    // Display name appears in the body text (inside <strong>)
    expect(screen.getAllByText(/CLI User/).length).toBeGreaterThan(0);
  });

  it('shows confirming state when authenticated but currentUser is null (conservative)', async () => {
    mockToken = 'existing-token';
    mockCurrentUser = null;
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(makeExchangeResponse()),
    }));

    render(ResumeExchange);

    await waitFor(() => {
      expect(screen.getByText(/resume as a different account/i)).toBeInTheDocument();
    });
  });

  it('shows confirming state when an existing playground context is present', async () => {
    mockToken = null;
    mockPlaygroundContext = { sessionId: 'old-sess', bearer: 'old-bearer', nickname: 'OldNick' };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(makeExchangeResponse()),
    }));

    render(ResumeExchange);

    await waitFor(() => {
      expect(screen.getByText(/resume as a different account/i)).toBeInTheDocument();
    });
  });

  // ── Confirm: accept ────────────────────────────────────────────────────────

  it('confirm accept: adopts and navigates (durable)', async () => {
    mockToken = 'existing-token';
    mockCurrentUser = { id: 'other-user', email: 'o@x.com', displayName: 'Other' };
    const resp = makeExchangeResponse({ kind: 'durable', account_id: 'user-123', org_id: 'org-xyz', session_id: 'sess-abc' });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(resp),
    }));

    render(ResumeExchange);

    await waitFor(() => {
      expect(screen.getByText(/resume as a different account/i)).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole('button', { name: /continue as/i }));

    await waitFor(() => expect(mockSetAccessOnly).toHaveBeenCalledWith('test-bearer-token'));
    expect(mockNavigate).toHaveBeenCalledWith('/orgs/org-xyz/sessions/sess-abc');
  });

  // ── Confirm: decline (bearer NOT persisted) ────────────────────────────────

  it('confirm decline: shows retry hint and does NOT call setAccessOnly or setPlaygroundContext', async () => {
    mockToken = 'existing-token';
    mockCurrentUser = { id: 'other-user', email: 'o@x.com', displayName: 'Other' };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(makeExchangeResponse()),
    }));

    render(ResumeExchange);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole('button', { name: /cancel/i }));

    await waitFor(() => {
      expect(screen.getByText(/run the command again from your terminal/i)).toBeInTheDocument();
    });
    expect(mockSetAccessOnly).not.toHaveBeenCalled();
    expect(mockSetPlaygroundContext).not.toHaveBeenCalled();
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  // ── Error on failed exchange ───────────────────────────────────────────────

  it('shows generic error state on non-ok response (expired/used token)', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 410,
      json: vi.fn().mockResolvedValue({ error: 'resume.token_used' }),
    }));

    render(ResumeExchange);

    await waitFor(() => {
      expect(screen.getByText(/this resume link has expired/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/run the command again from your terminal/i)).toBeInTheDocument();
    // No raw error code rendered (no oracle)
    expect(screen.queryByText(/resume\.token_used/)).not.toBeInTheDocument();
  });

  it('shows generic error state on network failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('Network error')));

    render(ResumeExchange);

    await waitFor(() => {
      expect(screen.getByText(/this resume link has expired/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/run the command again/i)).toBeInTheDocument();
  });

  // ── Token/bearer never rendered ────────────────────────────────────────────

  it('never renders the resume token or bearer in the DOM', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(makeExchangeResponse({ bearer: 'SECRET-BEARER-XYZ' })),
    }));

    render(ResumeExchange);

    // Wait until adopt completes
    await waitFor(() => expect(mockSetAccessOnly).toHaveBeenCalled());

    expect(document.body.textContent).not.toContain('test-resume-token');
    expect(document.body.textContent).not.toContain('SECRET-BEARER-XYZ');
  });
});
