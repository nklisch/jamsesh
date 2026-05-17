<script lang="ts">
  import { subscribe } from '$lib/ws.svelte';
  import type { components } from '$lib/api/types.gen';

  type EventEnvelope = components['schemas']['EventEnvelope'];

  let {
    sessionId,
  }: {
    sessionId: string;
  } = $props();

  const MAX_EVENTS = 100;

  let events = $state<EventEnvelope[]>([]);

  function formatEvent(env: EventEnvelope): { icon: string; html: string; isConflict: boolean } {
    const p = env.payload as Record<string, unknown>;
    switch (env.type) {
      case 'commit.arrived':
        return {
          icon: '●',
          html: `<span class="who">${p.author_id}</span> pushed <span class="sha">${String(p.sha).slice(0, 7)}</span> to ${p.ref}: ${p.summary}`,
          isConflict: false,
        };
      case 'merge.succeeded':
        return {
          icon: '⇈',
          html: `auto-merger merged <em>${String(p.source_sha).slice(0, 7)}</em> into draft`,
          isConflict: false,
        };
      case 'conflict.detected':
        return {
          icon: '⚡',
          html: `<span class="who">${p.source_ref}</span> conflict vs draft`,
          isConflict: true,
        };
      case 'conflict.resolved':
        return {
          icon: '✓',
          html: `conflict resolved via <span class="sha">${String(p.resolving_commit_sha).slice(0, 7)}</span>`,
          isConflict: false,
        };
      case 'comment.added':
        return {
          icon: '💬',
          html: `<span class="who">${p.author_id}</span> commented: ${p.body}`,
          isConflict: false,
        };
      case 'comment.resolved':
        return {
          icon: '✓',
          html: `<span class="who">${p.resolved_by}</span> resolved a comment`,
          isConflict: false,
        };
      case 'ref.forked':
        return {
          icon: '⤴',
          html: `ref forked: <span class="sha">${p.ref}</span> from <span class="sha">${String(p.parent_sha).slice(0, 7)}</span>`,
          isConflict: false,
        };
      case 'mode.changed':
        return {
          icon: '⟳',
          html: `mode changed on <span class="sha">${p.ref}</span>: ${p.old_mode} → ${p.new_mode}`,
          isConflict: false,
        };
      case 'turn.ended':
        return {
          icon: '◉',
          html: `<span class="who">${p.user_id}</span> turn ended on ${p.ref}`,
          isConflict: false,
        };
      case 'presence.updated':
        return {
          icon: '◌',
          html: `<span class="who">${p.user_id}</span> active on ${p.ref}`,
          isConflict: false,
        };
      case 'session.finalizing':
        return {
          icon: '⏳',
          html: `session is finalizing`,
          isConflict: false,
        };
      case 'session.ended':
        return {
          icon: '■',
          html: `session ended`,
          isConflict: false,
        };
      default:
        return {
          icon: '·',
          html: env.type,
          isConflict: false,
        };
    }
  }

  function addEvent(env: EventEnvelope) {
    events = [env, ...events].slice(0, MAX_EVENTS);
  }

  $effect(() => {
    const ALL_TYPES = [
      'commit.arrived',
      'merge.succeeded',
      'conflict.detected',
      'conflict.resolved',
      'comment.added',
      'comment.resolved',
      'ref.forked',
      'mode.changed',
      'turn.ended',
      'presence.updated',
      'session.finalizing',
      'session.ended',
    ] as const;

    const unsubs = ALL_TYPES.map((type) =>
      subscribe(sessionId, type, (env) => addEvent(env as EventEnvelope)),
    );

    return () => {
      for (const u of unsubs) u();
    };
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
</script>

<div class="activity-feed" aria-label="Activity feed">
  {#if events.length === 0}
    <p class="empty">No activity yet — live events will appear here.</p>
  {:else}
    <ul class="feed-list">
      {#each events as env (env.seq)}
        {@const fmt = formatEvent(env)}
        <li class="feed-item" class:conflict={fmt.isConflict}>
          <span class="icon" aria-hidden="true">{fmt.icon}</span>
          <!-- eslint-disable-next-line svelte/no-at-html-tags -->
          <span class="text">{@html fmt.html}</span>
          <span class="when">{timeAgo(env.timestamp)}</span>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .activity-feed {
    height: 100%;
  }

  .empty {
    margin: 0;
    font-size: var(--font-size-sm);
    color: var(--color-text-tertiary);
    padding: 12px 0;
  }

  .feed-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .feed-item {
    padding: 8px 12px;
    border-radius: var(--radius-sm);
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    line-height: 1.45;
    display: flex;
    gap: 10px;
    align-items: center;
  }

  .feed-item:hover {
    background: var(--color-bg-tertiary);
  }

  .feed-item.conflict {
    background: var(--color-danger-muted);
    color: var(--color-danger);
  }

  .feed-item :global(.who) {
    color: var(--color-text-primary);
    font-weight: var(--font-weight-medium);
  }

  .feed-item.conflict :global(.who) {
    color: var(--color-danger);
  }

  .feed-item :global(.sha) {
    font-family: var(--font-mono);
    font-size: 11px;
  }

  .icon {
    display: inline-block;
    width: 14px;
    color: var(--color-text-tertiary);
    flex-shrink: 0;
    text-align: center;
  }

  .feed-item.conflict .icon {
    color: var(--color-danger);
  }

  .text {
    flex: 1;
  }

  .when {
    font: 10px var(--font-mono);
    color: var(--color-text-tertiary);
    flex-shrink: 0;
    margin-left: auto;
  }
</style>
