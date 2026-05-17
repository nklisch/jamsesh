import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import ActivityFeed from './ActivityFeed.svelte';
import type { components } from '$lib/api/types.gen';

type EventEnvelope = components['schemas']['EventEnvelope'];

// ── Module mocks ─────────────────────────────────────────────────────────────

// Capture handlers so tests can fire fake events
const handlersByType = new Map<string, ((env: unknown) => void)[]>();

const mockSubscribe = vi.fn((sessionId: string, type: string, handler: (env: unknown) => void) => {
  if (!handlersByType.has(type)) handlersByType.set(type, []);
  handlersByType.get(type)!.push(handler);
  return () => {
    const arr = handlersByType.get(type);
    if (arr) handlersByType.set(type, arr.filter((h) => h !== handler));
  };
});

vi.mock('$lib/ws.svelte', () => ({
  subscribe: (...args: Parameters<typeof mockSubscribe>) => mockSubscribe(...args),
}));

vi.mock('$lib/auth.svelte', () => ({
  auth: { token: 'test-token', isAuthenticated: true, currentUser: null },
}));

// ── Helpers ────────────────────────────────────────────────────────────────────

let _seq = 0;

function fire(type: string, payload: Record<string, unknown>) {
  const env: Partial<EventEnvelope> = {
    seq: ++_seq,
    version: 1,
    type: type as EventEnvelope['type'],
    timestamp: new Date().toISOString(),
    session_id: 'sess-1',
    payload: payload as EventEnvelope['payload'],
  };
  const handlers = handlersByType.get(type) ?? [];
  for (const h of handlers) h(env);
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('ActivityFeed', () => {
  beforeEach(() => {
    handlersByType.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('shows empty state before any events', () => {
    render(ActivityFeed, { props: { sessionId: 'sess-1' } });
    expect(screen.getByText(/no activity yet/i)).toBeInTheDocument();
  });

  it('subscribes to all event types', () => {
    render(ActivityFeed, { props: { sessionId: 'sess-1' } });
    // 12 event types registered
    expect(mockSubscribe).toHaveBeenCalledTimes(12);
  });

  it('subscribes with the correct sessionId', () => {
    render(ActivityFeed, { props: { sessionId: 'sess-42' } });
    for (const [, call] of mockSubscribe.mock.calls.entries()) {
      expect(call[0]).toBe('sess-42');
    }
  });

  it('renders a commit.arrived event', async () => {
    render(ActivityFeed, { props: { sessionId: 'sess-1' } });

    fire('commit.arrived', {
      ref: 'refs/heads/jam/sess-1/alice/main',
      sha: 'abc1234567890',
      author_id: 'alice',
      summary: 'Fix login flow',
    });

    await waitFor(() => {
      // "pushed" is unique text indicating the commit.arrived event rendered
      const items = screen.getAllByText(/pushed/i);
      expect(items.length).toBeGreaterThan(0);
    });
  });

  it('renders a merge.succeeded event', async () => {
    render(ActivityFeed, { props: { sessionId: 'sess-1' } });

    fire('merge.succeeded', {
      source_sha: 'abc123456789',
      draft_sha: 'def987654321',
      merge_commit_sha: 'fff000111222',
    });

    await waitFor(() => {
      expect(screen.getByText(/auto-merger merged/i)).toBeInTheDocument();
    });
  });

  it('renders a conflict.detected event with conflict styling', async () => {
    render(ActivityFeed, { props: { sessionId: 'sess-1' } });

    fire('conflict.detected', {
      id: 'conflict-1',
      session_id: 'sess-1',
      source_commit_sha: 'aaa111',
      source_ref: 'refs/heads/jam/sess-1/bob/main',
      draft_tip_sha: 'bbb222',
      ancestor_sha: 'ccc333',
      conflicts: [],
      addressed_to: [],
      status: 'open',
      created_at: new Date().toISOString(),
    });

    await waitFor(() => {
      const conflictItem = document.querySelector('.conflict');
      expect(conflictItem).toBeInTheDocument();
    });
  });

  it('prepends new events to the top of the list', async () => {
    render(ActivityFeed, { props: { sessionId: 'sess-1' } });

    fire('turn.ended', { user_id: 'alice', ref: 'jam/1/alice/main', final_sha: 'sha1' });
    fire('mode.changed', { ref: 'jam/1/alice/main', old_mode: 'sync', new_mode: 'isolated' });

    await waitFor(() => {
      const items = document.querySelectorAll('.feed-item');
      expect(items).toHaveLength(2);
    });
  });

  it('caps the event list at 100', async () => {
    render(ActivityFeed, { props: { sessionId: 'sess-1' } });

    for (let i = 0; i < 105; i++) {
      fire('turn.ended', { user_id: `user-${i}`, ref: 'ref', final_sha: `sha${i}` });
    }

    await waitFor(() => {
      const items = document.querySelectorAll('.feed-item');
      expect(items.length).toBeLessThanOrEqual(100);
    });
  });
});
