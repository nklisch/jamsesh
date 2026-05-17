import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { fireEvent } from '@testing-library/svelte';
import RefActionsMenu from './RefActionsMenu.svelte';

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderMenu(
  overrides: {
    ref?: string;
    x?: number;
    y?: number;
    onaction?: (action: string, ref: string) => void;
    onclose?: () => void;
  } = {},
) {
  return render(RefActionsMenu, {
    props: {
      ref: overrides.ref ?? 'refs/heads/jam/sess-1/alice/main',
      x: overrides.x ?? 100,
      y: overrides.y ?? 200,
      onaction: overrides.onaction,
      onclose: overrides.onclose,
    },
  });
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('RefActionsMenu', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders the Fork… menu item', () => {
    renderMenu();
    expect(screen.getByRole('menuitem', { name: /fork/i })).toBeInTheDocument();
  });

  it('renders the Switch mode… menu item', () => {
    renderMenu();
    expect(screen.getByRole('menuitem', { name: /switch mode/i })).toBeInTheDocument();
  });

  it('shows abbreviated ref label in header', () => {
    renderMenu({ ref: 'refs/heads/jam/sess-1/alice/main' });
    expect(screen.getByTitle('refs/heads/jam/sess-1/alice/main')).toBeInTheDocument();
  });

  it('calls onaction with fork and closes on Fork click', async () => {
    const onaction = vi.fn();
    const onclose = vi.fn();
    const ref = 'refs/heads/jam/sess-1/alice/main';
    renderMenu({ ref, onaction, onclose });

    await fireEvent.click(screen.getByRole('menuitem', { name: /fork/i }));

    expect(onaction).toHaveBeenCalledWith('fork', ref);
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('calls onaction with mode-switch and closes on Switch mode click', async () => {
    const onaction = vi.fn();
    const onclose = vi.fn();
    const ref = 'refs/heads/jam/sess-1/alice/main';
    renderMenu({ ref, onaction, onclose });

    await fireEvent.click(screen.getByRole('menuitem', { name: /switch mode/i }));

    expect(onaction).toHaveBeenCalledWith('mode-switch', ref);
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('calls onclose when backdrop is clicked', async () => {
    const onclose = vi.fn();
    renderMenu({ onclose });

    const backdrop = document.querySelector('.backdrop') as HTMLElement;
    await fireEvent.click(backdrop);

    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('calls onclose when Escape key is pressed on backdrop', async () => {
    const onclose = vi.fn();
    renderMenu({ onclose });

    const backdrop = document.querySelector('.backdrop') as HTMLElement;
    await fireEvent.keyDown(backdrop, { key: 'Escape' });

    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('positions menu at given x/y', () => {
    renderMenu({ x: 150, y: 300 });
    const menu = document.querySelector('.menu') as HTMLElement;
    expect(menu.style.left).toBe('150px');
    expect(menu.style.top).toBe('300px');
  });
});
