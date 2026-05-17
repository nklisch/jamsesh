import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import { fireEvent } from '@testing-library/svelte';
import ModeSwitchDialog from './ModeSwitchDialog.svelte';

// ── Module mocks ─────────────────────────────────────────────────────────────

const mockPOST = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: { POST: (...args: unknown[]) => mockPOST(...args) },
}));

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderDialog(
  overrides: {
    currentMode?: 'sync' | 'isolated';
    ref?: string;
    onclose?: () => void;
    onsuccess?: () => void;
  } = {},
) {
  return render(ModeSwitchDialog, {
    props: {
      orgId: 'org-1',
      sessionId: 'sess-1',
      ref: overrides.ref ?? 'refs/heads/jam/sess-1/alice/main',
      currentMode: overrides.currentMode,
      onclose: overrides.onclose,
      onsuccess: overrides.onsuccess,
    },
  });
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('ModeSwitchDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders dialog with Switch mode title', () => {
    renderDialog();
    expect(screen.getByRole('dialog', { name: /switch ref mode/i })).toBeInTheDocument();
  });

  it('shows current mode badge', () => {
    renderDialog({ currentMode: 'sync' });
    // The mode badge showing the current mode
    const badge = document.querySelector('.mode-sync');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveTextContent('sync');
  });

  it('shows abbreviated ref in field', () => {
    renderDialog({ ref: 'refs/heads/jam/sess-1/alice/main' });
    expect(screen.getByTitle('refs/heads/jam/sess-1/alice/main')).toBeInTheDocument();
  });

  it('defaults to isolated when current mode is sync', () => {
    renderDialog({ currentMode: 'sync' });
    const isolated = screen.getByDisplayValue('isolated') as HTMLInputElement;
    expect(isolated.checked).toBe(true);
  });

  it('defaults to sync when current mode is isolated', () => {
    renderDialog({ currentMode: 'isolated' });
    const sync = screen.getByDisplayValue('sync') as HTMLInputElement;
    expect(sync.checked).toBe(true);
  });

  it('submit button is disabled when selected mode equals current mode', async () => {
    renderDialog({ currentMode: 'sync' });
    // Switch back to sync (which is the current mode)
    const syncRadio = screen.getByDisplayValue('sync') as HTMLInputElement;
    await fireEvent.click(syncRadio);
    const btn = screen.getByRole('button', { name: /switch mode/i });
    expect(btn).toBeDisabled();
  });

  it('submit button is enabled when selected mode differs from current', () => {
    renderDialog({ currentMode: 'sync' });
    // default selection is isolated (opposite of sync) — so button should be enabled
    const btn = screen.getByRole('button', { name: /switch mode/i });
    expect(btn).not.toBeDisabled();
  });

  it('calls onclose when Close button clicked', async () => {
    const onclose = vi.fn();
    renderDialog({ onclose });
    await fireEvent.click(screen.getByLabelText(/^close$/i));
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('calls onclose when Cancel button clicked', async () => {
    const onclose = vi.fn();
    renderDialog({ onclose });
    await fireEvent.click(screen.getByRole('button', { name: /^cancel$/i }));
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('submits mode change and calls onsuccess + onclose', async () => {
    mockPOST.mockResolvedValue({ data: {}, error: null });
    const onsuccess = vi.fn();
    const onclose = vi.fn();
    renderDialog({ currentMode: 'sync', onsuccess, onclose });

    await fireEvent.click(screen.getByRole('button', { name: /switch mode/i }));

    await waitFor(() => {
      expect(mockPOST).toHaveBeenCalledWith(
        '/api/orgs/{orgID}/sessions/{sessionID}/ref-modes',
        expect.objectContaining({
          params: { path: { orgID: 'org-1', sessionID: 'sess-1' } },
          body: expect.objectContaining({ ref: 'refs/heads/jam/sess-1/alice/main', mode: 'isolated' }),
        }),
      );
      expect(onsuccess).toHaveBeenCalledTimes(1);
      expect(onclose).toHaveBeenCalledTimes(1);
    });
  });

  it('shows error alert when POST fails', async () => {
    mockPOST.mockResolvedValue({ data: null, error: { message: 'bad' } });
    renderDialog({ currentMode: 'sync' });

    await fireEvent.click(screen.getByRole('button', { name: /switch mode/i }));

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
      expect(screen.getByRole('alert')).toHaveTextContent(/failed to update mode/i);
    });
  });
});
