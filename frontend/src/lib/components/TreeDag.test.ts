import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import TreeDag from './TreeDag.svelte';
import type { components } from '$lib/api/types.gen';

type Ref = components['schemas']['Ref'];

// ── Module mocks ─────────────────────────────────────────────────────────────

const mockGET = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: { GET: (...args: unknown[]) => mockGET(...args) },
}));

vi.mock('$lib/auth.svelte', () => ({
  auth: { token: 'test-token', isAuthenticated: true, currentUser: null },
}));

const handlersByType = new Map<string, ((env: unknown) => void)[]>();
const mockSubscribe = vi.fn((_sessionId: string, type: string, handler: (env: unknown) => void) => {
  if (!handlersByType.has(type)) handlersByType.set(type, []);
  handlersByType.get(type)!.push(handler);
  return () => {};
});

vi.mock('$lib/ws.svelte', () => ({
  subscribe: (...args: Parameters<typeof mockSubscribe>) => mockSubscribe(...args),
}));

// ── Fixtures ──────────────────────────────────────────────────────────────────

function makeRef(overrides: Partial<Ref> = {}): Ref {
  return {
    ref: 'refs/heads/jam/sess-1/alice/main',
    sha: 'abc1234567890123456789012345678901234abcd',
    mode: 'sync',
    ...overrides,
  };
}

function renderTree(
  props: {
    treeState?: 'tree-collapsed' | 'tree-expanded' | 'tree-wide';
    selectedSha?: string | null;
    onselect?: (sha: string) => void;
  } = {},
) {
  return render(TreeDag, {
    props: {
      orgId: 'org-1',
      sessionId: 'sess-1',
      treeState: props.treeState ?? 'tree-expanded',
      selectedSha: props.selectedSha ?? null,
      onselect: props.onselect ?? vi.fn(),
    },
  });
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('TreeDag', () => {
  beforeEach(() => {
    handlersByType.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('fetches refs on mount', async () => {
    mockGET.mockResolvedValue({ data: { refs: [] }, error: null });
    renderTree();
    await waitFor(() => {
      expect(mockGET).toHaveBeenCalledWith('/api/orgs/{orgID}/sessions/{sessionID}/refs', {
        params: { path: { orgID: 'org-1', sessionID: 'sess-1' } },
      });
    });
  });

  it('renders ref groups in expanded mode', async () => {
    const refs = [
      makeRef({ ref: 'refs/heads/jam/sess-1/alice/main', mode: 'sync' }),
    ];
    mockGET.mockResolvedValue({ data: { refs }, error: null });
    renderTree({ treeState: 'tree-expanded' });

    await waitFor(() => {
      const full = document.querySelector('.tree-full');
      expect(full).toBeInTheDocument();
    });
  });

  it('renders rail in collapsed mode', async () => {
    mockGET.mockResolvedValue({ data: { refs: [] }, error: null });
    renderTree({ treeState: 'tree-collapsed' });

    await waitFor(() => {
      const rail = document.querySelector('.tree-rail');
      expect(rail).toBeInTheDocument();
    });
  });

  it('emits select event when a commit is clicked (expanded)', async () => {
    const onselect = vi.fn();
    const sha = 'abc1234567890123456789012345678901234abcd';
    const refs = [makeRef({ sha })];
    mockGET.mockResolvedValue({ data: { refs }, error: null });

    renderTree({ treeState: 'tree-expanded', onselect });

    await waitFor(() => {
      const btn = document.querySelector('.commit-item');
      expect(btn).toBeInTheDocument();
    });

    const btn = document.querySelector('.commit-item') as HTMLButtonElement;
    await fireEvent.click(btn);

    expect(onselect).toHaveBeenCalledWith(sha);
  });

  it('emits select event when a rail commit is clicked (collapsed)', async () => {
    const onselect = vi.fn();
    const sha = 'abc1234567890123456789012345678901234abcd';
    const refs = [makeRef({ sha })];
    mockGET.mockResolvedValue({ data: { refs }, error: null });

    renderTree({ treeState: 'tree-collapsed', onselect });

    await waitFor(() => {
      const btn = document.querySelector('.rail-commit');
      expect(btn).toBeInTheDocument();
    });

    const btn = document.querySelector('.rail-commit') as HTMLButtonElement;
    await fireEvent.click(btn);

    expect(onselect).toHaveBeenCalledWith(sha);
  });

  it('marks selected commit visually', async () => {
    const sha = 'abc1234567890123456789012345678901234abcd';
    const refs = [makeRef({ sha })];
    mockGET.mockResolvedValue({ data: { refs }, error: null });

    renderTree({ treeState: 'tree-expanded', selectedSha: sha });

    await waitFor(() => {
      const selected = document.querySelector('.commit-item.selected');
      expect(selected).toBeInTheDocument();
    });
  });

  it('subscribes to relevant WS event types', () => {
    mockGET.mockResolvedValue({ data: { refs: [] }, error: null });
    renderTree();

    const subscribedTypes = mockSubscribe.mock.calls.map((c) => c[1]);
    expect(subscribedTypes).toContain('commit.arrived');
    expect(subscribedTypes).toContain('merge.succeeded');
    expect(subscribedTypes).toContain('ref.forked');
    expect(subscribedTypes).toContain('mode.changed');
  });

  it('re-fetches refs when a WS event fires', async () => {
    mockGET.mockResolvedValue({ data: { refs: [] }, error: null });
    renderTree();

    // Wait for initial fetch
    await waitFor(() => expect(mockGET).toHaveBeenCalledTimes(1));

    // Fire a WS event
    const handlers = handlersByType.get('commit.arrived') ?? [];
    for (const h of handlers) h({ type: 'commit.arrived', seq: 1 });

    await waitFor(() => expect(mockGET).toHaveBeenCalledTimes(2));
  });

  it('renders sync mode badge', async () => {
    const refs = [makeRef({ mode: 'sync' })];
    mockGET.mockResolvedValue({ data: { refs }, error: null });

    renderTree({ treeState: 'tree-expanded' });

    await waitFor(() => {
      const badge = document.querySelector('.mode-mini.sync');
      expect(badge).toBeInTheDocument();
    });
  });

  it('renders isolated mode badge', async () => {
    const refs = [makeRef({ mode: 'isolated' })];
    mockGET.mockResolvedValue({ data: { refs }, error: null });

    renderTree({ treeState: 'tree-expanded' });

    await waitFor(() => {
      const badge = document.querySelector('.mode-mini.isolated');
      expect(badge).toBeInTheDocument();
    });
  });
});
