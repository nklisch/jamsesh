// SessionTombstone.test.ts
// Tests: loading state, 200 tombstone render, 404 redirect to live session, error state.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/svelte';
import SessionTombstone from './SessionTombstone.svelte';
import type { components } from '$lib/api/types.gen';

type PlaygroundTombstone = components['schemas']['PlaygroundTombstone'];

// ── Module mocks ────────────────────────────────────────────────────────────

const mockGET = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: {
    GET: (...args: unknown[]) => mockGET(...args),
  },
}));

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  navigate: (...args: unknown[]) => mockNavigate(...args),
  current: { name: 'playground-ended', params: { sessionId: 'sess-pg-1' }, requiresAuth: false },
}));

// ── Fixtures ──────────────────────────────────────────────────────────────────

function makeTombstone(overrides: Partial<PlaygroundTombstone> = {}): PlaygroundTombstone {
  const endedAt = new Date(Date.now() - 3600 * 1000).toISOString();
  return {
    session_id: 'sess-pg-1',
    org_id: 'org_playground',
    members_count: 3,
    commits_count: 29,
    auto_merges_count: 14,
    duration_seconds: 86280, // 23h 58m
    end_reason: 'hard_cap',
    ended_at: endedAt,
    expires_at: new Date(Date.now() + 30 * 24 * 3600 * 1000).toISOString(),
    ...overrides,
  };
}

const DEFAULT_PROPS = { sessionId: 'sess-pg-1' };

// ── Tests ────────────────────────────────────────────────────────────────────

describe('SessionTombstone', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  // ── Loading state ─────────────────────────────────────────────────────────

  it('renders loading state before the GET resolves', () => {
    mockGET.mockReturnValue(new Promise(() => {}));
    render(SessionTombstone, { props: DEFAULT_PROPS });
    expect(screen.getByText(/Loading session details/i)).toBeInTheDocument();
    expect(document.querySelector('[aria-busy="true"]')).toBeInTheDocument();
  });

  // ── 200: tombstone rendered ───────────────────────────────────────────────

  it('calls GET /api/playground/sessions/{id}/tombstone on mount', async () => {
    mockGET.mockResolvedValue({ data: makeTombstone(), error: undefined, response: { status: 200 } });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(mockGET).toHaveBeenCalledOnce();
    });
    const [url, opts] = mockGET.mock.calls[0] as [string, { params: { path: { id: string } } }];
    expect(url).toBe('/api/playground/sessions/{id}/tombstone');
    expect(opts.params.path.id).toBe('sess-pg-1');
  });

  it('renders the post-destruction headline', async () => {
    mockGET.mockResolvedValue({ data: makeTombstone(), error: undefined, response: { status: 200 } });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByText(/This playground session has ended/i)).toBeInTheDocument();
    });
  });

  it('renders member count from tombstone', async () => {
    mockGET.mockResolvedValue({ data: makeTombstone({ members_count: 3 }), error: undefined, response: { status: 200 } });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByText('3')).toBeInTheDocument();
    });
    expect(screen.getByText('members')).toBeInTheDocument();
  });

  it('renders commits count from tombstone', async () => {
    mockGET.mockResolvedValue({ data: makeTombstone({ commits_count: 29 }), error: undefined, response: { status: 200 } });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByText('29')).toBeInTheDocument();
    });
    expect(screen.getByText('commits')).toBeInTheDocument();
  });

  it('renders auto_merges_count from tombstone', async () => {
    mockGET.mockResolvedValue({ data: makeTombstone({ auto_merges_count: 14 }), error: undefined, response: { status: 200 } });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByText('14')).toBeInTheDocument();
    });
    expect(screen.getByText('auto-merges')).toBeInTheDocument();
  });

  it('renders duration formatted as h and m', async () => {
    mockGET.mockResolvedValue({
      data: makeTombstone({ duration_seconds: 86280 }), // 23h 58m
      error: undefined,
      response: { status: 200 },
    });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByText('23h 58m')).toBeInTheDocument();
    });
  });

  it('renders duration in minutes only when under an hour', async () => {
    mockGET.mockResolvedValue({
      data: makeTombstone({ duration_seconds: 2700 }), // 45m
      error: undefined,
      response: { status: 200 },
    });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByText('45m')).toBeInTheDocument();
    });
  });

  it('renders "Try another playground" CTA', async () => {
    mockGET.mockResolvedValue({ data: makeTombstone(), error: undefined, response: { status: 200 } });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByText(/Try another playground/i)).toBeInTheDocument();
    });
  });

  it('"Try another playground" navigates to /playground', async () => {
    mockGET.mockResolvedValue({ data: makeTombstone(), error: undefined, response: { status: 200 } });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByText(/Try another playground/i)).toBeInTheDocument();
    });
    await fireEvent.click(screen.getByText(/Try another playground/i));
    expect(mockNavigate).toHaveBeenCalledWith('/playground');
  });

  it('renders "Sign up for a durable account" CTA', async () => {
    mockGET.mockResolvedValue({ data: makeTombstone(), error: undefined, response: { status: 200 } });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByText(/Sign up for a durable account/i)).toBeInTheDocument();
    });
  });

  it('"Sign up for a durable account" navigates to /', async () => {
    mockGET.mockResolvedValue({ data: makeTombstone(), error: undefined, response: { status: 200 } });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByText(/Sign up for a durable account/i)).toBeInTheDocument();
    });
    await fireEvent.click(screen.getByText(/Sign up for a durable account/i));
    expect(mockNavigate).toHaveBeenCalledWith('/');
  });

  // ── 404: session still active (redirect) ──────────────────────────────────

  it('on 404: redirects to the live session view (session still active)', async () => {
    mockGET.mockResolvedValue({
      data: undefined,
      error: { error: 'not_found', message: 'not found' },
      response: { status: 404 },
    });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/orgs/org_playground/sessions/sess-pg-1');
    });
  });

  // ── Error state ───────────────────────────────────────────────────────────

  it('on unexpected error: renders an error state', async () => {
    mockGET.mockResolvedValue({
      data: undefined,
      error: { message: 'something went wrong' },
      response: { status: 500 },
    });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });
    // The h1 "Something went wrong." and the lede paragraph both match, so use getAllByText.
    const matches = screen.getAllByText(/Something went wrong/i);
    expect(matches.length).toBeGreaterThan(0);
  });

  it('on network failure: renders the error state with a server-unreachable message', async () => {
    mockGET.mockRejectedValue(new TypeError('Failed to fetch'));
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });
    expect(screen.getByText(/Could not reach the server/i)).toBeInTheDocument();
    // Loading indicator should be gone after the error surfaces.
    expect(screen.queryByText(/Loading session details/i)).not.toBeInTheDocument();
  });

  it('clicking "Try again" re-fires the GET request', async () => {
    mockGET.mockResolvedValueOnce({
      data: undefined,
      error: { message: 'server error' },
      response: { status: 500 },
    });

    render(SessionTombstone, { props: DEFAULT_PROPS });
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });

    mockGET.mockResolvedValueOnce({ data: makeTombstone(), error: undefined, response: { status: 200 } });
    const tryAgainBtn = screen.getByRole('button', { name: /try again/i });
    await fireEvent.click(tryAgainBtn);

    await waitFor(() => {
      expect(screen.getByText(/This playground session has ended/i)).toBeInTheDocument();
    });
    expect(mockGET).toHaveBeenCalledTimes(2);
  });

  // ── Top bar ───────────────────────────────────────────────────────────────

  it('renders the jamsesh wordmark in the top bar', async () => {
    mockGET.mockResolvedValue({ data: makeTombstone(), error: undefined, response: { status: 200 } });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    const wordmark = document.querySelector('.wordmark');
    expect(wordmark).toBeInTheDocument();
    expect(wordmark!.textContent).toMatch(/jamsesh/);
  });

  it('renders a "Sign in →" link in the top bar', async () => {
    mockGET.mockResolvedValue({ data: makeTombstone(), error: undefined, response: { status: 200 } });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    expect(screen.getByText(/Sign in →/i)).toBeInTheDocument();
  });

  it('clicking "Sign in" navigates to /login', async () => {
    mockGET.mockResolvedValue({ data: makeTombstone(), error: undefined, response: { status: 200 } });
    render(SessionTombstone, { props: DEFAULT_PROPS });
    await fireEvent.click(screen.getByText(/Sign in →/i));
    expect(mockNavigate).toHaveBeenCalledWith('/login');
  });
});
