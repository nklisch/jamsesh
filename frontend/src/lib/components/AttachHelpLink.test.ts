import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/svelte';
import AttachHelpLink from './AttachHelpLink.svelte';

// ── Clipboard mock ────────────────────────────────────────────────────────────
// Required because jsdom does not provide navigator.clipboard by default.
const writeText = vi.fn().mockResolvedValue(undefined);

// ── Setup / teardown ──────────────────────────────────────────────────────────

beforeEach(() => {
  localStorage.clear();
  Object.defineProperty(globalThis.navigator, 'clipboard', {
    value: { writeText },
    configurable: true,
  });
  writeText.mockClear();
});

afterEach(() => cleanup());

// ── Helper ────────────────────────────────────────────────────────────────────

function renderLink(overrides: {
  sessionId?: string | null;
  variant?: 'inline' | 'icon';
} = {}) {
  return render(AttachHelpLink, {
    props: {
      sessionId: overrides.sessionId !== undefined ? overrides.sessionId : 'test-session-abc',
      variant: overrides.variant,
    },
  });
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('AttachHelpLink', () => {
  // ── Inline variant (default) ──────────────────────────────────────────────

  it('renders a button in inline mode by default', () => {
    renderLink();
    const btn = screen.getByRole('button', { name: /setup help/i });
    expect(btn).toBeInTheDocument();
    expect(btn.textContent).toMatch(/setup help/i);
  });

  it('renders a button in inline mode when variant="inline" is explicit', () => {
    renderLink({ variant: 'inline' });
    const btn = screen.getByRole('button', { name: /setup help/i });
    expect(btn).toBeInTheDocument();
  });

  it('walkthrough is not in the DOM before clicking', () => {
    renderLink();
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  it('clicking the inline link opens the walkthrough dialog', async () => {
    renderLink();
    const btn = screen.getByRole('button', { name: /setup help/i });
    await fireEvent.click(btn);
    expect(document.querySelector('[role="dialog"]')).not.toBeNull();
  });

  it('closing the walkthrough via ESC removes the dialog from the DOM', async () => {
    renderLink();
    const btn = screen.getByRole('button', { name: /setup help/i });
    await fireEvent.click(btn);
    expect(document.querySelector('[role="dialog"]')).not.toBeNull();

    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  it('closing the walkthrough via backdrop click removes the dialog from the DOM', async () => {
    renderLink();
    const btn = screen.getByRole('button', { name: /setup help/i });
    await fireEvent.click(btn);

    // The dialog landmark is now on the inner <article class="modal-card">;
    // .modal-backdrop is the click-to-dismiss scrim
    // (idea-attach-onboarding-dialog-role-on-card). Test the scrim click.
    const dialog = document.querySelector('[role="dialog"]');
    expect(dialog).not.toBeNull();
    const backdrop = document.querySelector('.modal-backdrop') as HTMLElement | null;
    expect(backdrop).not.toBeNull();
    await fireEvent.click(backdrop!);
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  // ── Icon variant ──────────────────────────────────────────────────────────

  it('renders a 28×28 icon-only button in icon mode', () => {
    renderLink({ variant: 'icon' });
    const btn = screen.getByRole('button', { name: /setup help/i });
    expect(btn).toBeInTheDocument();
    // The inline text label should NOT be present in icon variant
    expect(btn.textContent?.trim()).not.toMatch(/setup help/i);
  });

  it('clicking the icon button opens the walkthrough dialog', async () => {
    renderLink({ variant: 'icon' });
    const btn = screen.getByRole('button', { name: /setup help/i });
    await fireEvent.click(btn);
    expect(document.querySelector('[role="dialog"]')).not.toBeNull();
  });

  // ── sessionId forwarding ──────────────────────────────────────────────────

  it('forwards a specific sessionId to the walkthrough (session shown in eyebrow)', async () => {
    const sid = 'session-uuid-12345';
    renderLink({ sessionId: sid });
    const btn = screen.getByRole('button', { name: /setup help/i });
    await fireEvent.click(btn);

    // The walkthrough renders the session id in the eyebrow and in the CC pane.
    // Use getAllByText since the id appears in multiple elements (eyebrow + cc-cmd).
    const matches = screen.getAllByText(new RegExp(sid));
    expect(matches.length).toBeGreaterThan(0);
  });

  it('forwards sessionId=null — walkthrough shows chrome-help placeholder, not a session id', async () => {
    renderLink({ sessionId: null });
    const btn = screen.getByRole('button', { name: /setup help/i });
    await fireEvent.click(btn);

    // With sessionId=null the eyebrow reads "Claude Code setup", not a session id.
    expect(screen.getByText(/claude code setup/i)).toBeInTheDocument();
    // The CC pane shows the placeholder prompt, not a copyable join command.
    expect(
      screen.getByText(/open a session view to copy its join command/i),
    ).toBeInTheDocument();
  });
});
