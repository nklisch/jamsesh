<script lang="ts">
  import { onMount } from 'svelte';
  import { client } from '$lib/api/client';
  import { subscribe } from '$lib/ws.svelte';
  import AuthorDot from './AuthorDot.svelte';
  import type { components } from '$lib/api/types.gen';

  type Ref = components['schemas']['Ref'];

  let {
    orgId,
    sessionId,
    treeState,
    selectedSha,
    onselect,
    onrefaction,
  }: {
    orgId: string;
    sessionId: string;
    treeState: 'tree-collapsed' | 'tree-expanded' | 'tree-wide';
    selectedSha: string | null;
    onselect: (sha: string) => void;
    onrefaction?: (event: { ref: string; action: 'menu'; x: number; y: number }) => void;
  } = $props();

  function handleRefContextMenu(ref: string, e: MouseEvent) {
    e.preventDefault();
    onrefaction?.({ ref, action: 'menu', x: e.clientX, y: e.clientY });
  }

  let refs = $state<Ref[]>([]);

  // Parse ref name: "refs/heads/jam/<sessionID>/<userID>/<branch>"
  // Returns a display label like "alice/main" or "draft".
  function shortRefName(ref: string): string {
    // refs/heads/jam/<sessionID>/<userID>/<branch>
    const parts = ref.replace('refs/heads/', '').split('/');
    // parts: ['jam', sessionID, userID, ...branchParts]
    if (parts.length >= 4) {
      const userId = parts[2];
      const branch = parts.slice(3).join('/');
      // If this is the draft ref (userId === 'draft' or similar)
      if (userId === 'draft' || branch === 'draft') return 'draft';
      return `${userId.slice(0, 6)}/${branch}`;
    }
    return parts[parts.length - 1] ?? ref;
  }

  function isDraftRef(ref: Ref): boolean {
    return ref.ref.includes('/draft') || shortRefName(ref.ref) === 'draft';
  }

  // v1: treat all refs as their own author (user id from ref path)
  function authorIdFromRef(ref: Ref): string {
    const parts = ref.ref.replace('refs/heads/', '').split('/');
    if (parts.length >= 4) return parts[2];
    return ref.ref;
  }

  async function fetchRefs() {
    const { data } = await client.GET('/api/orgs/{orgID}/sessions/{sessionID}/refs', {
      params: { path: { orgID: orgId, sessionID: sessionId } },
    });
    if (data) refs = data.refs;
  }

  $effect(() => {
    const refetchTypes = [
      'commit.arrived',
      'merge.succeeded',
      'ref.forked',
      'mode.changed',
    ] as const;

    const unsubs = refetchTypes.map((type) =>
      subscribe(sessionId, type, () => {
        void fetchRefs();
      }),
    );

    return () => {
      for (const u of unsubs) u();
    };
  });

  onMount(() => {
    void fetchRefs();
  });
</script>

<div class="tree-dag" class:collapsed={treeState === 'tree-collapsed'}>
  {#if treeState === 'tree-collapsed'}
    <!-- Rail view: dots only -->
    <div class="tree-rail" aria-label="Tree rail">
      <div class="rail-refs">
        {#each refs as ref}
          {@const authorId = authorIdFromRef(ref)}
          {@const shortName = shortRefName(ref.ref)}
          {@const isModeSync = ref.mode === 'sync'}
          <div
            class="rail-ref"
            title="{shortName} · {ref.mode} · {ref.sha.slice(0, 7)}"
            role="group"
            aria-label={shortName}
            oncontextmenu={(e) => handleRefContextMenu(ref.ref, e)}
          >
            <span
              class="rail-mode"
              class:sync={isModeSync}
              class:isolated={!isModeSync}
              class:draft={isDraftRef(ref)}
            >
              {shortName.slice(0, 6)}
            </span>
            <div class="rail-commits">
              <button
                class="rail-commit"
                class:selected={selectedSha === ref.sha}
                style="background: var(--author-{((ref.sha.charCodeAt(0) % 8) + 1)})"
                title={ref.sha.slice(0, 7)}
                onclick={() => onselect(ref.sha)}
                aria-label="Select commit {ref.sha.slice(0, 7)}"
                aria-pressed={selectedSha === ref.sha}
              ></button>
            </div>
          </div>
        {/each}
      </div>
    </div>
  {:else}
    <!-- Full expanded/wide view: ref groups with commit list -->
    <div class="tree-full" aria-label="Tree">
      {#each refs as ref}
        {@const authorId = authorIdFromRef(ref)}
        {@const shortName = shortRefName(ref.ref)}
        {@const isModeSync = ref.mode === 'sync'}
        {@const isDraft = isDraftRef(ref)}
        <div
          class="ref-group"
          role="group"
          aria-label={shortName}
          oncontextmenu={(e) => handleRefContextMenu(ref.ref, e)}
        >
          <div class="ref-header">
            <AuthorDot {authorId} size={12} />
            <span class="name" class:draft-name={isDraft}>{shortName}</span>
            <span
              class="mode-mini"
              class:sync={isModeSync && !isDraft}
              class:isolated={!isModeSync && !isDraft}
              class:draft={isDraft}
            >
              {isDraft ? 'auto-merged' : ref.mode}
            </span>
            <span class="where">@ {ref.sha.slice(0, 7)}</span>
          </div>
          <ul class="commit-list">
            <li>
              <button
                class="commit-item"
                class:selected={selectedSha === ref.sha}
                style="--commit-color: var(--author-{((ref.sha.charCodeAt(0) % 8) + 1)})"
                onclick={() => onselect(ref.sha)}
                aria-pressed={selectedSha === ref.sha}
              >
                <span class="msg">{ref.sha.slice(0, 7)}</span>
                <span class="sha">{ref.sha.slice(0, 7)}</span>
              </button>
            </li>
          </ul>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .tree-dag {
    height: 100%;
    overflow-y: auto;
  }

  /* ── Rail mode ───────────────────────────────────────────────────── */
  .rail-refs {
    display: flex;
    flex-direction: column;
    padding: 6px 0;
  }

  .rail-ref {
    padding: 8px 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
  }

  .rail-mode {
    font: 600 8px var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 1px 4px;
    border-radius: 2px;
    writing-mode: vertical-lr;
    transform: rotate(180deg);
  }

  .rail-mode.sync {
    background: var(--color-accent-muted);
    color: var(--color-accent);
  }

  .rail-mode.isolated {
    background: var(--color-warning-muted);
    color: var(--color-warning);
  }

  .rail-mode.draft {
    background: var(--color-bg-tertiary);
    color: var(--color-text-secondary);
  }

  .rail-commits {
    display: flex;
    flex-direction: column;
    gap: 6px;
    align-items: center;
  }

  .rail-commit {
    width: 14px;
    height: 14px;
    border-radius: 50%;
    cursor: pointer;
    position: relative;
    border: 0;
    box-shadow: 0 0 0 2px var(--color-bg-secondary);
    padding: 0;
  }

  .rail-commit:hover {
    transform: scale(1.2);
    transition: transform 0.1s;
  }

  .rail-commit.selected {
    box-shadow: 0 0 0 2px var(--color-accent), 0 0 0 4px var(--color-bg-secondary);
  }

  /* ── Full tree mode ──────────────────────────────────────────────── */
  .ref-group {
    padding: 8px 0;
    border-bottom: 1px solid var(--color-border);
  }

  .ref-group:last-child {
    border-bottom: 0;
  }

  .ref-header {
    padding: 6px 14px;
    display: flex;
    gap: 8px;
    align-items: center;
    font: var(--font-size-xs) var(--font-mono);
    color: var(--color-text-secondary);
  }

  .name {
    color: var(--color-text-primary);
    font-weight: var(--font-weight-medium);
    flex: 1;
  }

  .draft-name {
    color: var(--color-text-secondary);
  }

  .mode-mini {
    padding: 1px 6px;
    border-radius: 3px;
    font: var(--font-weight-semibold) 9px var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .mode-mini.sync {
    background: var(--color-accent-muted);
    color: var(--color-accent);
  }

  .mode-mini.isolated {
    background: var(--color-warning-muted);
    color: var(--color-warning);
  }

  .mode-mini.draft {
    background: var(--color-bg-tertiary);
    color: var(--color-text-secondary);
  }

  .where {
    font: 10px var(--font-mono);
    color: var(--color-text-tertiary);
  }

  .commit-list {
    list-style: none;
    padding: 0;
    margin: 0;
  }

  .commit-item {
    width: 100%;
    padding: 5px 14px 5px 30px;
    position: relative;
    cursor: pointer;
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 8px;
    align-items: center;
    background: transparent;
    border: 0;
    color: inherit;
    font-family: inherit;
    font-size: inherit;
    text-align: left;
  }

  .commit-item:hover {
    background: var(--color-bg-tertiary);
  }

  .commit-item.selected {
    background: var(--color-accent-muted);
    border-left: 2px solid var(--color-accent);
    padding-left: 28px;
  }

  .commit-item::before {
    content: '';
    position: absolute;
    left: 18px;
    top: 50%;
    transform: translateY(-50%);
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--commit-color);
  }

  .msg {
    font-size: var(--font-size-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .sha {
    font: 10px var(--font-mono);
    color: var(--color-text-tertiary);
  }
</style>
