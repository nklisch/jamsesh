<script lang="ts">
  import { onMount } from 'svelte';
  import { client } from '$lib/api/client';
  import { subscribe } from '$lib/ws.svelte';
  import type { components } from '$lib/api/types.gen';

  type Comment = components['schemas']['Comment'];
  type EventEnvelope = components['schemas']['EventEnvelope'];

  let {
    orgId,
    sessionId,
  }: {
    orgId: string;
    sessionId: string;
  } = $props();

  let comments = $state<Comment[]>([]);

  // ── Load state machine ────────────────────────────────────────────────────
  //
  //  loading → ready   (GET 200, items stored)
  //  loading → error   (GET non-200 or network failure)
  //  ready   → loading (re-fetch on WS event)
  //  error   → loading (re-fetch on WS event)
  type LoadState = 'loading' | 'ready' | 'error';
  let loadState = $state<LoadState>('loading');
  let loadError = $state('');

  async function fetchComments() {
    loadState = 'loading';
    const { data, error } = await client.GET(
      '/api/orgs/{orgID}/sessions/{sessionID}/comments',
      {
        params: { path: { orgID: orgId, sessionID: sessionId } },
      },
    );
    if (error) {
      loadError = 'Failed to load comments.';
      loadState = 'error';
    } else if (data) {
      comments = data.items;
      loadState = 'ready';
    }
  }

  $effect(() => {
    const unsubs = [
      subscribe(sessionId, 'comment.added', (env) => {
        const e = env as EventEnvelope;
        if (e.type === 'comment.added') {
          // Re-fetch to get the canonical Comment object from the server.
          void fetchComments();
        }
      }),
      subscribe(sessionId, 'comment.resolved', (env) => {
        const e = env as EventEnvelope;
        if (e.type === 'comment.resolved') {
          const p = e.payload as { comment_id: string; resolved_by: string; note?: string | null };
          // Update in place — mark resolved without a round-trip.
          comments = comments.map((c) =>
            c.id === p.comment_id
              ? { ...c, resolved_at: new Date().toISOString(), resolved_by: p.resolved_by }
              : c,
          );
        }
      }),
    ];
    return () => {
      for (const u of unsubs) u();
    };
  });

  onMount(() => {
    void fetchComments();
  });

  function timeAgo(iso: string): string {
    const diff = Date.now() - new Date(iso).getTime();
    const mins = Math.floor(diff / 60_000);
    if (mins < 2) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    return `${Math.floor(hours / 24)}d ago`;
  }

  function anchorLabel(comment: Comment): string {
    const { anchor } = comment;
    if (!anchor.file_path) return anchor.commit_sha.slice(0, 7);
    const lineInfo =
      anchor.line_range ? `:${anchor.line_range.start}` : '';
    return `${anchor.file_path}${lineInfo}`;
  }
</script>

<div class="comments-tab" aria-label="Comments">
  {#if loadState === 'loading'}
    <p class="status-msg">Loading comments…</p>
  {:else if loadState === 'error'}
    <p class="status-msg error" role="alert">{loadError}</p>
  {:else if comments.length === 0}
    <p class="status-msg">No comments yet.</p>
  {:else}
    <div class="comment-grid">
      {#each comments as comment (comment.id)}
        <div
          class="comment-card"
          class:resolved={!!comment.resolved_at}
          role="article"
          aria-label="Comment by {comment.author_id}"
        >
          <div class="meta-row">
            <strong>@{comment.author_id}</strong>
            <span class="kind kind-{comment.kind}">{comment.kind}</span>
            <span class="when">· {timeAgo(comment.created_at)}</span>
            {#if comment.resolved_at}
              <span class="resolved-badge">resolved</span>
            {/if}
          </div>
          {#if comment.addressed_to}
            <div class="addressed">
              <span class="addr-pill">→ {comment.addressed_to}</span>
            </div>
          {/if}
          <div class="body">{comment.body}</div>
          <div class="anchor">{anchorLabel(comment)}</div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .comments-tab {
    height: 100%;
  }

  .status-msg {
    margin: 0;
    font-size: var(--font-size-sm);
    color: var(--color-text-tertiary);
    padding: 12px 0;
  }

  .status-msg.error {
    color: var(--color-danger);
  }

  .comment-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: 10px;
  }

  .comment-card {
    padding: 12px 14px;
    border-radius: var(--radius-sm);
    border-left: 3px solid var(--color-accent);
    background: var(--color-accent-muted);
    cursor: pointer;
    transition: background 0.1s;
  }

  .comment-card:hover {
    background: color-mix(in srgb, var(--color-accent) 15%, transparent);
  }

  .comment-card.resolved {
    opacity: 0.55;
    background: var(--color-bg-tertiary);
    border-left-color: var(--color-text-tertiary);
  }

  .meta-row {
    display: flex;
    gap: 8px;
    align-items: center;
    margin-bottom: 6px;
    font-size: var(--font-size-xs);
    color: var(--color-text-secondary);
    flex-wrap: wrap;
  }

  .meta-row strong {
    color: var(--color-text-primary);
    font-weight: var(--font-weight-medium);
  }

  .when {
    color: var(--color-text-tertiary);
  }

  .kind {
    padding: 1px 6px;
    border-radius: 3px;
    font: var(--font-weight-semibold) 9px var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    background: var(--color-bg-tertiary);
    color: var(--color-text-secondary);
  }

  .kind-question {
    background: var(--color-accent-muted);
    color: var(--color-accent);
  }

  .kind-suggestion {
    background: var(--color-warning-muted);
    color: var(--color-warning);
  }

  .kind-action-request {
    background: var(--color-danger-muted);
    color: var(--color-danger);
  }

  .resolved-badge {
    padding: 1px 6px;
    border-radius: 3px;
    font: var(--font-weight-semibold) 9px var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    background: var(--color-bg-tertiary);
    color: var(--color-text-tertiary);
  }

  .addressed {
    margin-bottom: 4px;
  }

  .addr-pill {
    font: 10px var(--font-mono);
    color: var(--color-text-secondary);
    background: var(--color-bg-tertiary);
    padding: 1px 6px;
    border-radius: 3px;
  }

  .body {
    font-size: var(--font-size-sm);
    color: var(--color-text-primary);
    line-height: 1.5;
  }

  .anchor {
    font: 10px var(--font-mono);
    color: var(--color-text-tertiary);
    margin-top: 6px;
  }
</style>
