// ForkDialog.test.ts
// Tests that the refs fetch uses the real org id, and that a failed or unresolved
// ref surfaces an error and does NOT issue the fork.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import ForkDialog from './ForkDialog.svelte';

// ── Mock auth ────────────────────────────────────────────────────────────────

// Note: vi.mock factories are hoisted to the top of the file, so we cannot
// reference a module-level const here. Use a literal string and mirror it below.
vi.mock('$lib/auth.svelte', () => ({
  auth: {
    token: 'test-bearer-token-abc123',
    isAuthenticated: true,
    currentUser: { id: 'user-1', email: 'test@example.com', displayName: 'Test' },
  },
}));

// Mirror the literal above so assertions can reference it without duplication.
const TEST_TOKEN = 'test-bearer-token-abc123';

// ── Mock the API client ──────────────────────────────────────────────────────

const mockGET = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    GET: (...args: unknown[]) => mockGET(...args),
  },
}));

// ── Default props ─────────────────────────────────────────────────────────────

function renderDialog(overrides: {
  orgId?: string;
  sessionId?: string;
  sourceRef?: string;
  onclose?: () => void;
  onsuccess?: () => void;
} = {}) {
  return render(ForkDialog, {
    props: {
      orgId: overrides.orgId ?? 'org-real',
      sessionId: overrides.sessionId ?? 'sess-1',
      sourceRef: overrides.sourceRef ?? 'refs/heads/jam/sess-1/user/main',
      onclose: overrides.onclose,
      onsuccess: overrides.onsuccess,
    },
  });
}

// Click the "Fork" submit button to trigger form submission.
async function clickFork() {
  const forkBtn = screen.getByRole('button', { name: /^Fork$/i });
  await fireEvent.click(forkBtn);
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('ForkDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default: stub global fetch to return empty MCP success
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      text: async () => JSON.stringify({ result: {} }),
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it('refs fetch uses the real org id in the path params', async () => {
    const refsResponse = {
      refs: [{ ref: 'refs/heads/jam/sess-1/user/main', sha: 'abc123' }],
    };
    mockGET.mockResolvedValueOnce({ data: refsResponse, error: null });

    renderDialog({ orgId: 'org-real', sessionId: 'sess-1' });

    await clickFork();

    await waitFor(() => {
      expect(mockGET).toHaveBeenCalledWith(
        '/api/orgs/{orgID}/sessions/{sessionID}/refs',
        expect.objectContaining({
          params: { path: { orgID: 'org-real', sessionID: 'sess-1' } },
        }),
      );
    });
  });

  it('refs fetch failure surfaces an error and does NOT issue the fork', async () => {
    mockGET.mockResolvedValueOnce({ data: null, error: { message: 'network error' } });

    renderDialog();

    await clickFork();

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });

    // The /mcp fetch must NOT have been called
    expect(vi.mocked(fetch)).not.toHaveBeenCalled();
    expect(screen.getByRole('alert').textContent).toMatch(/could not fetch refs/i);
  });

  it('unresolved source ref surfaces an error and does NOT issue the fork', async () => {
    // Refs response does not include the sourceRef
    const refsResponse = {
      refs: [{ ref: 'refs/heads/jam/sess-1/other-user/branch', sha: 'xyz999' }],
    };
    mockGET.mockResolvedValueOnce({ data: refsResponse, error: null });

    renderDialog({ sourceRef: 'refs/heads/jam/sess-1/user/main' });

    await clickFork();

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });

    // The /mcp fetch must NOT have been called
    expect(vi.mocked(fetch)).not.toHaveBeenCalled();
    expect(screen.getByRole('alert').textContent).toMatch(/could not be resolved/i);
  });

  it('resolved ref issues the fork with target_commit_sha', async () => {
    const refsResponse = {
      refs: [{ ref: 'refs/heads/jam/sess-1/user/main', sha: 'deadbeef' }],
    };
    mockGET.mockResolvedValueOnce({ data: refsResponse, error: null });

    const onsuccess = vi.fn();
    renderDialog({
      orgId: 'org-real',
      sessionId: 'sess-1',
      sourceRef: 'refs/heads/jam/sess-1/user/main',
      onsuccess,
    });

    await clickFork();

    await waitFor(() => {
      expect(onsuccess).toHaveBeenCalledOnce();
    });

    // Check the MCP fetch was called with target_commit_sha
    const fetchCalls = vi.mocked(fetch).mock.calls;
    expect(fetchCalls.length).toBeGreaterThan(0);
    const [_url, options] = fetchCalls[0] as [string, RequestInit];
    const body = JSON.parse(options.body as string) as {
      params: { arguments: { target_commit_sha?: string } };
    };
    expect(body.params.arguments.target_commit_sha).toBe('deadbeef');
  });

  it('fork POST to /mcp carries Authorization: Bearer header', async () => {
    const refsResponse = {
      refs: [{ ref: 'refs/heads/jam/sess-1/user/main', sha: 'cafebabe' }],
    };
    mockGET.mockResolvedValueOnce({ data: refsResponse, error: null });

    renderDialog({
      orgId: 'org-real',
      sessionId: 'sess-1',
      sourceRef: 'refs/heads/jam/sess-1/user/main',
    });

    await clickFork();

    await waitFor(() => {
      expect(vi.mocked(fetch)).toHaveBeenCalled();
    });

    const fetchCalls = vi.mocked(fetch).mock.calls;
    expect(fetchCalls.length).toBeGreaterThan(0);
    const [url, options] = fetchCalls[0] as [string, RequestInit];
    expect(url).toBe('/mcp');

    // The Authorization header must be present and carry the bearer token.
    const headers = options.headers as Record<string, string>;
    expect(headers['Authorization']).toBe(`Bearer ${TEST_TOKEN}`);
  });
});
