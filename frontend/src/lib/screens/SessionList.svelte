<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import Chrome from '$lib/components/Chrome.svelte';
  import ModePill from '$lib/components/ModePill.svelte';
  import AuthorDot from '$lib/components/AuthorDot.svelte';
  import NewSessionDrawer from '$lib/components/NewSessionDrawer.svelte';
  import AttachHelpLink from '$lib/components/AttachHelpLink.svelte';
  import { client } from '$lib/api/client';
  import { subscribe } from '$lib/ws.svelte';
  import { auth } from '$lib/auth.svelte';
  import { navigate } from '$lib/router.svelte';
  import type { components } from '$lib/api/types.gen';

  type Session = components['schemas']['Session'];
  type FilterType = 'all' | 'active' | 'finalizing' | 'ended';

  // Props — orgId comes from route params; caller passes it in.
  let { orgId }: { orgId: string } = $props();

  // ── Load state machine ────────────────────────────────────────────────────
  //
  //  loading → ready  (GET 200, session list stored)
  //  loading → error  (GET non-200 or network failure)
  type LoadState = 'loading' | 'ready' | 'error';

  // State
  let sessions = $state<Session[]>([]);
  let loadState = $state<LoadState>('loading');
  let loadError = $state('');
  let activeFilter = $state<FilterType>('all');
  let drawerOpen = $state(false);

  // Filtered sessions
  const filteredSessions = $derived(
    activeFilter === 'all'
      ? sessions
      : sessions.filter((s) => s.status === activeFilter),
  );

  // Counts per status
  const counts = $derived({
    all: sessions.length,
    active: sessions.filter((s) => s.status === 'active').length,
    finalizing: sessions.filter((s) => s.status === 'finalizing').length,
    ended: sessions.filter((s) => s.status === 'ended').length,
  });

  async function loadSessions() {
    loadState = 'loading';
    const { data, error } = await client.GET('/api/orgs/{orgID}/sessions', {
      params: { path: { orgID: orgId } },
    });
    if (error) {
      loadError = 'Failed to load sessions.';
      loadState = 'error';
    } else if (data) {
      sessions = data.items;
      loadState = 'ready';
    }
  }

  function updateSession(updated: Partial<Session> & { id: string }) {
    sessions = sessions.map((s) => (s.id === updated.id ? { ...s, ...updated } : s));
  }

  // ── Unit 2: Per-session sequence guard for concurrent refetches ───────────
  //
  // Each refetch bumps the session's counter before the GET. On resolve, the
  // response is dropped if a newer refetch has started (seq mismatch). Counters
  // are monotonic and never reset — a returning id cannot accept an older response.
  //
  // Status-event handlers (session.finalizing, session.ended) call bumpAndUpdate
  // so any in-flight commit.arrived refetch is invalidated and cannot overwrite
  // the terminal status.
  const refetchSeq = new Map<string, number>();

  function bumpAndUpdate(id: string, patch: Partial<Session> & { id: string }) {
    refetchSeq.set(id, (refetchSeq.get(id) ?? 0) + 1);
    updateSession(patch);
  }

  function refetchSession(id: string) {
    const seq = (refetchSeq.get(id) ?? 0) + 1;
    refetchSeq.set(id, seq);
    void client
      .GET('/api/orgs/{orgID}/sessions/{sessionID}', {
        params: { path: { orgID: orgId, sessionID: id } },
      })
      .then(({ data, error: _err }) => {
        if (refetchSeq.get(id) !== seq) return; // a newer refetch superseded this one
        if (data) updateSession(data);
        // else: leave prior state; a failed GET is best-effort (error is ignored)
      });
  }

  // ── Unit 1: Stable subscription effect keyed on the session-id SET ────────
  //
  // The $effect reads only `sessionIdsKey` (a derived string of sorted ids) as
  // its reactive dependency. Reading the actual session list inside the effect is
  // done via `untrack(...)` so a field-only `updateSession` call (which reassigns
  // `sessions` but does not change the id set) does NOT re-run this effect.
  //
  // On a genuine id-set change the effect re-runs: it unsubscribes the old set
  // and immediately re-subscribes the current set within the same synchronous
  // run. The ws.svelte macrotask linger absorbs this cleanup→resubscribe window
  // so surviving sessions' sockets stay open.
  const TYPES = ['commit.arrived', 'session.finalizing', 'session.ended', 'presence.updated'] as const;
  type EventType = (typeof TYPES)[number];

  const sessionIdsKey = $derived(sessions.map((s) => s.id).sort().join(','));

  function makeHandler(id: string, type: EventType) {
    return (_env: { type: string; [key: string]: unknown }) => {
      if (type === 'session.finalizing') {
        bumpAndUpdate(id, { id, status: 'finalizing' });
      } else if (type === 'session.ended') {
        bumpAndUpdate(id, { id, status: 'ended' });
      } else {
        // commit.arrived + presence.updated: sequence-guarded refetch for fresh data.
        refetchSession(id);
      }
    };
  }

  $effect(() => {
    sessionIdsKey; // the ONLY reactive dependency — a field update doesn't change it
    const ids = untrack(() => sessions.map((s) => s.id));
    const unsubs: (() => void)[] = [];
    for (const id of ids) {
      for (const type of TYPES) {
        unsubs.push(subscribe(id, type, makeHandler(id, type)));
      }
    }
    return () => {
      for (const u of unsubs) u();
    };
  });

  onMount(() => {
    void loadSessions();
  });

  function formatRecency(isoDate: string): string {
    const diff = Date.now() - new Date(isoDate).getTime();
    const mins = Math.floor(diff / 60_000);
    if (mins < 2) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.floor(hours / 24);
    if (days < 7) return `${days}d ago`;
    const weeks = Math.floor(days / 7);
    return `${weeks}w ago`;
  }

  function parseScopeGlobs(scope: string): string[] {
    try {
      const parsed = JSON.parse(scope);
      if (Array.isArray(parsed)) return parsed as string[];
    } catch {
      // fallback: treat as comma-separated
    }
    return scope
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
  }

  function navigateToSession(session: Session) {
    navigate(`/orgs/${orgId}/sessions/${session.id}`);
  }

  const orgName = $derived(
    auth.currentUser?.displayName ?? orgId,
  );
</script>

<Chrome orgChip={orgId}>
  {#snippet children()}
    <div class="session-list-page">
      <div class="page-actions">
        <AttachHelpLink sessionId={null} />
        <button class="new-btn" onclick={() => (drawerOpen = true)}>
          <span class="plus" aria-hidden="true">+</span> New session
        </button>
      </div>

      <div class="page-header">
        <h1>Your sessions</h1>
        <p>Sessions you've joined or created. Most recent activity first.</p>
      </div>

      <div class="filters" role="group" aria-label="Filter sessions">
        {#each (['all', 'active', 'finalizing', 'ended'] as FilterType[]) as filter}
          <button
            class="filter-chip"
            class:active={activeFilter === filter}
            onclick={() => (activeFilter = filter)}
            aria-pressed={activeFilter === filter}
          >
            {filter.charAt(0).toUpperCase() + filter.slice(1)}
            <span class="count">{counts[filter]}</span>
          </button>
        {/each}
      </div>

      {#if loadState === 'loading'}
        <p class="loading-msg">Loading sessions…</p>
      {:else if loadState === 'error'}
        <p class="error-msg">{loadError}</p>
      {:else if filteredSessions.length === 0}
        <p class="empty-msg">No {activeFilter === 'all' ? '' : activeFilter} sessions.</p>
      {:else}
        <ul class="session-list" aria-label="Sessions">
          {#each filteredSessions as session (session.id)}
            {@const globs = parseScopeGlobs(session.scope)}
            {@const isEnded = session.status === 'ended'}
            {@const isFinalizing = session.status === 'finalizing'}
            <li>
              <button
                class="session-row"
                class:ended={isEnded}
                onclick={() => navigateToSession(session)}
                aria-label={session.name}
              >
                <div class="session-main">
                  <h3>
                    {session.name}
                    {#if isFinalizing}
                      <span class="status-pill finalizing">finalizing</span>
                    {:else if isEnded}
                      <span class="status-pill ended">ended</span>
                    {/if}
                  </h3>
                  <p class="goal">{session.goal}</p>
                  <div class="meta-strip">
                    {#if !isEnded}
                      <ModePill mode={session.default_mode} />
                    {/if}
                    <span>{session.members.length} {session.members.length === 1 ? 'member' : 'members'}</span>
                    <span aria-hidden="true">·</span>
                    {#if globs.length > 0}
                      <span class="scope">
                        scope:
                        {#each globs.slice(0, 3) as glob}
                          <code>{glob}</code>
                        {/each}
                      </span>
                    {/if}
                  </div>
                </div>
                <div class="session-right">
                  <div class="author-strip" aria-label="Members">
                    {#each session.members.slice(0, 5) as member}
                      <AuthorDot authorId={member.account_id} size={24} />
                    {/each}
                  </div>
                  <span class="last-activity">
                    {formatRecency(session.created_at)}
                  </span>
                </div>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    {#if drawerOpen}
      <NewSessionDrawer
        {orgId}
        onclose={() => (drawerOpen = false)}
      />
    {/if}

  {/snippet}
</Chrome>

<style>
  .session-list-page {
    padding: 32px 32px 80px;
    max-width: 1180px;
    margin: 0 auto;
  }

  .page-actions {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 12px;
  }

  .new-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 7px 14px;
    background: var(--color-bg-inverse);
    color: var(--color-text-inverse);
    border: 0;
    border-radius: var(--radius-md);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    cursor: pointer;
  }

  .new-btn:hover {
    opacity: 0.9;
  }

  .plus {
    opacity: 0.7;
  }

  .page-header {
    margin-bottom: 24px;
  }

  .page-header h1 {
    margin: 0 0 4px;
    font-size: var(--font-size-2xl);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.02em;
  }

  .page-header p {
    margin: 0;
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
  }

  .filters {
    display: flex;
    gap: 6px;
    margin-bottom: 20px;
    padding-bottom: 16px;
    border-bottom: 1px solid var(--color-border);
  }

  .filter-chip {
    padding: 6px 12px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--color-text-secondary);
    font: var(--font-size-sm) var(--font-sans);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .filter-chip.active {
    background: var(--color-bg-inverse);
    color: var(--color-text-inverse);
    border-color: var(--color-bg-inverse);
  }

  .count {
    font: var(--font-weight-medium) 11px var(--font-mono);
    opacity: 0.7;
  }

  .loading-msg,
  .error-msg,
  .empty-msg {
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
  }

  .error-msg {
    color: var(--color-danger);
  }

  .session-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
    list-style: none;
    padding: 0;
    margin: 0;
  }

  .session-row {
    width: 100%;
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 20px;
    padding: 18px 22px;
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    cursor: pointer;
    transition: border-color 0.1s;
    text-align: left;
    text-decoration: none;
    color: inherit;
    font-family: inherit;
    font-size: inherit;
  }

  .session-row:hover {
    border-color: var(--color-border-strong);
  }

  .session-row.ended {
    opacity: 0.6;
    background: var(--color-bg-tertiary);
  }

  .session-main h3 {
    margin: 0 0 4px;
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.01em;
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }

  .status-pill {
    font: var(--font-weight-semibold) 10px var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: var(--radius-sm);
  }

  .status-pill.finalizing {
    background: var(--color-warning-muted);
    color: var(--color-warning);
  }

  .status-pill.ended {
    background: var(--color-bg-secondary);
    color: var(--color-text-tertiary);
    border: 1px solid var(--color-border);
  }

  .goal {
    margin: 0 0 12px;
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    max-width: 640px;
  }

  .meta-strip {
    display: flex;
    gap: 18px;
    align-items: center;
    flex-wrap: wrap;
    color: var(--color-text-tertiary);
    font-size: var(--font-size-sm);
  }

  .scope {
    font: 11px var(--font-mono);
    color: var(--color-text-secondary);
    display: flex;
    gap: 4px;
    align-items: center;
    flex-wrap: wrap;
  }

  .scope code {
    background: var(--color-bg-tertiary);
    padding: 1px 5px;
    border-radius: 3px;
    font-family: var(--font-mono);
  }

  .session-right {
    display: flex;
    flex-direction: column;
    gap: 10px;
    align-items: flex-end;
    justify-content: space-between;
  }

  .author-strip {
    display: flex;
    align-items: center;
  }

  .author-strip :global(.dot) {
    margin-right: -8px;
    box-shadow: 0 0 0 2px var(--color-bg-secondary);
  }

  .author-strip :global(.dot:last-child) {
    margin-right: 0;
  }

  .last-activity {
    font: 11px var(--font-mono);
    color: var(--color-text-tertiary);
  }
</style>
