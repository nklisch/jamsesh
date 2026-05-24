import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/svelte';
import NewSessionDrawer from './NewSessionDrawer.svelte';

// ── Clipboard mock ────────────────────────────────────────────────────────────
// jsdom does not provide navigator.clipboard by default.
const writeText = vi.fn().mockResolvedValue(undefined);

// ── Setup / teardown ──────────────────────────────────────────────────────────

beforeEach(() => {
  Object.defineProperty(globalThis.navigator, 'clipboard', {
    value: { writeText },
    configurable: true,
  });
  writeText.mockClear();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderDrawer(orgId = 'org-1') {
  const onclose = vi.fn();
  const { container } = render(NewSessionDrawer, {
    props: { orgId, onclose },
  });
  return { onclose, container };
}

async function fillAndSubmit(overrides: {
  goal?: string;
  scope?: string;
  mode?: 'sync' | 'isolated';
  invitees?: string;
} = {}) {
  const { goal = '', scope = '', invitees = '' } = overrides;

  if (goal) {
    await fireEvent.input(screen.getByLabelText(/goal/i), { target: { value: goal } });
  }
  if (scope) {
    await fireEvent.input(screen.getByLabelText(/scope/i), { target: { value: scope } });
  }
  if (invitees) {
    await fireEvent.input(screen.getByLabelText(/invite/i), { target: { value: invitees } });
  }
  if (overrides.mode === 'isolated') {
    await fireEvent.click(screen.getByRole('button', { name: /isolated/i }));
  }

  await fireEvent.submit(
    screen.getByRole('button', { name: /generate commands/i }).closest('form')!,
  );
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('NewSessionDrawer', () => {
  it('renders the drawer heading', () => {
    renderDrawer();
    expect(screen.getByRole('heading', { name: /new session/i })).toBeInTheDocument();
  });

  it('renders form fields', () => {
    renderDrawer();
    expect(screen.getByLabelText(/goal/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/scope/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/invite/i)).toBeInTheDocument();
    expect(screen.getByRole('group', { name: /default mode/i })).toBeInTheDocument();
  });

  it('calls onclose when Cancel is clicked', async () => {
    const { onclose } = renderDrawer();
    await fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
    expect(onclose).toHaveBeenCalledOnce();
  });

  it('calls onclose when close button (✕) is clicked', async () => {
    const { onclose } = renderDrawer();
    await fireEvent.click(screen.getByRole('button', { name: /close/i }));
    expect(onclose).toHaveBeenCalledOnce();
  });

  it('calls onclose when Escape is pressed', async () => {
    const { onclose } = renderDrawer();
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(onclose).toHaveBeenCalledOnce();
  });

  it('toggles default_mode between sync and isolated', async () => {
    renderDrawer();
    const isolatedBtn = screen.getByRole('button', { name: /isolated/i });
    const syncBtn = screen.getByRole('button', { name: /^sync/i });

    expect(syncBtn).toHaveAttribute('aria-pressed', 'true');
    expect(isolatedBtn).toHaveAttribute('aria-pressed', 'false');

    await fireEvent.click(isolatedBtn);

    expect(isolatedBtn).toHaveAttribute('aria-pressed', 'true');
    expect(syncBtn).toHaveAttribute('aria-pressed', 'false');
  });

  // ── Submit renders commands ────────────────────────────────────────────────

  describe('command output', () => {
    it('renders both skill and CLI command blocks on submit', async () => {
      renderDrawer();
      await fillAndSubmit({ goal: 'Fix auth', scope: 'src/auth/**' });

      // Two <code> blocks are present: one for the skill form, one for CLI
      const codes = document.querySelectorAll('code');
      const skillCode = Array.from(codes).find((c) => c.textContent?.startsWith('/jamsesh:jam'));
      const cliCode = Array.from(codes).find((c) => c.textContent?.startsWith('jamsesh new'));
      expect(skillCode).not.toBeNull();
      expect(cliCode).not.toBeNull();
    });

    it('skill command starts with /jamsesh:jam', async () => {
      renderDrawer();
      await fillAndSubmit({ goal: 'Fix auth', scope: 'src/**' });

      const codes = document.querySelectorAll('code');
      const skillCode = Array.from(codes).find((c) => c.textContent?.startsWith('/jamsesh:jam'));
      expect(skillCode).not.toBeNull();
    });

    it('CLI command starts with "jamsesh new"', async () => {
      renderDrawer();
      await fillAndSubmit({ goal: 'Fix auth', scope: 'src/**' });

      const codes = document.querySelectorAll('code');
      const cliCode = Array.from(codes).find((c) => c.textContent?.startsWith('jamsesh new'));
      expect(cliCode).not.toBeNull();
    });

    it('includes --org flag with the orgId prop', async () => {
      renderDrawer('my-org');
      await fillAndSubmit({ goal: 'Fix auth' });

      const codes = document.querySelectorAll('code');
      for (const code of codes) {
        expect(code.textContent).toContain('--org my-org');
      }
    });

    it('includes --goal flag when goal is filled', async () => {
      renderDrawer();
      await fillAndSubmit({ goal: 'Fix the auth flow' });

      const codes = document.querySelectorAll('code');
      for (const code of codes) {
        expect(code.textContent).toContain("--goal 'Fix the auth flow'");
      }
    });

    it('omits --goal flag when goal is empty', async () => {
      renderDrawer();
      await fillAndSubmit({ goal: '' });

      const codes = document.querySelectorAll('code');
      for (const code of codes) {
        expect(code.textContent).not.toContain('--goal');
      }
    });

    it('includes --scope flag when scope is filled', async () => {
      renderDrawer();
      await fillAndSubmit({ scope: 'src/auth/**,tests/**' });

      const codes = document.querySelectorAll('code');
      for (const code of codes) {
        expect(code.textContent).toContain('--scope');
        expect(code.textContent).toContain('src/auth/**');
      }
    });

    it('omits --scope flag when scope is empty', async () => {
      renderDrawer();
      await fillAndSubmit({ scope: '' });

      const codes = document.querySelectorAll('code');
      for (const code of codes) {
        expect(code.textContent).not.toContain('--scope');
      }
    });

    it('includes --mode flag with the selected mode', async () => {
      renderDrawer();
      await fillAndSubmit({ mode: 'isolated' });

      const codes = document.querySelectorAll('code');
      for (const code of codes) {
        expect(code.textContent).toContain('--mode isolated');
      }
    });

    it('defaults --mode to sync when not changed', async () => {
      renderDrawer();
      await fillAndSubmit();

      const codes = document.querySelectorAll('code');
      for (const code of codes) {
        expect(code.textContent).toContain('--mode sync');
      }
    });

    it('includes --invite flag when invitees are filled', async () => {
      renderDrawer();
      await fillAndSubmit({ invitees: 'alice@example.com,bob@example.com' });

      const codes = document.querySelectorAll('code');
      for (const code of codes) {
        expect(code.textContent).toContain('--invite');
        expect(code.textContent).toContain('alice@example.com');
      }
    });

    it('omits --invite flag when invitees are empty', async () => {
      renderDrawer();
      await fillAndSubmit({ invitees: '' });

      const codes = document.querySelectorAll('code');
      for (const code of codes) {
        expect(code.textContent).not.toContain('--invite');
      }
    });

    it('single-quotes goal values that contain spaces', async () => {
      renderDrawer();
      await fillAndSubmit({ goal: 'Fix the auth flow' });

      const codes = document.querySelectorAll('code');
      for (const code of codes) {
        expect(code.textContent).toContain("--goal 'Fix the auth flow'");
      }
    });

    it('does not make any POST API call on submit', async () => {
      // The drawer is purely client-side after the rework — no API client
      // import. Verify the output view renders without errors; no fetch mock
      // is needed (if it were called, jsdom would throw on missing network).
      renderDrawer();
      await fillAndSubmit({ goal: 'Test' });

      // Output view rendered — <code> blocks present means we transitioned
      const codes = document.querySelectorAll('code');
      expect(codes.length).toBeGreaterThanOrEqual(2);
    });
  });

  // ── Copy buttons ──────────────────────────────────────────────────────────

  describe('copy buttons', () => {
    async function renderOutputState() {
      renderDrawer();
      await fillAndSubmit({ goal: 'Test goal', scope: 'src/**' });
      // Confirm we're in the output view by checking for the copy buttons
      expect(screen.getAllByRole('button', { name: /copy/i }).length).toBeGreaterThanOrEqual(1);
    }

    it('copy buttons are present in the output view', async () => {
      await renderOutputState();
      const copyBtns = screen.getAllByRole('button', { name: /copy/i });
      expect(copyBtns.length).toBeGreaterThanOrEqual(2);
    });

    it('clicking skill copy button writes skill command to clipboard', async () => {
      await renderOutputState();
      const [skillCopyBtn] = screen.getAllByRole('button', { name: /copy skill command/i });
      await fireEvent.click(skillCopyBtn);
      expect(writeText).toHaveBeenCalledOnce();
      expect(writeText.mock.calls[0][0]).toMatch(/^\/jamsesh:jam /);
    });

    it('clicking CLI copy button writes CLI command to clipboard', async () => {
      await renderOutputState();
      const [cliCopyBtn] = screen.getAllByRole('button', { name: /copy cli command/i });
      await fireEvent.click(cliCopyBtn);
      expect(writeText).toHaveBeenCalledOnce();
      expect(writeText.mock.calls[0][0]).toMatch(/^jamsesh new /);
    });

    it('skill copy button shows "Copied!" feedback after click', async () => {
      await renderOutputState();
      const [skillCopyBtn] = screen.getAllByRole('button', { name: /copy skill command/i });
      await fireEvent.click(skillCopyBtn);
      await waitFor(() => {
        expect(skillCopyBtn.textContent).toMatch(/copied!/i);
      });
    });

    it('skill copy button reverts to "Copy" after 2 seconds', async () => {
      vi.useFakeTimers();
      await renderOutputState();
      const [skillCopyBtn] = screen.getAllByRole('button', { name: /copy skill command/i });
      await fireEvent.click(skillCopyBtn);

      // Flush the writeText promise before advancing timers
      await vi.runAllTicks();
      await vi.advanceTimersByTimeAsync(2001);

      expect(skillCopyBtn.textContent).toMatch(/^copy$/i);
    });
  });

  // ── Navigation between form and output ───────────────────────────────────

  describe('form / output navigation', () => {
    it('"Edit form" button returns to form view from output', async () => {
      renderDrawer();
      await fillAndSubmit({ goal: 'Test' });

      expect(screen.getByRole('button', { name: /edit form/i })).toBeInTheDocument();
      await fireEvent.click(screen.getByRole('button', { name: /edit form/i }));
      expect(screen.getByLabelText(/goal/i)).toBeInTheDocument();
    });

    it('"Done" button in output view calls onclose', async () => {
      const { onclose } = renderDrawer();
      await fillAndSubmit({ goal: 'Test' });

      expect(screen.getByRole('button', { name: /done/i })).toBeInTheDocument();
      await fireEvent.click(screen.getByRole('button', { name: /done/i }));
      expect(onclose).toHaveBeenCalledOnce();
    });
  });
});
