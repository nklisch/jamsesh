// Login.test.ts
// Tests login mode transitions, OAuth redirect, and magic-link fetch.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Login from './Login.svelte';

describe('Login', () => {
  beforeEach(() => {
    // Reset location.assign spy before each test
    vi.spyOn(window, 'location', 'get').mockReturnValue({
      ...window.location,
      assign: vi.fn(),
    } as any);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders in choose mode by default', () => {
    render(Login);
    expect(screen.getByText('Sign in to jamsesh')).toBeInTheDocument();
    expect(screen.getByText('Continue with GitHub')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('you@example.com')).toBeInTheDocument();
  });

  it('shows the "or" divider in choose mode', () => {
    render(Login);
    expect(screen.getByText('or')).toBeInTheDocument();
  });

  it('OAuth button posts to /api/auth/oauth/start and assigns the returned authorize_url', async () => {
    const assignSpy = vi.fn();
    Object.defineProperty(window, 'location', {
      value: { ...window.location, assign: assignSpy },
      writable: true,
      configurable: true,
    });

    let captured: Request | null = null;
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      captured = input as Request;
      return new Response(
        JSON.stringify({
          authorize_url: 'https://github.com/login/oauth/authorize?state=abc',
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });

    render(Login);
    const githubBtn = screen.getByText('Continue with GitHub').closest('button')!;
    await fireEvent.click(githubBtn);

    await waitFor(() => {
      expect(assignSpy).toHaveBeenCalledWith(
        'https://github.com/login/oauth/authorize?state=abc',
      );
    });

    expect(fetchSpy).toHaveBeenCalled();
    expect(captured).not.toBeNull();
    expect(captured!.url).toMatch(/\/api\/auth\/oauth\/start$/);
    expect(captured!.method).toBe('POST');
    const body = await captured!.clone().json();
    expect(body).toEqual({ provider: 'github' });
  });

  it('OAuth button shows an error when the start call fails', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({ error: 'oauth.provider_not_configured', message: 'no github' }),
        { status: 503, headers: { 'Content-Type': 'application/json' } },
      ),
    );

    render(Login);
    const githubBtn = screen.getByText('Continue with GitHub').closest('button')!;
    await fireEvent.click(githubBtn);

    await waitFor(() => {
      expect(screen.getByText('Something went wrong')).toBeInTheDocument();
    });
    expect(screen.getByText(/Could not start GitHub sign-in/)).toBeInTheDocument();
  });

  it('submitting the magic-link form with 2xx response transitions to magic-link-sent', async () => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true } as Response);

    render(Login);
    const emailInput = screen.getByPlaceholderText('you@example.com') as HTMLInputElement;
    emailInput.value = 'test@example.com';
    await fireEvent.input(emailInput);

    const form = emailInput.closest('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(screen.getByText('Check your inbox')).toBeInTheDocument();
    });
  });

  it('posts to /api/auth/magic-link/request with email in body', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true } as Response);
    global.fetch = fetchMock;

    render(Login);
    const emailInput = screen.getByPlaceholderText('you@example.com') as HTMLInputElement;

    // Svelte 5 bind:value responds to the native 'input' event — use fireEvent.input
    // then also dispatch 'change' to ensure the value is committed before submit.
    emailInput.value = 'user@acme.com';
    await fireEvent.input(emailInput);

    const form = emailInput.closest('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/auth/magic-link/request',
        expect.objectContaining({
          method: 'POST',
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({ email: 'user@acme.com' }),
        }),
      );
    });
  });

  it('transitions to magic-link-error on non-2xx response', async () => {
    global.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 } as Response);

    render(Login);
    const form = screen.getByPlaceholderText('you@example.com').closest('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(screen.getByText('Something went wrong')).toBeInTheDocument();
    });
  });

  it('error state shows a Try again button that returns to choose mode', async () => {
    global.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 } as Response);

    render(Login);
    const form = screen.getByPlaceholderText('you@example.com').closest('form')!;
    await fireEvent.submit(form);

    await waitFor(() => screen.getByText('Something went wrong'));

    const tryAgainBtn = screen.getByRole('button', { name: /try again/i });
    await fireEvent.click(tryAgainBtn);

    expect(screen.getByText('Sign in to jamsesh')).toBeInTheDocument();
  });

  it('magic-link-sent state shows the email address and back affordance', async () => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true } as Response);

    render(Login);
    const emailInput = screen.getByPlaceholderText('you@example.com') as HTMLInputElement;
    emailInput.value = 'sent@example.com';
    await fireEvent.input(emailInput);

    const form = emailInput.closest('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(screen.getByText('Check your inbox')).toBeInTheDocument();
    });

    expect(screen.getByText(/Try a different email/i)).toBeInTheDocument();
  });
});
