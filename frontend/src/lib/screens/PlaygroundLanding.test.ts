// PlaygroundLanding.test.ts
// Tests: renders landing content, copy buttons, navigation links.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/svelte';
import PlaygroundLanding from './PlaygroundLanding.svelte';

// ── Module mocks ────────────────────────────────────────────────────────────

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  navigate: (...args: unknown[]) => mockNavigate(...args),
  current: { name: 'playground', params: {}, requiresAuth: false },
}));

// ── Tests ────────────────────────────────────────────────────────────────────

describe('PlaygroundLanding', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Provide a clipboard stub so copy tests don't throw.
    Object.defineProperty(globalThis.navigator, 'clipboard', {
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
      configurable: true,
      writable: true,
    });
  });

  afterEach(() => {
    cleanup();
  });

  // ── Static content ────────────────────────────────────────────────────────

  it('renders the jamsesh wordmark', () => {
    render(PlaygroundLanding);
    const wordmark = document.querySelector('.wordmark');
    expect(wordmark).toBeInTheDocument();
    expect(wordmark!.textContent).toMatch(/jamsesh/);
  });

  it('renders the hero eyebrow "Playground · no account needed"', () => {
    render(PlaygroundLanding);
    expect(screen.getByText(/Playground · no account needed/i)).toBeInTheDocument();
  });

  it('renders the hero headline', () => {
    render(PlaygroundLanding);
    expect(screen.getByText(/Run a multi-agent jam/i)).toBeInTheDocument();
  });

  it('renders the lede paragraph', () => {
    render(PlaygroundLanding);
    expect(screen.getByText(/throwaway session/i)).toBeInTheDocument();
  });

  it('renders step 1 with the install command', () => {
    render(PlaygroundLanding);
    expect(screen.getByText(/claude plugin marketplace add/i)).toBeInTheDocument();
  });

  it('renders step 2 with the playground new command', () => {
    render(PlaygroundLanding);
    // The command appears in two places (cmd-box code + inline hint), so use getAllByText.
    const matches = screen.getAllByText(/jamsesh playground new/i);
    expect(matches.length).toBeGreaterThan(0);
  });

  it('renders the ephemeral sessions note', () => {
    render(PlaygroundLanding);
    expect(screen.getByText(/Ephemeral/)).toBeInTheDocument();
  });

  it('renders "Real git", "Auto-merger", and "Addressed comments" feature cards', () => {
    render(PlaygroundLanding);
    expect(screen.getByText('Real git')).toBeInTheDocument();
    expect(screen.getByText('Auto-merger')).toBeInTheDocument();
    expect(screen.getByText('Addressed comments')).toBeInTheDocument();
  });

  it('renders the sign-up footer link', () => {
    render(PlaygroundLanding);
    expect(screen.getByText(/Sign up for a durable account|Sign up →/i)).toBeInTheDocument();
  });

  // ── Navigation ────────────────────────────────────────────────────────────

  it('clicking "Sign in →" navigates to /login', async () => {
    render(PlaygroundLanding);
    const link = screen.getByText(/sign in →/i);
    await fireEvent.click(link);
    expect(mockNavigate).toHaveBeenCalledWith('/login');
  });

  it('clicking the sign-up footer link navigates to /', async () => {
    render(PlaygroundLanding);
    // The footer anchor text varies; match by href
    const footerLinks = document.querySelectorAll('.doc-foot a');
    expect(footerLinks.length).toBeGreaterThan(0);
    await fireEvent.click(footerLinks[footerLinks.length - 1]);
    expect(mockNavigate).toHaveBeenCalledWith('/');
  });

  // ── Copy buttons ──────────────────────────────────────────────────────────

  it('renders two Copy buttons (one per command)', () => {
    render(PlaygroundLanding);
    const copyBtns = screen.getAllByText('Copy');
    expect(copyBtns).toHaveLength(2);
  });

  it('clicking a Copy button calls navigator.clipboard.writeText', async () => {
    render(PlaygroundLanding);
    const [firstCopy] = screen.getAllByText('Copy');
    await fireEvent.click(firstCopy);
    expect(navigator.clipboard.writeText).toHaveBeenCalledOnce();
    expect((navigator.clipboard.writeText as ReturnType<typeof vi.fn>).mock.calls[0][0]).toContain(
      'claude plugin marketplace add',
    );
  });

  it('Copy button label changes to "Copied!" immediately after click', async () => {
    vi.useFakeTimers();
    render(PlaygroundLanding);
    const [firstCopy] = screen.getAllByText('Copy');
    await fireEvent.click(firstCopy);
    // After click the label should be "Copied!"
    expect(firstCopy.textContent).toBe('Copied!');
    vi.useRealTimers();
  });
});
