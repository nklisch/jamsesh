import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import ActivityFeed from './ActivityFeed.svelte';
import type { components } from '$lib/api/types.gen';

type EventEnvelope = components['schemas']['EventEnvelope'];

// ── Module mocks ─────────────────────────────────────────────────────────────

// Capture handlers so tests can fire fake events
const handlersByType = new Map<string, ((env: unknown) => void)[]>();

const mockSubscribe = vi.fn((_sessionId: string, type: string, handler: (env: unknown) => void) => {
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

  // ── XSS coverage (gate-tests-xss-activityfeed-component) ───────────────────

  it('does not execute <img onerror> payload injected via comment.added body', async () => {
    // Arrange: delete any stale marker from a prior run
    delete (window as unknown as Record<string, unknown>).__pwned;

    const { container } = render(ActivityFeed, { props: { sessionId: 'sess-1' } });

    // Act: fire a comment whose body is a classic XSS probe
    fire('comment.added', {
      author_id: 'attacker',
      body: '<img src=x onerror="window.__pwned=1">',
    });

    await waitFor(() => {
      // The feed item must have appeared — verify "commented:" text is present
      // so we know the event was actually rendered (not silently dropped).
      expect(container.querySelector('.feed-item')).not.toBeNull();
    });

    // Assert 1: no live <img> element in the DOM (would only exist if {@html} was used)
    expect(container.querySelector('img')).toBeNull();

    // Assert 2: the payload was not silently dropped — the literal text "<img" must
    // appear somewhere in the rendered output as an escaped text node (not an element).
    // innerHTML encodes "<" as "&lt;", so the safe escaped form is what we expect.
    expect(container.innerHTML).toMatch(/&lt;img/i);

    // Assert 3: the injected script did not run
    expect((window as unknown as Record<string, unknown>).__pwned).toBeUndefined();
  });

  it('does not inject a <script> element via comment.added body', async () => {
    delete (window as unknown as Record<string, unknown>).__pwned;

    const { container } = render(ActivityFeed, { props: { sessionId: 'sess-1' } });

    fire('comment.added', {
      author_id: 'attacker',
      body: '<script>window.__pwned=2;<\/script>',
    });

    await waitFor(() => {
      expect(container.querySelector('.feed-item')).not.toBeNull();
    });

    // No live <script> element must be present in the component tree
    expect(container.querySelector('script')).toBeNull();

    // The raw payload must not appear as an unescaped script tag
    expect(container.innerHTML).not.toMatch(/<script/i);

    // The text content was not silently dropped — escaped form must be present
    expect(container.innerHTML).toMatch(/&lt;script/i);

    // The injected script did not run
    expect((window as unknown as Record<string, unknown>).__pwned).toBeUndefined();
  });

  // ── Table-driven XSS: all event-types × all string fields ─────────────────
  //
  // For every branch of formatEvent, each string field that is interpolated
  // into the DOM is probed with a classic img-onerror payload.  The test
  // verifies three things:
  //   1. No live <img> element was injected (i.e. not parsed as HTML).
  //   2. The onerror handler never ran (window.__pwned_<field> stays undefined).
  //   3. The payload text was actually rendered — escaped — so the field was
  //      not silently dropped (innerHTML must contain "&lt;img").
  //
  // sha() fields: sha values are sliced to 7 chars before rendering, so the
  // probe string must be ≤7 chars for it to survive and be visible in the
  // output.  We use a dedicated short probe for those fields.

  type XssCase = {
    /** Label shown in test output */
    label: string;
    /** EventEnvelope.type value */
    eventType: string;
    /** Full payload to fire; the probed field already has the XSS value */
    payload: Record<string, unknown>;
    /** Field name used as part of the window marker */
    field: string;
    /**
     * If true, the payload is rendered through sha() which truncates to 7
     * chars, so we check for the short probe instead of "&lt;img".
     */
    isSha?: boolean;
  };

  // Short probe for sha fields (sha() slices to 7 chars, so we use something
  // that will still appear after truncation — a plain string that doesn't
  // look like HTML so the "no img element" assertion still holds while the
  // "field was rendered" assertion uses a different check).
  //
  // NOTE: sha fields cannot carry an img-onerror payload through the 7-char
  // slice anyway, so for sha fields we only verify:
  //   a) no img element in the DOM, and
  //   b) the field value appears (truncated) in the rendered output.
  //
  // We use a safe 7-char probe for sha fields and skip the &lt;img check.

  const SHA_PROBE = 'safe-sha'; // 8 chars, sliced to "safe-sha"[0..7] = "safe-sh"
  const SHA_VISIBLE = 'safe-sh'; // what sha() renders after slicing to 7

  const XSS_PROBE = '<img src=x onerror="window.__pwned_{FIELD}=1">';

  const xssCases: XssCase[] = [
    // commit.arrived — author_id (who fragment)
    {
      label: 'commit.arrived / author_id',
      eventType: 'commit.arrived',
      field: 'author_id',
      payload: {
        author_id: XSS_PROBE.replace('{FIELD}', 'commit_author_id'),
        sha: 'abc1234',
        ref: 'refs/heads/main',
        summary: 'safe summary',
      },
    },
    // commit.arrived — ref (txt fragment inside template literal)
    {
      label: 'commit.arrived / ref',
      eventType: 'commit.arrived',
      field: 'commit_ref',
      payload: {
        author_id: 'safe-author',
        sha: 'abc1234',
        ref: XSS_PROBE.replace('{FIELD}', 'commit_ref'),
        summary: 'safe summary',
      },
    },
    // commit.arrived — summary (txt fragment inside template literal)
    {
      label: 'commit.arrived / summary',
      eventType: 'commit.arrived',
      field: 'commit_summary',
      payload: {
        author_id: 'safe-author',
        sha: 'abc1234',
        ref: 'refs/heads/main',
        summary: XSS_PROBE.replace('{FIELD}', 'commit_summary'),
      },
    },
    // commit.arrived — sha (sha fragment; truncated to 7 chars)
    {
      label: 'commit.arrived / sha',
      eventType: 'commit.arrived',
      field: 'commit_sha',
      isSha: true,
      payload: {
        author_id: 'safe-author',
        sha: SHA_PROBE,
        ref: 'refs/heads/main',
        summary: 'safe summary',
      },
    },
    // merge.succeeded — source_sha (sha fragment)
    {
      label: 'merge.succeeded / source_sha',
      eventType: 'merge.succeeded',
      field: 'merge_source_sha',
      isSha: true,
      payload: {
        source_sha: SHA_PROBE,
        draft_sha: 'abc1234',
        merge_commit_sha: 'def5678',
      },
    },
    // conflict.detected — source_ref (who fragment)
    {
      label: 'conflict.detected / source_ref',
      eventType: 'conflict.detected',
      field: 'conflict_source_ref',
      payload: {
        source_ref: XSS_PROBE.replace('{FIELD}', 'conflict_source_ref'),
        id: 'c1',
        session_id: 'sess-1',
        source_commit_sha: 'aaa',
        draft_tip_sha: 'bbb',
        ancestor_sha: 'ccc',
        conflicts: [],
        addressed_to: [],
        status: 'open',
        created_at: new Date().toISOString(),
      },
    },
    // conflict.resolved — resolving_commit_sha (sha fragment)
    {
      label: 'conflict.resolved / resolving_commit_sha',
      eventType: 'conflict.resolved',
      field: 'conflict_resolving_sha',
      isSha: true,
      payload: {
        resolving_commit_sha: SHA_PROBE,
      },
    },
    // comment.added — author_id (who fragment)  [extends existing focused test]
    {
      label: 'comment.added / author_id',
      eventType: 'comment.added',
      field: 'comment_author_id',
      payload: {
        author_id: XSS_PROBE.replace('{FIELD}', 'comment_author_id'),
        body: 'safe body',
      },
    },
    // comment.added — body (txt fragment)  [extends existing focused test]
    {
      label: 'comment.added / body',
      eventType: 'comment.added',
      field: 'comment_body',
      payload: {
        author_id: 'safe-author',
        body: XSS_PROBE.replace('{FIELD}', 'comment_body'),
      },
    },
    // comment.resolved — resolved_by (who fragment)
    {
      label: 'comment.resolved / resolved_by',
      eventType: 'comment.resolved',
      field: 'comment_resolved_by',
      payload: {
        resolved_by: XSS_PROBE.replace('{FIELD}', 'comment_resolved_by'),
      },
    },
    // ref.forked — ref (sha fragment)
    {
      label: 'ref.forked / ref',
      eventType: 'ref.forked',
      field: 'fork_ref',
      isSha: true,
      payload: {
        ref: SHA_PROBE,
        parent_sha: 'abc1234',
      },
    },
    // ref.forked — parent_sha (sha fragment)
    {
      label: 'ref.forked / parent_sha',
      eventType: 'ref.forked',
      field: 'fork_parent_sha',
      isSha: true,
      payload: {
        ref: 'abc1234',
        parent_sha: SHA_PROBE,
      },
    },
    // mode.changed — ref (sha fragment)
    {
      label: 'mode.changed / ref',
      eventType: 'mode.changed',
      field: 'mode_ref',
      isSha: true,
      payload: {
        ref: SHA_PROBE,
        old_mode: 'sync',
        new_mode: 'isolated',
      },
    },
    // mode.changed — old_mode (txt fragment inside template literal)
    {
      label: 'mode.changed / old_mode',
      eventType: 'mode.changed',
      field: 'mode_old_mode',
      payload: {
        ref: 'abc1234',
        old_mode: XSS_PROBE.replace('{FIELD}', 'mode_old_mode'),
        new_mode: 'isolated',
      },
    },
    // mode.changed — new_mode (txt fragment inside template literal)
    {
      label: 'mode.changed / new_mode',
      eventType: 'mode.changed',
      field: 'mode_new_mode',
      payload: {
        ref: 'abc1234',
        old_mode: 'sync',
        new_mode: XSS_PROBE.replace('{FIELD}', 'mode_new_mode'),
      },
    },
    // turn.ended — user_id (who fragment)
    {
      label: 'turn.ended / user_id',
      eventType: 'turn.ended',
      field: 'turn_user_id',
      payload: {
        user_id: XSS_PROBE.replace('{FIELD}', 'turn_user_id'),
        ref: 'refs/heads/main',
        final_sha: 'abc1234',
      },
    },
    // turn.ended — ref (txt fragment inside template literal)
    {
      label: 'turn.ended / ref',
      eventType: 'turn.ended',
      field: 'turn_ref',
      payload: {
        user_id: 'safe-user',
        ref: XSS_PROBE.replace('{FIELD}', 'turn_ref'),
        final_sha: 'abc1234',
      },
    },
    // presence.updated — user_id (who fragment)
    {
      label: 'presence.updated / user_id',
      eventType: 'presence.updated',
      field: 'presence_user_id',
      payload: {
        user_id: XSS_PROBE.replace('{FIELD}', 'presence_user_id'),
        ref: 'refs/heads/main',
      },
    },
    // presence.updated — ref (txt fragment inside template literal)
    {
      label: 'presence.updated / ref',
      eventType: 'presence.updated',
      field: 'presence_ref',
      payload: {
        user_id: 'safe-user',
        ref: XSS_PROBE.replace('{FIELD}', 'presence_ref'),
      },
    },
  ];

  it.each(xssCases)(
    'escapes XSS payload — $label',
    async ({ eventType, field, payload, isSha }) => {
      const markerKey = `__pwned_${field}`;
      delete (window as unknown as Record<string, unknown>)[markerKey];

      const { container } = render(ActivityFeed, { props: { sessionId: 'sess-1' } });

      fire(eventType, payload);

      await waitFor(() => {
        expect(container.querySelector('.feed-item')).not.toBeNull();
      });

      // 1. No live <img> element must exist in the DOM
      expect(container.querySelector('img')).toBeNull();

      // 2. The onerror handler must not have run
      expect((window as unknown as Record<string, unknown>)[markerKey]).toBeUndefined();

      if (isSha) {
        // sha() truncates to 7 chars — confirm the truncated value appears
        expect(container.textContent).toContain(SHA_VISIBLE);
      } else {
        // 3. The payload was rendered as escaped text, not silently dropped
        expect(container.innerHTML).toMatch(/&lt;img/i);
      }
    },
  );
});
