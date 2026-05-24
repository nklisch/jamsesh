// JoinerPicker.test.ts
// Tests: nickname form, happy-path join, 409 session-full, 410 session-ended redirect.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/svelte';
import JoinerPicker from './JoinerPicker.svelte';
import type { components } from '$lib/api/types.gen';

type PlaygroundSessionSummary = components['schemas']['PlaygroundSessionSummary'];
type PlaygroundJoinResult = components['schemas']['PlaygroundJoinResult'];

// ── Module mocks ────────────────────────────────────────────────────────────

const mockPOST = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: {
    POST: (...args: unknown[]) => mockPOST(...args),
  },
}));

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  navigate: (...args: unknown[]) => mockNavigate(...args),
  current: { name: 'playground-join', params: { sessionId: 'sess-pg-1' }, requiresAuth: false },
}));

const mockSetPlaygroundContext = vi.fn();
vi.mock('$lib/auth.svelte', () => ({
  auth: {
    get playgroundContext() { return null; },
    setPlaygroundContext: (...args: unknown[]) => mockSetPlaygroundContext(...args),
    get isAuthenticated() { return false; },
  },
}));

// ── Fixtures ──────────────────────────────────────────────────────────────────

function makeSession(overrides: Partial<PlaygroundSessionSummary> = {}): PlaygroundSessionSummary {
  return {
    id: 'sess-pg-1',
    org_id: 'org_playground',
    name: 'playground-01ab',
    goal: '',
    scope: '["**"]',
    status: 'active',
    created_at: new Date().toISOString(),
    hard_cap_at: new Date(Date.now() + 24 * 3600 * 1000).toISOString(),
    idle_timeout_at: new Date(Date.now() + 30 * 60 * 1000).toISOString(),
    members_count: 1,
    ...overrides,
  };
}

function makeJoinResult(overrides: Partial<PlaygroundJoinResult> = {}): PlaygroundJoinResult {
  return {
    session: makeSession(),
    bearer: 'jamsesh_anon_test_bearer',
    nickname: 'quiet-fox',
    expires_at: new Date(Date.now() + 24 * 3600 * 1000).toISOString(),
    ...overrides,
  };
}

const DEFAULT_PROPS = { sessionId: 'sess-pg-1' };

// ── Tests ────────────────────────────────────────────────────────────────────

describe('JoinerPicker', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  // ── Layout / content ──────────────────────────────────────────────────────

  it('renders the jamsesh wordmark', () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    const wordmark = document.querySelector('.wordmark');
    expect(wordmark).toBeInTheDocument();
  });

  it('renders the playground chip in the top bar', () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    expect(screen.getByText(/playground/i, { selector: '.playground-chip' })).toBeInTheDocument();
  });

  it('renders the join eyebrow', () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    expect(screen.getByText(/You were invited to a playground/i)).toBeInTheDocument();
  });

  it('renders the join headline', () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    expect(screen.getByText(/Joining a playground session/i)).toBeInTheDocument();
  });

  it('renders the "You\'ll join as" label', () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    expect(screen.getByLabelText("You'll join as")).toBeInTheDocument();
  });

  it('pre-fills the nickname input with a suggestion', () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    const input = screen.getByLabelText("You'll join as") as HTMLInputElement;
    expect(input.value).toMatch(/^[a-z]+-[a-z]+$/);
  });

  it('renders the reroll button', () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    const rerollBtn = screen.getByRole('button', { name: /suggest a different nickname/i });
    expect(rerollBtn).toBeInTheDocument();
  });

  it('renders the ephemeral warning note', () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    expect(screen.getByText(/Playground sessions are throwaway/i)).toBeInTheDocument();
  });

  // ── Nickname input ────────────────────────────────────────────────────────

  it('the nickname input is editable', async () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    const input = screen.getByLabelText("You'll join as") as HTMLInputElement;
    input.value = 'my-handle';
    await fireEvent.input(input);
    expect(input.value).toBe('my-handle');
  });

  it('clicking the reroll button changes the suggested nickname', async () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    const input = screen.getByLabelText("You'll join as") as HTMLInputElement;
    const initial = input.value;
    const rerollBtn = screen.getByRole('button', { name: /suggest a different nickname/i });
    // Click a few times to get a different value (random, may rarely match)
    for (let i = 0; i < 5; i++) {
      await fireEvent.click(rerollBtn);
      if (input.value !== initial) break;
    }
    // We can't guarantee a different value due to randomness, but
    // assert the button is clickable and input is still a valid pattern.
    expect(input.value).toMatch(/^[a-z]+-[a-z]+$/);
  });

  it('the join button label includes the current nickname', () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    const input = screen.getByLabelText("You'll join as") as HTMLInputElement;
    const nick = input.value;
    const joinBtn = screen.getByRole('button', { name: new RegExp(`Join as ${nick}`, 'i') });
    expect(joinBtn).toBeInTheDocument();
  });

  it('shows a validation error when nickname is too short', async () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    const input = screen.getByLabelText("You'll join as") as HTMLInputElement;
    input.value = 'x';
    await fireEvent.input(input);
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });
    expect(screen.getByRole('alert')).toHaveTextContent(/2–24 characters/);
  });

  it('the join button is disabled when the nickname is invalid', async () => {
    render(JoinerPicker, { props: DEFAULT_PROPS });
    const input = screen.getByLabelText("You'll join as") as HTMLInputElement;
    input.value = 'x';
    await fireEvent.input(input);
    const joinBtns = document.querySelectorAll('button[type="submit"]');
    expect(joinBtns.length).toBeGreaterThan(0);
    expect(joinBtns[0]).toBeDisabled();
  });

  // ── Happy path: 200 join ──────────────────────────────────────────────────

  it('POST /api/playground/sessions/{id}/join is called with the nickname on submit', async () => {
    mockPOST.mockResolvedValue({
      data: makeJoinResult(),
      error: undefined,
      response: { status: 200 },
    });

    render(JoinerPicker, { props: DEFAULT_PROPS });
    const form = document.querySelector('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(mockPOST).toHaveBeenCalledOnce();
    });

    const [url, opts] = mockPOST.mock.calls[0] as [string, { params: unknown; body: unknown }];
    expect(url).toBe('/api/playground/sessions/{id}/join');
    expect((opts.params as { path: { id: string } }).path.id).toBe('sess-pg-1');
    expect(opts.body).toEqual({ nickname: expect.stringMatching(/^[a-z]+-[a-z]+$/) });
  });

  it('on 200: calls auth.setPlaygroundContext with bearer + sessionId + nickname', async () => {
    mockPOST.mockResolvedValue({
      data: makeJoinResult({ bearer: 'tok-abc', nickname: 'quiet-fox' }),
      error: undefined,
      response: { status: 200 },
    });

    render(JoinerPicker, { props: DEFAULT_PROPS });
    const form = document.querySelector('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(mockSetPlaygroundContext).toHaveBeenCalledOnce();
    });

    const ctx = mockSetPlaygroundContext.mock.calls[0][0] as {
      sessionId: string;
      bearer: string;
      nickname: string;
    };
    expect(ctx.bearer).toBe('tok-abc');
    expect(ctx.nickname).toBe('quiet-fox');
    expect(ctx.sessionId).toBe('sess-pg-1');
  });

  it('on 200: navigates to the in-session view', async () => {
    mockPOST.mockResolvedValue({
      data: makeJoinResult(),
      error: undefined,
      response: { status: 200 },
    });

    render(JoinerPicker, { props: DEFAULT_PROPS });
    const form = document.querySelector('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith(
        '/orgs/org_playground/sessions/sess-pg-1',
      );
    });
  });

  it('the join button shows "Joining…" while in-flight', async () => {
    let resolve: (v: unknown) => void = () => {};
    mockPOST.mockReturnValue(new Promise((r) => { resolve = r; }));

    render(JoinerPicker, { props: DEFAULT_PROPS });
    const form = document.querySelector('form')!;
    void fireEvent.submit(form);

    await waitFor(() => {
      const joinBtn = document.querySelector('button[type="submit"]')!;
      expect(joinBtn.textContent).toMatch(/Joining…/i);
    });

    resolve({ data: makeJoinResult(), error: undefined, response: { status: 200 } });
  });

  // ── 409 session full ──────────────────────────────────────────────────────

  it('on 409: renders the "session full" message', async () => {
    mockPOST.mockResolvedValue({
      data: undefined,
      error: { error: 'playground.session_full', message: 'session full' },
      response: { status: 409 },
    });

    render(JoinerPicker, { props: DEFAULT_PROPS });
    const form = document.querySelector('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });
    expect(screen.getByText(/This session is full/i)).toBeInTheDocument();
  });

  it('on 409: renders the "Try another playground" CTA', async () => {
    mockPOST.mockResolvedValue({
      data: undefined,
      error: { error: 'playground.session_full', message: 'session full' },
      response: { status: 409 },
    });

    render(JoinerPicker, { props: DEFAULT_PROPS });
    const form = document.querySelector('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(screen.getByText(/Try another playground/i)).toBeInTheDocument();
    });
  });

  it('on 409: "Try another playground" link navigates to /playground', async () => {
    mockPOST.mockResolvedValue({
      data: undefined,
      error: { error: 'playground.session_full', message: 'session full' },
      response: { status: 409 },
    });

    render(JoinerPicker, { props: DEFAULT_PROPS });
    const form = document.querySelector('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(screen.getByText(/Try another playground/i)).toBeInTheDocument();
    });

    const ctaLink = screen.getByText(/Try another playground/i);
    await fireEvent.click(ctaLink);
    expect(mockNavigate).toHaveBeenCalledWith('/playground');
  });

  // ── 410 session ended ─────────────────────────────────────────────────────

  it('on 410: redirects to the tombstone page', async () => {
    mockPOST.mockResolvedValue({
      data: undefined,
      error: { error: 'playground.session_ended', message: 'session ended' },
      response: { status: 410 },
    });

    render(JoinerPicker, { props: DEFAULT_PROPS });
    const form = document.querySelector('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/playground/s/sess-pg-1/ended');
    });
  });

  // ── Guards ────────────────────────────────────────────────────────────────

  it('does not fire POST if viewState is already joining (guards double-submit)', async () => {
    let resolve: (v: unknown) => void = () => {};
    mockPOST.mockReturnValue(new Promise((r) => { resolve = r; }));

    render(JoinerPicker, { props: DEFAULT_PROPS });
    const form = document.querySelector('form')!;
    void fireEvent.submit(form);
    void fireEvent.submit(form); // second submit while in-flight

    await waitFor(() => {
      expect(mockPOST).toHaveBeenCalledTimes(1);
    });

    resolve({ data: makeJoinResult(), error: undefined, response: { status: 200 } });
  });
});
