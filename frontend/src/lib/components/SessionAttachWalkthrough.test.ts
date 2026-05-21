import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/svelte';
import SessionAttachWalkthrough from './SessionAttachWalkthrough.svelte';

// ── Clipboard mock ────────────────────────────────────────────────────────────

const writeText = vi.fn().mockResolvedValue(undefined);

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderWalkthrough(overrides: {
  open?: boolean;
  sessionId?: string | null;
  onclose?: () => void;
  onopenSession?: () => void;
} = {}) {
  return render(SessionAttachWalkthrough, {
    props: {
      open: overrides.open ?? true,
      sessionId: overrides.sessionId !== undefined ? overrides.sessionId : 'test-session-id',
      onclose: overrides.onclose ?? vi.fn(),
      onopenSession: overrides.onopenSession,
    },
  });
}

// ── Setup / teardown ──────────────────────────────────────────────────────────

describe('SessionAttachWalkthrough', () => {
  beforeEach(() => {
    localStorage.clear();
    Object.defineProperty(globalThis.navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });
    writeText.mockClear();
  });

  afterEach(() => cleanup());

  // ── open=false renders nothing ────────────────────────────────────────────

  it('renders nothing when open is false', () => {
    renderWalkthrough({ open: false });
    expect(document.querySelector('.modal-backdrop')).toBeNull();
  });

  // ── full mode when localStorage flag is absent ────────────────────────────

  it('renders full walkthrough when open=true and dismissed flag is absent', () => {
    renderWalkthrough({ open: true });
    expect(document.querySelector('.modal-card.first-time')).toBeInTheDocument();
    expect(document.querySelector('.modal-card.compact')).toBeNull();
  });

  it('renders full walkthrough when open=true and flag is not "true"', () => {
    localStorage.setItem('jamsesh.attach-walkthrough-dismissed', 'false');
    renderWalkthrough({ open: true });
    expect(document.querySelector('.modal-card.first-time')).toBeInTheDocument();
  });

  // ── compact mode when localStorage flag is 'true' ─────────────────────────

  it('renders compact view when dismissed flag is "true"', () => {
    localStorage.setItem('jamsesh.attach-walkthrough-dismissed', 'true');
    renderWalkthrough({ open: true });
    expect(document.querySelector('.modal-card.compact')).toBeInTheDocument();
    expect(document.querySelector('.modal-card.first-time')).toBeNull();
  });

  // ── accessibility attributes ──────────────────────────────────────────────

  it('has correct dialog role and aria attributes', () => {
    renderWalkthrough();
    const dialog = document.querySelector('[role="dialog"]');
    expect(dialog).toBeInTheDocument();
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAttribute('aria-label', 'Attach Claude Code to this jam');
  });

  // ── click-to-copy: shell commands ─────────────────────────────────────────

  it('copies marketplace command when that term-line is clicked', async () => {
    renderWalkthrough({ open: true });
    const lines = document.querySelectorAll('.term-line');
    // First line = marketplace command
    await fireEvent.click(lines[0]);
    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith(
        'claude plugin marketplace add nklisch/jamsesh',
      );
    });
  });

  it('copies install command when second term-line is clicked', async () => {
    renderWalkthrough({ open: true });
    const lines = document.querySelectorAll('.term-line');
    await fireEvent.click(lines[1]);
    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith('claude plugins install jamsesh');
    });
  });

  it('shows "copied" feedback badge on copied term-line', async () => {
    renderWalkthrough({ open: true });
    const lines = document.querySelectorAll('.term-line');
    expect(lines[0]).not.toHaveClass('copied');
    await fireEvent.click(lines[0]);
    await waitFor(() => {
      expect(lines[0]).toHaveClass('copied');
    });
  });

  // ── click-to-copy: CC join command ────────────────────────────────────────

  it('copies join command when cc-input is clicked', async () => {
    renderWalkthrough({ open: true, sessionId: 'my-session-abc' });
    const ccInput = document.querySelector('.cc-input') as HTMLElement;
    await fireEvent.click(ccInput);
    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith('/jamsesh:join my-session-abc');
    });
  });

  it('shows copied class on cc-input after click', async () => {
    renderWalkthrough({ open: true, sessionId: 'my-session-abc' });
    const ccInput = document.querySelector('.cc-input') as HTMLElement;
    expect(ccInput).not.toHaveClass('copied');
    await fireEvent.click(ccInput);
    await waitFor(() => {
      expect(ccInput).toHaveClass('copied');
    });
  });

  // ── "Don't show again" checkbox + close persists flag ────────────────────

  it('writes dismissed flag to localStorage when checkbox is checked and modal is closed via Skip button', async () => {
    const onclose = vi.fn();
    renderWalkthrough({ open: true, onclose });
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement;
    await fireEvent.click(checkbox);
    expect(checkbox.checked).toBe(true);

    const skipBtn = screen.getByRole('button', { name: /skip for now/i });
    await fireEvent.click(skipBtn);

    expect(localStorage.getItem('jamsesh.attach-walkthrough-dismissed')).toBe('true');
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('does NOT write localStorage when checkbox is unchecked and modal is closed', async () => {
    const onclose = vi.fn();
    renderWalkthrough({ open: true, onclose });
    // Checkbox is unchecked by default — just close
    const skipBtn = screen.getByRole('button', { name: /skip for now/i });
    await fireEvent.click(skipBtn);

    expect(localStorage.getItem('jamsesh.attach-walkthrough-dismissed')).toBeNull();
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  // ── ESC key calls onclose ────────────────────────────────────────────────

  it('calls onclose when ESC key is pressed while open', async () => {
    const onclose = vi.fn();
    renderWalkthrough({ open: true, onclose });
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('does not call onclose on ESC when modal is closed', async () => {
    const onclose = vi.fn();
    renderWalkthrough({ open: false, onclose });
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(onclose).not.toHaveBeenCalled();
  });

  // ── Backdrop click calls onclose ─────────────────────────────────────────

  it('calls onclose when backdrop is clicked directly', async () => {
    const onclose = vi.fn();
    renderWalkthrough({ open: true, onclose });
    const backdrop = document.querySelector('.modal-backdrop') as HTMLElement;
    await fireEvent.click(backdrop);
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('does NOT call onclose when clicking inside the modal card', async () => {
    const onclose = vi.fn();
    renderWalkthrough({ open: true, onclose });
    const card = document.querySelector('.modal-card') as HTMLElement;
    await fireEvent.click(card);
    expect(onclose).not.toHaveBeenCalled();
  });

  // ── "Open session view →" button ─────────────────────────────────────────

  it('calls onopenSession when provided and "Open session view" is clicked', async () => {
    const onclose = vi.fn();
    const onopenSession = vi.fn();
    renderWalkthrough({ open: true, onclose, onopenSession });
    const btn = screen.getByRole('button', { name: /open session view/i });
    await fireEvent.click(btn);
    expect(onopenSession).toHaveBeenCalledTimes(1);
    expect(onclose).not.toHaveBeenCalled();
  });

  it('falls back to onclose when onopenSession is not provided and "Open session view" is clicked', async () => {
    const onclose = vi.fn();
    renderWalkthrough({ open: true, onclose });
    const btn = screen.getByRole('button', { name: /open session view/i });
    await fireEvent.click(btn);
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  // ── "First-time setup?" link in compact mode ─────────────────────────────

  it('switches to full mode when "First-time setup?" is clicked in compact', async () => {
    localStorage.setItem('jamsesh.attach-walkthrough-dismissed', 'true');
    renderWalkthrough({ open: true });

    expect(document.querySelector('.modal-card.compact')).toBeInTheDocument();

    const reopenLink = screen.getByText(/first-time setup/i);
    await fireEvent.click(reopenLink);

    await waitFor(() => {
      expect(document.querySelector('.modal-card.first-time')).toBeInTheDocument();
    });
    // localStorage must NOT be cleared — the flag persists
    expect(localStorage.getItem('jamsesh.attach-walkthrough-dismissed')).toBe('true');
  });

  // ── Chrome-help mode (sessionId === null) ─────────────────────────────────

  it('shows placeholder hint in CC pane when sessionId is null', () => {
    renderWalkthrough({ open: true, sessionId: null });
    expect(screen.getByText(/open a session view to copy its join command/i)).toBeInTheDocument();
    // The copyable cc-input with a join command must NOT exist
    const ccInputs = document.querySelectorAll('.cc-input:not(.cc-input--placeholder)');
    expect(ccInputs.length).toBe(0);
  });

  it('still renders copyable shell commands in chrome-help mode', () => {
    renderWalkthrough({ open: true, sessionId: null });
    const termLines = document.querySelectorAll('.term-line');
    // Full mode should still render (no dismissed flag set)
    expect(termLines.length).toBe(2);
  });

  // ── Long command: join command uses full string for copy ──────────────────

  it('copies full join command string (not truncated text) for a long UUID sessionId', async () => {
    const longId = '01926e42-0000-7000-a000-000000000010';
    renderWalkthrough({ open: true, sessionId: longId });
    const ccInput = document.querySelector('.cc-input') as HTMLElement;
    await fireEvent.click(ccInput);
    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith(`/jamsesh:join ${longId}`);
    });
  });

  // ── Close button in compact mode ──────────────────────────────────────────

  it('calls onclose when Close button is clicked in compact mode', async () => {
    localStorage.setItem('jamsesh.attach-walkthrough-dismissed', 'true');
    const onclose = vi.fn();
    renderWalkthrough({ open: true, onclose });
    const closeBtn = screen.getByRole('button', { name: /^close$/i });
    await fireEvent.click(closeBtn);
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  // ── "Open session view" also writes dismiss flag if checked ──────────────

  it('writes dismiss flag if checked when "Open session view" is clicked', async () => {
    const onclose = vi.fn();
    const onopenSession = vi.fn();
    renderWalkthrough({ open: true, onclose, onopenSession });
    const checkbox = screen.getByRole('checkbox');
    await fireEvent.click(checkbox);
    const btn = screen.getByRole('button', { name: /open session view/i });
    await fireEvent.click(btn);
    expect(localStorage.getItem('jamsesh.attach-walkthrough-dismissed')).toBe('true');
    expect(onopenSession).toHaveBeenCalledTimes(1);
  });

  // ── Compact mode in chrome-help (sessionId=null, flag set) ───────────────

  it('shows compact placeholder in CC pane when sessionId is null and flag set', () => {
    localStorage.setItem('jamsesh.attach-walkthrough-dismissed', 'true');
    renderWalkthrough({ open: true, sessionId: null });
    expect(document.querySelector('.modal-card.compact')).toBeInTheDocument();
    expect(screen.getByText(/open a session view to copy its join command/i)).toBeInTheDocument();
  });
});
