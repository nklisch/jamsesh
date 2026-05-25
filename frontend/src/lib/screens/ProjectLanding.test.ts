// ProjectLanding.test.ts
// Tests: renders mockup structure (numbered sections, hero CTA, schematic,
// method steps, colophon); internal navigation links call navigate(); external
// links use plain hrefs; GitHub icon is present.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/svelte';
import ProjectLanding from './ProjectLanding.svelte';

// ── Module mocks ─────────────────────────────────────────────────────────────

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  navigate: (...args: unknown[]) => mockNavigate(...args),
  current: { name: 'home', params: {}, requiresAuth: true },
}));

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('ProjectLanding', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  // ── Static structure ───────────────────────────────────────────────────────

  it('renders the jamsesh wordmark', () => {
    render(ProjectLanding);
    const wordmark = document.querySelector('.wordmark');
    expect(wordmark).toBeInTheDocument();
    expect(wordmark!.textContent).toMatch(/jamsesh/);
  });

  it('renders the hero eyebrow', () => {
    render(ProjectLanding);
    expect(screen.getByText(/For teams jamming on code, specs, docs/i)).toBeInTheDocument();
  });

  it('renders the hero h1', () => {
    render(ProjectLanding);
    expect(screen.getByRole('heading', { level: 1 })).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1 }).textContent).toMatch(/Your team on a call/i);
  });

  it('renders the hero lede paragraph', () => {
    render(ProjectLanding);
    expect(screen.getByText(/Each person drives their own Claude Code agent/i)).toBeInTheDocument();
  });

  it('renders the section 01 number label', () => {
    render(ProjectLanding);
    expect(screen.getByText(/01/)).toBeInTheDocument();
  });

  it('renders the section 02 — SCHEMATIC label', () => {
    render(ProjectLanding);
    expect(screen.getByText(/SCHEMATIC/i)).toBeInTheDocument();
  });

  it('renders the schematic body text', () => {
    render(ProjectLanding);
    expect(screen.getByText(/Three humans, three agents/i)).toBeInTheDocument();
  });

  it('renders the schematic diagram lines', () => {
    render(ProjectLanding);
    expect(screen.getByText(/refines auth flow/i)).toBeInTheDocument();
    expect(screen.getByText(/adds magic-link spec/i)).toBeInTheDocument();
    expect(screen.getByText(/amber \+ teal-fox into draft/i)).toBeInTheDocument();
  });

  it('renders the section 03 — METHOD label', () => {
    render(ProjectLanding);
    expect(screen.getByText(/METHOD/i)).toBeInTheDocument();
  });

  it('renders all three method steps', () => {
    render(ProjectLanding);
    expect(screen.getByText(/A · VOICE-PACED/i)).toBeInTheDocument();
    expect(screen.getByText(/B · EVERYONE IN THE LOOP/i)).toBeInTheDocument();
    expect(screen.getByText(/C · REAL GIT UNDERNEATH/i)).toBeInTheDocument();
  });

  it('renders method step headings', () => {
    render(ProjectLanding);
    expect(screen.getByText(/Talk through direction/i)).toBeInTheDocument();
    expect(screen.getByText(/Comments cross the boundary/i)).toBeInTheDocument();
    expect(screen.getByText(/Nothing parallel to learn/i)).toBeInTheDocument();
  });

  it('renders the colophon with version and license', () => {
    // Version is sourced from __APP_VERSION__ (Vite `define` → package.json),
    // so the assertion uses a semver-ish regex rather than the literal —
    // future package.json bumps don't require a test edit.
    // (gate-tests-projectlanding-hardcoded-version-string)
    render(ProjectLanding);
    expect(
      screen.getByText(/jamsesh \/ Apache-2\.0 \/ v?\d+\.\d+\.\d+ \/ 2026/i),
    ).toBeInTheDocument();
  });

  // ── Internal navigation ────────────────────────────────────────────────────

  it('"Try the playground →" click calls navigate("/playground")', async () => {
    render(ProjectLanding);
    // Find the CTA link by its text content.
    const ctaLink = screen.getByText(/Try the playground →/i);
    await fireEvent.click(ctaLink);
    expect(mockNavigate).toHaveBeenCalledWith('/playground');
  });

  it('"Sign in →" link in top bar calls navigate("/login")', async () => {
    render(ProjectLanding);
    // There are multiple "Sign in" links (topbar + colophon); get the first.
    const signinLinks = screen.getAllByText(/Sign in/i);
    expect(signinLinks.length).toBeGreaterThan(0);
    await fireEvent.click(signinLinks[0]);
    expect(mockNavigate).toHaveBeenCalledWith('/login');
  });

  it('wordmark click calls navigate("/")', async () => {
    render(ProjectLanding);
    const wordmark = document.querySelector('.wordmark') as HTMLElement;
    await fireEvent.click(wordmark);
    expect(mockNavigate).toHaveBeenCalledWith('/');
  });

  // ── External links ─────────────────────────────────────────────────────────

  it('GitHub icon link has the correct href and target="_blank"', () => {
    render(ProjectLanding);
    const githubLink = document.querySelector('a[aria-label="GitHub repository"]') as HTMLAnchorElement;
    expect(githubLink).toBeInTheDocument();
    expect(githubLink.href).toContain('github.com/nklisch/jamsesh');
    expect(githubLink.target).toBe('_blank');
  });

  it('renders the GitHub SVG icon inside the GitHub link', () => {
    render(ProjectLanding);
    const githubLink = document.querySelector('a[aria-label="GitHub repository"]');
    const svg = githubLink?.querySelector('svg');
    expect(svg).toBeInTheDocument();
  });

  it('Self-host link in top nav points to the correct SELF_HOST.md URL', () => {
    render(ProjectLanding);
    const selfHostLinks = document.querySelectorAll(
      'a[href="https://github.com/nklisch/jamsesh/blob/main/docs/SELF_HOST.md"]',
    );
    expect(selfHostLinks.length).toBeGreaterThan(0);
  });
});
