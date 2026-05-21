<script lang="ts">
  import { onMount } from 'svelte';
  import { client } from '$lib/api/client';
  import ThemeToggle from '$lib/components/ThemeToggle.svelte';
  import AttachHelpLink from '$lib/components/AttachHelpLink.svelte';
  import AuthorDot from '$lib/components/AuthorDot.svelte';
  import TreeDag from '$lib/components/TreeDag.svelte';
  import ActivityFeed from '$lib/components/ActivityFeed.svelte';
  import CommentsTab from '$lib/components/CommentsTab.svelte';
  import ArtifactPane from '$lib/components/ArtifactPane.svelte';
  import CommentComposer from '$lib/components/CommentComposer.svelte';
  import RefActionsMenu from '$lib/components/RefActionsMenu.svelte';
  import ForkDialog from '$lib/components/ForkDialog.svelte';
  import ModeSwitchDialog from '$lib/components/ModeSwitchDialog.svelte';
  import WsStatusBanner from '$lib/components/WsStatusBanner.svelte';
  import { auth } from '$lib/auth.svelte';
  import { navigate } from '$lib/router.svelte';
  import type { components } from '$lib/api/types.gen';

  type Session = components['schemas']['Session'];
  type TreeState = 'tree-collapsed' | 'tree-expanded' | 'tree-wide';
  type BottomTab = 'activity' | 'comments';
  type ActiveDialog = null | 'fork' | 'mode-switch';

  let {
    orgId,
    sessionId,
  }: {
    orgId: string;
    sessionId: string;
  } = $props();

  // Session data
  let session = $state<Session | null>(null);
  let loadError = $state<string | null>(null);

  // Tree state — persisted per session in localStorage
  const TREE_STATES: TreeState[] = ['tree-collapsed', 'tree-expanded', 'tree-wide'];
  // Use a function to build the key so we capture sessionId in a closure (avoids
  // the "reference only captures initial value" svelte-check warning).
  const treeStateKey = () => `jamsesh.tree-state.${sessionId}`;

  function loadTreeState(): TreeState {
    const stored = typeof localStorage !== 'undefined' ? localStorage.getItem(treeStateKey()) : null;
    if (stored && TREE_STATES.includes(stored as TreeState)) return stored as TreeState;
    return 'tree-collapsed';
  }

  let treeState = $state<TreeState>(loadTreeState());

  $effect(() => {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(treeStateKey(), treeState);
    }
  });

  function cycleTree() {
    const idx = TREE_STATES.indexOf(treeState);
    treeState = TREE_STATES[(idx + 1) % TREE_STATES.length];
  }

  // Bottom panel
  let bottomExpanded = $state(false);
  let activeTab = $state<BottomTab>('activity');

  function switchTab(tab: BottomTab) {
    activeTab = tab;
    if (!bottomExpanded) bottomExpanded = true;
  }

  // Commit selection
  let selectedCommitSha = $state<string | null>(null);
  // File selection (set by future file-tree or external signal)
  let selectedFilePath = $state<string | null>(null);

  function handleSelectCommit(sha: string) {
    selectedCommitSha = sha;
    // Clear file selection when switching commits.
    selectedFilePath = null;
  }

  // Comment composer
  let composerOpen = $state(false);
  let composerRange = $state<{ start: number; end: number } | null>(null);

  function handleRangeSelect(range: { start: number; end: number } | null) {
    composerRange = range;
    if (range) composerOpen = true;
  }

  // Ref actions
  let activeMenuRef = $state<{ ref: string; x: number; y: number } | null>(null);
  let activeDialog = $state<ActiveDialog>(null);
  let activeDialogRef = $state<string>('');
  let activeDialogRefMode = $state<'sync' | 'isolated' | undefined>(undefined);

  function handleRefAction(event: { ref: string; action: 'menu'; x: number; y: number }) {
    activeMenuRef = { ref: event.ref, x: event.x, y: event.y };
  }

  function handleMenuAction(action: 'fork' | 'mode-switch', ref: string) {
    activeMenuRef = null;
    activeDialogRef = ref;
    activeDialog = action;
    // Fetch current mode for the mode-switch dialog (best-effort).
    activeDialogRefMode = undefined;
  }

  function closeDialog() {
    activeDialog = null;
    activeDialogRef = '';
    activeDialogRefMode = undefined;
  }

  async function loadSession() {
    const { data, error } = await client.GET('/api/orgs/{orgID}/sessions/{sessionID}', {
      params: { path: { orgID: orgId, sessionID: sessionId } },
    });
    if (error) {
      loadError = 'Failed to load session.';
    } else if (data) {
      session = data;
    }
  }

  function parseScopeGlobs(scope: string): string[] {
    try {
      const parsed = JSON.parse(scope);
      if (Array.isArray(parsed)) return parsed as string[];
    } catch {
      // fallback
    }
    return scope.split(',').map((s) => s.trim()).filter(Boolean);
  }

  onMount(() => {
    void loadSession();
  });
</script>

<div class="shell" data-tree-state={treeState}>
  <!-- App chrome -->
  <header class="app-chrome">
    <div class="wordmark" aria-label="jamsesh">jam<span class="dot">·</span>sesh</div>
    <nav class="breadcrumb" aria-label="Breadcrumb">
      <button class="breadcrumb-link" onclick={() => navigate(`/orgs/${orgId}/sessions`)}>
        {orgId}
      </button>
      <span class="sep" aria-hidden="true">/</span>
      <span class="here">{session?.name ?? sessionId}</span>
    </nav>
    <div class="chrome-spacer"></div>
    <AttachHelpLink sessionId={sessionId} />
    <ThemeToggle />
    {#if auth.currentUser}
      <AuthorDot authorId={auth.currentUser.id} size={26} title={auth.currentUser.displayName} />
    {/if}
  </header>

  {#if loadError}
    <div class="load-error" role="alert">{loadError}</div>
  {:else if session}
    <!-- Session header -->
    <div class="session-header">
      <div class="header-info">
        <h1>{session.name}</h1>
        <p class="goal">{session.goal}</p>
        <div class="meta-strip">
          {#each parseScopeGlobs(session.scope).slice(0, 3) as glob}
            <span>scope <code>{glob}</code></span>
            <span aria-hidden="true">·</span>
          {/each}
          <span>default mode <code>{session.default_mode}</code></span>
          <span aria-hidden="true">·</span>
          <span>{session.members.length} member{session.members.length !== 1 ? 's' : ''}</span>
        </div>
      </div>
      <div class="header-actions">
        <button
          class="header-btn"
          aria-label="Finalize session"
          onclick={() => navigate(`/orgs/${orgId}/sessions/${sessionId}/finalize`)}
        >Finalize</button>
      </div>
    </div>

    <!-- WebSocket reconnect indicator (absent when the socket is healthy) -->
    <WsStatusBanner {sessionId} />

    <!-- Main body: tree rail | artifact -->
    <div class="top" class:tree-collapsed={treeState === 'tree-collapsed'} class:tree-expanded={treeState === 'tree-expanded'} class:tree-wide={treeState === 'tree-wide'}>
      <!-- Tree pane -->
      <aside class="pane tree" aria-label="Tree">
        <div class="tree-head">
          <span class="tree-title">tree · {session.members.length} refs</span>
          <button
            class="tree-resize-btn"
            onclick={cycleTree}
            title="cycle tree: {treeState}"
            aria-label="Cycle tree width"
          >⇔</button>
        </div>
        <div class="tree-scroll">
          <TreeDag
            {orgId}
            {sessionId}
            {treeState}
            selectedSha={selectedCommitSha}
            onselect={handleSelectCommit}
            onrefaction={handleRefAction}
          />
        </div>
      </aside>

      <!-- Artifact slot -->
      <main class="pane artifact" aria-label="Artifact">
        <div class="artifact-slot" data-selected-sha={selectedCommitSha ?? ''}>
          <ArtifactPane
            {sessionId}
            {orgId}
            selectedSha={selectedCommitSha}
            selectedPath={selectedFilePath}
            onrangeselect={handleRangeSelect}
          />
        </div>
        {#if composerOpen && selectedCommitSha}
          <div class="composer-overlay">
            <CommentComposer
              {orgId}
              {sessionId}
              anchorCommitSha={selectedCommitSha}
              anchorFilePath={selectedFilePath}
              anchorLineStart={composerRange?.start ?? null}
              anchorLineEnd={composerRange?.end ?? null}
              onsubmit={() => { composerOpen = false; }}
              oncancel={() => { composerOpen = false; }}
            />
          </div>
        {/if}
      </main>
    </div>

    <!-- Bottom panel: tabbed activity / comments -->
    <div class="bottom" class:expanded={bottomExpanded} aria-label="Bottom panel">
      <div class="bottom-tabs" role="tablist" aria-label="Bottom panel tabs">
        <button
          class="bottom-tab"
          class:active={activeTab === 'activity'}
          role="tab"
          aria-selected={activeTab === 'activity'}
          aria-controls="panel-activity"
          onclick={() => switchTab('activity')}
        >
          Activity
        </button>
        <button
          class="bottom-tab"
          class:active={activeTab === 'comments'}
          role="tab"
          aria-selected={activeTab === 'comments'}
          aria-controls="panel-comments"
          onclick={() => switchTab('comments')}
        >
          Comments
        </button>
        <div class="bottom-latest" aria-hidden="true">
          <span class="live-dot"></span>
          <span class="latest-text">Live session activity</span>
        </div>
        <button
          class="bottom-toggle"
          onclick={() => (bottomExpanded = !bottomExpanded)}
          aria-label={bottomExpanded ? 'Collapse panel' : 'Expand panel'}
          aria-expanded={bottomExpanded}
        >
          {bottomExpanded ? 'collapse ↓' : 'expand ↑'}
        </button>
      </div>

      {#if bottomExpanded}
        <div
          class="bottom-body"
          id="panel-activity"
          role="tabpanel"
          aria-labelledby="tab-activity"
          hidden={activeTab !== 'activity'}
        >
          {#if activeTab === 'activity'}
            <ActivityFeed {sessionId} />
          {/if}
        </div>
        <div
          class="bottom-body"
          id="panel-comments"
          role="tabpanel"
          aria-labelledby="tab-comments"
          hidden={activeTab !== 'comments'}
        >
          {#if activeTab === 'comments'}
            <CommentsTab {orgId} {sessionId} />
          {/if}
        </div>
      {/if}
    </div>

    <!-- Ref context menu -->
    {#if activeMenuRef}
      <RefActionsMenu
        ref={activeMenuRef.ref}
        x={activeMenuRef.x}
        y={activeMenuRef.y}
        onaction={handleMenuAction}
        onclose={() => { activeMenuRef = null; }}
      />
    {/if}

    <!-- Fork dialog -->
    {#if activeDialog === 'fork'}
      <ForkDialog
        {sessionId}
        sourceRef={activeDialogRef}
        onclose={closeDialog}
        onsuccess={closeDialog}
      />
    {/if}

    <!-- Mode-switch dialog -->
    {#if activeDialog === 'mode-switch'}
      <ModeSwitchDialog
        {orgId}
        {sessionId}
        ref={activeDialogRef}
        currentMode={activeDialogRefMode}
        onclose={closeDialog}
        onsuccess={closeDialog}
      />
    {/if}
  {:else}
    <div class="loading-shell" aria-busy="true">Loading session…</div>
  {/if}
</div>

<style>
  .shell {
    height: 100vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    font-family: var(--font-sans);
  }

  /* ── App chrome ─────────────────────────────────────────────────── */
  .app-chrome {
    padding: 10px 20px;
    display: flex;
    align-items: center;
    gap: 14px;
    border-bottom: 1px solid var(--color-border);
    flex-shrink: 0;
  }

  .wordmark {
    font: var(--font-weight-semibold) var(--font-size-base) var(--font-sans);
    letter-spacing: -0.03em;
    user-select: none;
  }

  .dot {
    color: var(--color-accent);
  }

  .breadcrumb {
    display: flex;
    gap: 6px;
    align-items: center;
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
  }

  .breadcrumb-link {
    background: transparent;
    border: 0;
    color: var(--color-text-secondary);
    cursor: pointer;
    font: inherit;
    font-size: var(--font-size-sm);
    padding: 0;
  }

  .breadcrumb-link:hover {
    color: var(--color-text-primary);
    text-decoration: underline;
  }

  .sep {
    color: var(--color-text-tertiary);
  }

  .here {
    color: var(--color-text-primary);
    font-weight: var(--font-weight-medium);
  }

  .chrome-spacer {
    flex: 1;
  }

  /* ── Session header ─────────────────────────────────────────────── */
  .session-header {
    padding: 16px 20px;
    border-bottom: 1px solid var(--color-border);
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 20px;
    align-items: start;
    flex-shrink: 0;
  }

  .session-header h1 {
    margin: 0 0 4px;
    font-size: var(--font-size-xl);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.02em;
  }

  .goal {
    margin: 0 0 10px;
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    max-width: 680px;
  }

  .meta-strip {
    display: flex;
    gap: 14px;
    align-items: center;
    font-size: 11px;
    color: var(--color-text-tertiary);
    font-family: var(--font-mono);
    flex-wrap: wrap;
  }

  .meta-strip code {
    background: var(--color-bg-tertiary);
    padding: 1px 5px;
    border-radius: 3px;
    color: var(--color-text-secondary);
    font-family: var(--font-mono);
  }

  .header-actions {
    display: flex;
    gap: 8px;
    flex-shrink: 0;
  }

  .header-btn {
    padding: 7px 14px;
    background: var(--color-bg-secondary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    cursor: pointer;
  }

  /* ── Top section: tree | artifact ──────────────────────────────── */
  .top {
    display: grid;
    flex: 1;
    min-height: 0;
    transition: grid-template-columns 0.2s ease;
  }

  .top.tree-collapsed {
    grid-template-columns: 56px minmax(0, 1fr);
  }

  .top.tree-expanded {
    grid-template-columns: 280px minmax(0, 1fr);
  }

  .top.tree-wide {
    grid-template-columns: 40% 60%;
  }

  .pane {
    overflow: hidden;
    display: flex;
    flex-direction: column;
    background: var(--color-bg-primary);
  }

  .pane.tree {
    border-right: 1px solid var(--color-border);
    background: var(--color-bg-secondary);
    position: relative;
  }

  .tree-head {
    padding: 8px 12px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--color-border);
    font: var(--font-weight-semibold) 10px var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--color-text-secondary);
    min-height: 24px;
    flex-shrink: 0;
  }

  .tree-title {
    display: none;
  }

  .top.tree-expanded .tree-title,
  .top.tree-wide .tree-title {
    display: inline;
  }

  .tree-resize-btn {
    width: 22px;
    height: 22px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    color: var(--color-text-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    cursor: pointer;
    font-size: 14px;
    padding: 0;
    flex-shrink: 0;
  }

  .tree-resize-btn:hover {
    background: var(--color-bg-tertiary);
    color: var(--color-text-primary);
  }

  .tree-scroll {
    flex: 1;
    overflow-y: auto;
  }

  .pane.artifact {
    background: var(--color-bg-primary);
    position: relative;
  }

  .artifact-slot {
    flex: 1;
    height: 100%;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }

  /* Composer overlay anchored to the bottom-right of the artifact pane */
  .composer-overlay {
    position: absolute;
    bottom: 16px;
    right: 16px;
    z-index: 50;
  }

  /* ── Bottom panel ──────────────────────────────────────────────── */
  .bottom {
    flex-shrink: 0;
    border-top: 1px solid var(--color-border);
    background: var(--color-bg-secondary);
    max-height: 44px;
    overflow: hidden;
    transition: max-height 0.2s ease;
    display: flex;
    flex-direction: column;
  }

  .bottom.expanded {
    max-height: 320px;
  }

  .bottom-tabs {
    display: flex;
    align-items: stretch;
    border-bottom: 1px solid var(--color-border);
    flex-shrink: 0;
  }

  .bottom-tab {
    padding: 12px 18px;
    background: transparent;
    border: 0;
    color: var(--color-text-secondary);
    font: var(--font-weight-medium) 11px var(--font-mono);
    cursor: pointer;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    border-bottom: 2px solid transparent;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin-bottom: -1px;
  }

  .bottom-tab.active {
    color: var(--color-accent);
    border-bottom-color: var(--color-accent);
    background: var(--color-bg-primary);
  }

  .bottom-latest {
    flex: 1;
    padding: 0 16px;
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    overflow: hidden;
  }

  .live-dot {
    display: inline-block;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--color-success);
    animation: pulse 1.5s ease infinite;
    flex-shrink: 0;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }

  .latest-text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--font-size-sm);
  }

  .bottom-toggle {
    padding: 4px 10px;
    background: transparent;
    color: var(--color-text-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    font: 10px var(--font-mono);
    cursor: pointer;
    margin: auto 18px;
  }

  .bottom-body {
    flex: 1;
    overflow-y: auto;
    padding: 14px 24px;
  }

  .bottom-body[hidden] {
    display: none;
  }

  /* ── Loading / error ────────────────────────────────────────────── */
  .loading-shell {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
  }

  .load-error {
    padding: 20px;
    color: var(--color-danger);
    font-size: var(--font-size-sm);
  }
</style>
