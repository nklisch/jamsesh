// SessionsLanding.test.ts
// SessionsLanding renders itself (no children prop needed); Chrome is composed
// internally with a fixed snippet, so we can render it directly.

import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import SessionsLanding from './SessionsLanding.svelte';

// Mock auth so we can spy on signOut.
vi.mock('$lib/auth.svelte', () => ({
  auth: {
    currentUser: null,
    isAuthenticated: true,
    signOut: vi.fn(),
  },
}));

describe('SessionsLanding', () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders the Sessions heading', () => {
    render(SessionsLanding);
    expect(screen.getByRole('heading', { name: 'Sessions' })).toBeInTheDocument();
  });

  it('renders the placeholder message', () => {
    render(SessionsLanding);
    expect(screen.getByText(/No sessions yet/i)).toBeInTheDocument();
  });

  it('wraps content in Chrome — wordmark is present', () => {
    render(SessionsLanding);
    const topbar = document.querySelector('.topbar');
    expect(topbar).not.toBeNull();
    expect(topbar!.textContent).toContain('jam');
  });

  it('passes orgChip="default-org" to Chrome', () => {
    render(SessionsLanding);
    expect(screen.getByText('default-org')).toBeInTheDocument();
  });

  it('renders a sign-out button', () => {
    render(SessionsLanding);
    expect(screen.getByRole('button', { name: /sign out/i })).toBeInTheDocument();
  });

  it('sign-out button calls auth.signOut()', async () => {
    const { auth } = await import('$lib/auth.svelte');
    render(SessionsLanding);

    const signOutBtn = screen.getByRole('button', { name: /sign out/i });
    await fireEvent.click(signOutBtn);

    expect(auth.signOut).toHaveBeenCalledOnce();
  });
});
