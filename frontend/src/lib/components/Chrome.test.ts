// Chrome.test.ts
// Uses a companion ChromeTestHarness.svelte to supply the required children
// snippet inline — the cleanest pattern for Svelte 5 snippet props in Vitest.
// (The `children: () => 'string'` pattern used in sibling design-system tests
// is a known bug; our tests use the correct approach.)

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ChromeTestHarness from './ChromeTestHarness.svelte';

// We mock auth.svelte so we can control currentUser without touching localStorage.
vi.mock('$lib/auth.svelte', () => {
  let _currentUser: { id: string; email: string; displayName: string } | null = null;
  return {
    auth: {
      get currentUser() { return _currentUser; },
      get isAuthenticated() { return true; },
      _setUser(u: typeof _currentUser) { _currentUser = u; },
    },
  };
});

describe('Chrome', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('always renders the wordmark', () => {
    render(ChromeTestHarness);
    // The wordmark text is split: "jam·sesh" — check for "jam" and "sesh" parts
    const topbar = document.querySelector('.topbar');
    expect(topbar).not.toBeNull();
    expect(topbar!.textContent).toContain('jam');
    expect(topbar!.textContent).toContain('sesh');
  });

  it('renders the body slot content', () => {
    render(ChromeTestHarness);
    expect(screen.getByTestId('chrome-body')).toBeInTheDocument();
    expect(screen.getByTestId('chrome-body')).toHaveTextContent('body content');
  });

  it('renders orgChip when provided', () => {
    render(ChromeTestHarness, { props: { orgChip: 'acme' } });
    expect(screen.getByText('acme')).toBeInTheDocument();
  });

  it('does not render breadcrumb when orgChip is omitted', () => {
    render(ChromeTestHarness);
    // No chip elements should be present
    const chips = document.querySelectorAll('.chip');
    expect(chips.length).toBe(0);
  });

  it('renders sessionChip alongside orgChip when both provided', () => {
    render(ChromeTestHarness, { props: { orgChip: 'acme', sessionChip: 'sess-1' } });
    expect(screen.getByText('acme')).toBeInTheDocument();
    expect(screen.getByText('sess-1')).toBeInTheDocument();
  });

  it('does not render sessionChip when only orgChip provided', () => {
    render(ChromeTestHarness, { props: { orgChip: 'acme' } });
    const chips = document.querySelectorAll('.chip');
    expect(chips.length).toBe(1);
  });

  it('renders AuthorDot when auth.currentUser is set', async () => {
    // Access the mocked auth to set a user
    const { auth } = await import('$lib/auth.svelte');
    (auth as any)._setUser({ id: 'user-1', email: 'a@b.com', displayName: 'Alice' });

    render(ChromeTestHarness);
    // AuthorDot renders as a <span role="img">
    const dot = document.querySelector('[role="img"]');
    expect(dot).not.toBeNull();
    expect(dot!.getAttribute('aria-label')).toBe('Alice');
  });

  it('does not render AuthorDot when auth.currentUser is null', async () => {
    const { auth } = await import('$lib/auth.svelte');
    (auth as any)._setUser(null);

    render(ChromeTestHarness);
    const dot = document.querySelector('[role="img"]');
    expect(dot).toBeNull();
  });

  it('always renders ThemeToggle', () => {
    render(ChromeTestHarness);
    // ThemeToggle renders as a button with aria-label matching theme labels
    const toggles = document.querySelectorAll('.theme-toggle');
    expect(toggles.length).toBeGreaterThan(0);
  });
});
