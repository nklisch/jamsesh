<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { subscribe } from '$lib/ws.svelte';
  import { auth } from '$lib/auth.svelte';
  import { navigate } from '$lib/router.svelte';
  import AuthorDot from '$lib/components/AuthorDot.svelte';
  import ThemeToggle from '$lib/components/ThemeToggle.svelte';
  import SquashMessageEditor from '$lib/components/SquashMessageEditor.svelte';
  import LockBanner from '$lib/components/finalize/LockBanner.svelte';
  import RefGroupList from '$lib/components/finalize/RefGroupList.svelte';
  import CommandRunner from '$lib/components/finalize/CommandRunner.svelte';
  import { finalizeLock } from '$lib/finalize/useFinalizeLock.svelte';
  import { finalizePlan } from '$lib/finalize/useFinalizePlan.svelte';
  import { finalizeCuration } from '$lib/finalize/useFinalizeCuration.svelte';
  import { finalizeExecution } from '$lib/finalize/useFinalizeExecution.svelte';
  import type { RefGroup } from '$lib/finalize/useFinalizeCuration.svelte';
  import type { components } from '$lib/api/types.gen';

  type PlanMode = components['schemas']['PlanMode'];

  let {
    orgId,
    sessionId,
  }: {
    orgId: string;
    sessionId: string;
  } = $props();

  // ── Orchestration-level derived ──────────────────────────────────────────
  // interactionsDisabled crosses three modules; computed here as the orchestrator.
  const interactionsDisabled = $derived(
    finalizeLock.conflict !== null ||
      finalizeExecution.sessionEnded ||
      !finalizeCuration.isCaller,
  );
  const runCommand = $derived(
    finalizePlan.planId ? `jamsesh finalize-run ${finalizePlan.planId}` : '',
  );

  // ── Curation mutation helpers (call schedulePatch after each) ────────────
  function triggerPatch() {
    if (interactionsDisabled) return;
    finalizePlan.schedulePatch({
      orgId,
      sessionId,
      lockId: finalizeLock.status?.lock_id ?? '',
      selectedShas: finalizeCuration.selectedShas,
      targetBranch: finalizeCuration.targetBranch,
      baseSha: finalizePlan.plan?.base_sha ?? '',
      mode: finalizeCuration.mode,
      commitMessage: finalizeCuration.commitMessage,
      onError: (msg) => finalizeLock.setError(msg),
      onSuccess: () =>
        void finalizePlan.refetch(orgId, sessionId, finalizeLock.status!.lock_id, (msg) =>
          finalizeLock.setError(msg),
        ),
    });
  }

  function addCommit(sha: string) {
    finalizeCuration.addCommit(sha);
    triggerPatch();
  }

  function removeCommit(sha: string) {
    finalizeCuration.removeCommit(sha);
    triggerPatch();
  }

  function moveUp(index: number) {
    finalizeCuration.moveUp(index);
    triggerPatch();
  }

  function moveDown(index: number) {
    finalizeCuration.moveDown(index);
    triggerPatch();
  }

  function addAllInGroup(group: RefGroup) {
    finalizeCuration.addAllInGroup(group);
    triggerPatch();
  }

  function setMode(next: PlanMode) {
    finalizeCuration.setMode(next);
    triggerPatch();
  }

  function setTargetBranch(e: Event) {
    finalizeCuration.setTargetBranch((e.currentTarget as HTMLInputElement).value);
    triggerPatch();
  }

  function setCommitMessage(next: string) {
    finalizeCuration.setCommitMessage(next);
    triggerPatch();
  }

  // ── Lock flow ─────────────────────────────────────────────────────────────
  async function acquireLock(opts: { override?: boolean } = {}): Promise<void> {
    await finalizeLock.acquire(orgId, sessionId, opts);
    if (finalizeLock.status && !finalizeLock.conflict) {
      // Set isCaller immediately from lock — don't wait for plan fetch.
      finalizeCuration.setIsCaller(finalizeLock.status.is_caller);
      const plan = await finalizePlan.refetch(
        orgId,
        sessionId,
        finalizeLock.status.lock_id,
        (msg) => finalizeLock.setError(msg),
      );
      if (plan) {
        finalizeCuration.setDistinctAuthors(plan.co_authors ?? []);
        // Refine isCaller from plan (server may have updated it).
        finalizeCuration.setIsCaller(plan.lock_status?.is_caller ?? finalizeLock.status.is_caller);
        finalizeCuration.adoptFromPlan({
          targetBranch: plan.target_branch ?? '',
          commitMessage: plan.mode === 'squash' ? (plan.commit_message ?? '') : '',
          selectedShas: plan.selected_commits.map((c) => c.sha),
        });
      }
      await finalizeCuration.loadRefs(
        orgId,
        sessionId,
        finalizePlan.plan?.selected_commits ?? [],
      );
    }
  }

  async function overrideLock(): Promise<void> {
    finalizePlan.cancelPendingPatch();
    await acquireLock({ override: true });
  }

  // ── WS subscriptions ─────────────────────────────────────────────────────
  const unsubs: Array<() => void> = [];

  function startSubscriptions(): void {
    unsubs.push(
      subscribe(sessionId, 'session.finalizing', () => {
        if (finalizeLock.status && finalizeCuration.isCaller) return;
        void acquireLock({ override: false });
      }),
    );
    unsubs.push(
      subscribe(sessionId, 'session.ended', (env) => {
        const payload = env as { reason?: string };
        if (payload.reason === 'shipped') {
          finalizeExecution.endSession();
        } else if (payload.reason === 'abandon' || payload.reason === 'timeout') {
          finalizeLock.setError(`Session ended (${payload.reason}).`);
        }
      }),
    );
    unsubs.push(
      subscribe(sessionId, 'mode.changed', () => {
        void finalizeCuration.loadRefs(
          orgId,
          sessionId,
          finalizePlan.plan?.selected_commits ?? [],
        );
        void finalizePlan.refetch(orgId, sessionId, finalizeLock.status?.lock_id ?? '', (msg) =>
          finalizeLock.setError(msg),
        );
      }),
    );
  }

  function stopSubscriptions(): void {
    for (const u of unsubs) u();
    unsubs.length = 0;
  }

  onMount(() => {
    // Reset all module singletons so this render starts with clean state.
    finalizeLock.reset();
    finalizePlan.reset();
    finalizeCuration.reset();
    finalizeExecution.reset();
    void (async () => {
      await acquireLock();
      startSubscriptions();
    })();
  });

  onDestroy(() => {
    finalizePlan.cancelPendingPatch();
    stopSubscriptions();
    if (finalizeLock.status && !finalizeExecution.sessionEnded && finalizeCuration.isCaller) {
      void finalizeLock.release(orgId, sessionId);
    }
  });
</script>

<div class="finalize-view">
  <!-- App chrome -->
  <header class="app-chrome">
    <div class="wordmark" aria-label="jamsesh">jam<span class="dot">·</span>sesh</div>
    <nav class="breadcrumb" aria-label="Breadcrumb">
      <button class="breadcrumb-link" onclick={() => navigate(`/orgs/${orgId}/sessions`)}>
        {orgId}
      </button>
      <span class="sep" aria-hidden="true">/</span>
      <button class="breadcrumb-link" onclick={() => navigate(`/orgs/${orgId}/sessions/${sessionId}`)}>
        {sessionId}
      </button>
      <span class="sep" aria-hidden="true">/</span>
      <span class="here">finalize</span>
    </nav>
    <div class="chrome-spacer"></div>
    <ThemeToggle />
    {#if auth.currentUser}
      <AuthorDot authorId={auth.currentUser.id} size={26} title={auth.currentUser.displayName} />
    {/if}
  </header>

  <LockBanner
    lockConflict={finalizeLock.conflict}
    lockError={finalizeLock.error}
    lock={finalizeLock.status}
    isCaller={finalizeCuration.isCaller}
    sessionEnded={finalizeExecution.sessionEnded}
    onWait={() => navigate(`/orgs/${orgId}/sessions/${sessionId}`)}
    onOverride={() => void overrideLock()}
    onDismissError={() => finalizeLock.dismissError()}
  />

  <!-- Page head -->
  <section class="page-head">
    <h1>Finalize session</h1>
    <div class="sub">
      {#if finalizePlan.plan}
        Base <code>{finalizePlan.plan.base_sha.slice(0, 7)}</code> ·
        pick the commits to ship.
      {:else if finalizeLock.loading}
        Loading lock…
      {:else if finalizeLock.conflict}
        Held by another member.
      {:else}
        Preparing plan…
      {/if}
    </div>
  </section>

  {#if !finalizeLock.conflict}
    <!-- Mode bar -->
    <section class="mode-bar" aria-label="Finalization mode">
      <h3>Finalization mode</h3>
      <div class="seg" role="group" aria-label="Mode selector">
        <button
          type="button"
          class:on={finalizeCuration.mode === 'squash'}
          onclick={() => setMode('squash')}
          aria-pressed={finalizeCuration.mode === 'squash'}
          disabled={interactionsDisabled}
        >Squash into one commit</button>
        <button
          type="button"
          class:on={finalizeCuration.mode === 'preserve'}
          onclick={() => setMode('preserve')}
          aria-pressed={finalizeCuration.mode === 'preserve'}
          disabled={interactionsDisabled}
        >Preserve all commits</button>
      </div>
      <div class="mode-help">
        {#if finalizeCuration.mode === 'squash'}
          <strong>Squash</strong> creates a single commit on the
          target branch with every contributor as a
          <code>Co-authored-by</code> trailer.
        {:else}
          <strong>Preserve</strong> cherry-picks each curated commit
          individually onto the target branch.
        {/if}
      </div>
    </section>

    <!-- Main: source pool | cart -->
    <main class="main">
      <!-- Source pool -->
      <section class="panel source" aria-label="Available commits">
        <RefGroupList
          refs={finalizeCuration.availableGroups}
          selected={new Set(finalizeCuration.selectedShas)}
          onToggle={(sha) =>
            finalizeCuration.selectedShas.includes(sha) ? removeCommit(sha) : addCommit(sha)}
          onAddAll={(group) => addAllInGroup(group)}
          disabled={interactionsDisabled}
        />
      </section>

      <!-- Cart panel -->
      <section class="panel cart" aria-label="Your final branch">
        <div class="panel-head cart-head">
          <h2>Your final branch</h2>
          <span class="reorder-hint" aria-hidden="true">▲ ▼ to reorder</span>
          <div class="meta">{finalizeCuration.selectedShas.length} selected</div>
        </div>

        <div class="cart-config">
          <div class="field-row">
            <label for="target-branch">Target branch</label>
            <input
              id="target-branch"
              type="text"
              value={finalizeCuration.targetBranch}
              oninput={setTargetBranch}
              disabled={interactionsDisabled}
              placeholder="jamsesh/feature-name"
            />
          </div>

          {#if finalizeCuration.mode === 'squash'}
            <div class="msg-editor">
              <SquashMessageEditor
                message={finalizeCuration.commitMessage}
                onmessagechange={setCommitMessage}
                coAuthors={finalizeCuration.distinctAuthors}
              />
            </div>
          {/if}
        </div>

        <div class="cart-body">
          {#if finalizeCuration.selectedShas.length === 0}
            <p class="empty">No commits selected. Toggle commits on the left to add them.</p>
          {/if}
          {#each finalizeCuration.selectedShas as sha, index (sha)}
            {@const cartCommit = finalizePlan.plan?.selected_commits.find((c) => c.sha === sha)}
            <div class="cart-item">
              <span class="num">{index + 1}</span>
              <div class="body">
                <div class="top">
                  {#if cartCommit}
                    <AuthorDot authorId={cartCommit.account_id ?? cartCommit.author_email} size={9} />
                    <span class="name">{cartCommit.author_name}</span>
                  {:else}
                    <span class="sha">unknown author</span>
                  {/if}
                  <span class="sha">{sha.slice(0, 7)}</span>
                </div>
                <div class="msg">{cartCommit?.subject ?? '(awaiting plan)'}</div>
              </div>
              <div class="reorder" aria-label="Reorder">
                <button
                  type="button"
                  aria-label="Move up"
                  disabled={interactionsDisabled || index === 0}
                  onclick={() => moveUp(index)}
                >▲</button>
                <button
                  type="button"
                  aria-label="Move down"
                  disabled={interactionsDisabled || index === finalizeCuration.selectedShas.length - 1}
                  onclick={() => moveDown(index)}
                >▼</button>
              </div>
              <button
                class="remove"
                type="button"
                aria-label="Remove from cart"
                disabled={interactionsDisabled}
                onclick={() => removeCommit(sha)}
              >×</button>
            </div>
          {/each}
        </div>

        <footer class="cart-foot">
          {#if finalizePlan.plan}
            <div class="summary-block">
              {#if finalizeCuration.mode === 'squash'}
                This will create a branch <strong>{finalizeCuration.targetBranch || '<target>'}</strong>
                from base commit <code>{finalizePlan.plan.base_sha.slice(0, 7)}</code>,
                then squash {finalizeCuration.selectedShas.length} commits from
                {finalizeCuration.distinctAuthors.length} author{finalizeCuration.distinctAuthors.length === 1 ? '' : 's'} into
                one commit. Conflicts during the squash will be left in
                your working tree for you to resolve. Nothing will be
                pushed.
              {:else}
                This will create a branch <strong>{finalizeCuration.targetBranch || '<target>'}</strong>
                from base commit <code>{finalizePlan.plan.base_sha.slice(0, 7)}</code>,
                then cherry-pick {finalizeCuration.selectedShas.length} commits in order.
                Conflicts during cherry-pick will be left in your
                working tree for you to resolve. Nothing will be pushed.
              {/if}
            </div>
          {/if}

          <div class="stats" aria-label="Stats">
            <div class="stat">
              <span class="label">Commits</span>
              <span class="val">{finalizeCuration.selectedShas.length}</span>
            </div>
            <div class="stat">
              <span class="label">Authors</span>
              <span class="val">{finalizeCuration.distinctAuthors.length}</span>
            </div>
            <div class="stat">
              <span class="label">Mode</span>
              <span class="val">{finalizeCuration.mode}</span>
            </div>
          </div>

          <CommandRunner
            command={runCommand}
            ready={!!runCommand}
            disabled={interactionsDisabled || !finalizeCuration.canRun}
            oncopy={() => finalizeExecution.markCopied()}
          />

          {#if finalizeExecution.sessionEnded}
            <div class="shipped-state">
              <strong>Shipped!</strong> Session marked as ended.
              <button class="btn ghost" type="button" onclick={() => navigate(`/orgs/${orgId}/sessions`)}>
                Back to sessions
              </button>
            </div>
          {:else}
            <button
              class="ship-btn"
              type="button"
              onclick={() =>
                void finalizeExecution.markShipped(
                  orgId,
                  sessionId,
                  finalizeCuration.targetBranch,
                  () => finalizePlan.cancelPendingPatch(),
                  (msg) => finalizeLock.setError(msg),
                )}
              disabled={interactionsDisabled || finalizeExecution.markShippedInFlight}
            >
              {finalizeExecution.markShippedInFlight ? 'Marking…' : 'Mark as shipped'}
            </button>
            {#if finalizeExecution.copiedRunCommand}
              <p class="ship-hint">Once you've run the command and pushed, click "Mark as shipped".</p>
            {/if}
          {/if}
        </footer>
      </section>
    </main>
  {/if}

</div>

<style>
  .finalize-view {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    font-family: var(--font-sans);
  }

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
  }
  .wordmark .dot { color: var(--color-accent); }
  .breadcrumb {
    display: flex; gap: 6px; align-items: center;
    font-size: var(--font-size-sm); color: var(--color-text-secondary);
  }
  .breadcrumb-link {
    background: transparent; border: 0;
    color: var(--color-text-secondary);
    cursor: pointer; font: inherit; padding: 0;
  }
  .breadcrumb-link:hover { color: var(--color-text-primary); text-decoration: underline; }
  .here { color: var(--color-text-primary); font-weight: var(--font-weight-medium); }
  .sep { color: var(--color-text-tertiary); }
  .chrome-spacer { flex: 1; }

  .page-head {
    padding: 22px 28px 16px;
    border-bottom: 1px solid var(--color-border);
  }
  .page-head h1 {
    margin: 0 0 4px;
    font-size: var(--font-size-xl);
    font-weight: var(--font-weight-semibold);
  }
  .sub { font-size: var(--font-size-sm); color: var(--color-text-secondary); }
  .sub code {
    font: var(--font-size-xs) var(--font-mono);
    background: var(--color-bg-tertiary);
    padding: 1px 6px; border-radius: 3px;
    color: var(--color-text-primary);
  }

  .mode-bar {
    padding: 14px 28px;
    display: flex; gap: 18px; align-items: center;
    border-bottom: 1px solid var(--color-border);
    background: var(--color-bg-secondary);
  }
  .mode-bar h3 {
    margin: 0;
    font: var(--font-weight-semibold) 10px var(--font-mono);
    text-transform: uppercase; letter-spacing: 0.1em;
    color: var(--color-text-secondary);
  }
  .seg {
    display: inline-flex; border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md); overflow: hidden;
    background: var(--color-bg-primary);
  }
  .seg button {
    border: 0; background: transparent;
    padding: 7px 16px;
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    color: var(--color-text-secondary); cursor: pointer;
    border-right: 1px solid var(--color-border-strong);
  }
  .seg button:last-child { border-right: 0; }
  .seg button:hover:not(:disabled) { background: var(--color-bg-tertiary); color: var(--color-text-primary); }
  .seg button.on { background: var(--color-bg-inverse); color: var(--color-text-inverse); }
  .seg button:disabled { opacity: 0.5; cursor: not-allowed; }
  .mode-help { flex: 1; font-size: var(--font-size-sm); color: var(--color-text-secondary); }
  .mode-help strong { color: var(--color-text-primary); font-weight: var(--font-weight-semibold); }

  .main {
    flex: 1; display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1.1fr);
    min-height: 0;
  }
  .panel { display: flex; flex-direction: column; min-height: 0; overflow: hidden; }
  .panel-head {
    padding: 14px 22px;
    display: flex; align-items: center; gap: 12px;
    border-bottom: 1px solid var(--color-border);
    background: var(--color-bg-secondary);
  }
  .panel-head h2 {
    margin: 0; font-size: var(--font-size-base);
    font-weight: var(--font-weight-semibold);
  }
  .panel-head .meta {
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-mono);
    color: var(--color-text-secondary); margin-left: auto;
  }

  .panel.source { border-right: 1px solid var(--color-border); background: var(--color-bg-primary); display: flex; flex-direction: column; min-height: 0; }

  .panel.cart { background: var(--color-bg-secondary); display: flex; flex-direction: column; min-height: 0; }
  .cart-head .reorder-hint { font-size: var(--font-size-xs); color: var(--color-text-tertiary); margin-left: 8px; }

  .cart-config {
    padding: 16px 22px;
    border-bottom: 1px solid var(--color-border);
    background: var(--color-bg-primary);
    display: grid; gap: 12px;
  }
  .field-row {
    display: grid; grid-template-columns: 120px 1fr; gap: 14px;
    align-items: center;
  }
  .field-row label {
    font: var(--font-weight-semibold) 10px var(--font-mono);
    text-transform: uppercase; letter-spacing: 0.1em;
    color: var(--color-text-secondary);
  }
  .field-row input[type='text'] {
    padding: 8px 12px; border-radius: var(--radius-sm);
    border: 1px solid var(--color-border-strong);
    background: var(--color-bg-secondary); color: var(--color-text-primary);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-mono);
  }
  .field-row input[type='text']:focus {
    outline: 2px solid var(--color-accent); outline-offset: -1px;
    border-color: var(--color-accent);
  }
  .msg-editor { grid-column: 1 / -1; display: grid; gap: 8px; }

  .cart-body { flex: 1; overflow: auto; padding: 12px 22px 16px; }
  .empty { color: var(--color-text-secondary); font-size: var(--font-size-sm); }

  .cart-item {
    display: grid; grid-template-columns: 32px 1fr auto auto; gap: 10px;
    padding: 10px 14px;
    background: var(--color-bg-primary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    margin-bottom: 8px;
    align-items: center;
  }
  .cart-item .num {
    width: 24px; height: 24px;
    display: inline-flex; align-items: center; justify-content: center;
    background: var(--color-bg-tertiary); color: var(--color-text-secondary);
    border-radius: var(--radius-sm);
    font: var(--font-weight-semibold) 11px var(--font-mono);
  }
  .cart-item .body { min-width: 0; }
  .cart-item .top {
    display: flex; gap: 7px; align-items: center;
    font: var(--font-size-xs) var(--font-mono); color: var(--color-text-tertiary);
    margin-bottom: 3px;
  }
  .cart-item .top .name { color: var(--color-text-secondary); font-weight: var(--font-weight-medium); }
  .cart-item .msg {
    font-size: var(--font-size-sm); color: var(--color-text-primary);
    line-height: 1.35;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .cart-item .reorder { display: flex; flex-direction: column; }
  .cart-item .reorder button {
    width: 22px; height: 16px; border: 0;
    background: transparent; color: var(--color-text-tertiary);
    cursor: pointer; font-size: 10px;
  }
  .cart-item .reorder button:hover:not(:disabled) { color: var(--color-text-primary); }
  .cart-item .reorder button:disabled { opacity: 0.3; cursor: not-allowed; }
  .cart-item .remove {
    width: 26px; height: 26px;
    border: 1px solid var(--color-border-strong);
    background: var(--color-bg-secondary); color: var(--color-danger);
    border-radius: var(--radius-sm); cursor: pointer;
    font: var(--font-weight-semibold) 13px var(--font-sans);
  }
  .cart-item .remove:hover:not(:disabled) { background: var(--color-danger-muted); }
  .cart-item .remove:disabled { opacity: 0.5; cursor: not-allowed; }

  .cart-foot {
    padding: 14px 22px 18px;
    border-top: 1px solid var(--color-border);
    background: var(--color-bg-primary);
  }
  .summary-block {
    font-size: var(--font-size-sm); color: var(--color-text-secondary);
    line-height: 1.6;
    margin-bottom: 14px;
  }
  .summary-block strong { color: var(--color-text-primary); font-weight: var(--font-weight-semibold); }
  .summary-block code {
    font: var(--font-size-xs) var(--font-mono);
    background: var(--color-bg-tertiary); padding: 1px 5px;
    border-radius: 3px; color: var(--color-text-primary);
  }

  .stats {
    display: flex; gap: 16px;
    padding: 10px 12px; margin: 10px 0;
    background: var(--color-bg-tertiary); border-radius: var(--radius-md);
  }
  .stats .stat { display: flex; flex-direction: column; gap: 2px; }
  .stats .label {
    font: 10px var(--font-mono); color: var(--color-text-tertiary);
    text-transform: uppercase; letter-spacing: 0.06em;
  }
  .stats .val {
    font: var(--font-weight-semibold) var(--font-size-base) var(--font-mono);
    color: var(--color-text-primary);
  }

  .ship-btn {
    width: 100%; padding: 10px; margin-top: 10px;
    background: var(--color-bg-secondary); color: var(--color-text-primary);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    cursor: pointer;
  }
  .ship-btn:hover:not(:disabled) { background: var(--color-bg-tertiary); }
  .ship-btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .ship-hint { font-size: var(--font-size-xs); color: var(--color-text-secondary); margin: 6px 0 0; }

  .shipped-state {
    margin-top: 12px; padding: 12px;
    background: var(--color-accent-muted); color: var(--color-accent);
    border-radius: var(--radius-md);
    display: flex; align-items: center; gap: 12px; justify-content: space-between;
  }

  .btn {
    padding: 7px 14px;
    border-radius: var(--radius-md);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    cursor: pointer;
  }
  .btn.primary {
    background: var(--color-bg-inverse); color: var(--color-text-inverse);
    border: 1px solid var(--color-bg-inverse);
    width: auto; padding: 7px 14px;
  }
  .btn.ghost {
    background: transparent; color: var(--color-text-primary);
    border: 1px solid var(--color-border-strong);
  }

</style>
