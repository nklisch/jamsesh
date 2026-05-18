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

  // Fragment-based structured representation — no HTML strings, no XSS sinks.
  type Fragment =
    | { kind: 'text'; value: string }
    | { kind: 'emphasis'; value: string } // rendered as <span class="who">
    | { kind: 'sha'; value: string }       // rendered as <span class="sha">

  type FormattedEvent = {
    icon: string;
    fragments: Fragment[];
    isConflict: boolean;
  };

  function txt(value: string): Fragment { return { kind: 'text', value }; }
  function who(value: string): Fragment { return { kind: 'emphasis', value }; }
  function sha(value: string): Fragment { return { kind: 'sha', value: String(value).slice(0, 7) }; }

  function formatEvent(env: EventEnvelope): FormattedEvent {
    const p = env.payload as Record<string, unknown>;
    switch (env.type) {
      case 'commit.arrived':
        return {
          icon: '●',
          fragments: [
            who(String(p.author_id)),
            txt(' pushed '),
            sha(String(p.sha)),
            txt(` to ${String(p.ref)}: ${String(p.summary)}`),
          ],
          isConflict: false,
        };
      case 'merge.succeeded':
        return {
          icon: '⇈',
          fragments: [
            txt('auto-merger merged '),
            sha(String(p.source_sha)),
            txt(' into draft'),
          ],
          isConflict: false,
        };
      case 'conflict.detected':
        return {
          icon: '⚡',
          fragments: [
            who(String(p.source_ref)),
            txt(' conflict vs draft'),
          ],
          isConflict: true,
        };
      case 'conflict.resolved':
        return {
          icon: '✓',
          fragments: [
            txt('conflict resolved via '),
            sha(String(p.resolving_commit_sha)),
          ],
          isConflict: false,
        };
      case 'comment.added':
        return {
          icon: '💬',
          fragments: [
            who(String(p.author_id)),
            txt(' commented: '),
            txt(String(p.body)),
          ],
          isConflict: false,
        };
      case 'comment.resolved':
        return {
          icon: '✓',
          fragments: [
            who(String(p.resolved_by)),
            txt(' resolved a comment'),
          ],
          isConflict: false,
        };
      case 'ref.forked':
        return {
          icon: '⤴',
          fragments: [
            txt('ref forked: '),
            sha(String(p.ref)),
            txt(' from '),
            sha(String(p.parent_sha)),
          ],
          isConflict: false,
        };
      case 'mode.changed':
        return {
          icon: '⟳',
          fragments: [
            txt('mode changed on '),
            sha(String(p.ref)),
            txt(`: ${String(p.old_mode)} → ${String(p.new_mode)}`),
          ],
          isConflict: false,
        };
      case 'turn.ended':
        return {
          icon: '◉',
          fragments: [
            who(String(p.user_id)),
            txt(` turn ended on ${String(p.ref)}`),
          ],
          isConflict: false,
        };
      case 'presence.updated':
        return {
          icon: '◌',
          fragments: [
            who(String(p.user_id)),
            txt(` active on ${String(p.ref)}`),
          ],
          isConflict: false,
        };
      case 'session.finalizing':
        return {
          icon: '⏳',
          fragments: [txt('session is finalizing')],
          isConflict: false,
        };
      case 'session.ended':
        return {
          icon: '■',
          fragments: [txt('session ended')],
          isConflict: false,
        };
      default:
        return {
          icon: '·',
          fragments: [txt(env.type)],
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
          <span class="text">
            {#each fmt.fragments as frag}
              {#if frag.kind === 'text'}
                {frag.value}
              {:else if frag.kind === 'emphasis'}
                <span class="who">{frag.value}</span>
              {:else if frag.kind === 'sha'}
                <span class="sha">{frag.value}</span>
              {/if}
            {/each}
          </span>
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

  .feed-item .who {
    color: var(--color-text-primary);
    font-weight: var(--font-weight-medium);
  }

  .feed-item.conflict .who {
    color: var(--color-danger);
  }

  .feed-item .sha {
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
