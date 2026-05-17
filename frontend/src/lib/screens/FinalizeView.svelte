<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { client } from '$lib/api/client';
  import { subscribe } from '$lib/ws.svelte';
  import { auth } from '$lib/auth.svelte';
  import { navigate } from '$lib/router.svelte';
  import AuthorDot from '$lib/components/AuthorDot.svelte';
  import ThemeToggle from '$lib/components/ThemeToggle.svelte';
  import SquashMessageEditor from '$lib/components/SquashMessageEditor.svelte';
  import LockBanner from '$lib/components/finalize/LockBanner.svelte';
  import type { components } from '$lib/api/types.gen';

  type LockStatus = components['schemas']['LockStatus'];
  type PlanResponse = components['schemas']['PlanResponse'];
  type PlanMode = components['schemas']['PlanMode'];
  type PlanCommit = components['schemas']['PlanCommit'];
  type Ref = components['schemas']['Ref'];

  // Client-side source-pool grouping. Until the plan endpoint exposes
  // an explicit `available_refs` field, we derive groups from the
  // session refs list (one card per ref). Commits-per-ref expansion
  // is left as a stub; v1 surfaces the ref tip + a single "add tip"
  // affordance plus whatever the plan already carries as
  // `selected_commits`.
  type SourceCommit = {
    sha: string;
    author_name: string;
    author_id: string;
    subject: string;
  };
  type RefGroup = {
    ref: string;
    kind: 'draft' | 'isolated' | 'sync';
    tip_sha: string;
    commits: SourceCommit[];
  };

  let {
    orgId,
    sessionId,
  }: {
    orgId: string;
    sessionId: string;
  } = $props();

  // ── Connection / lock state ─────────────────────────────────────────
  let lock = $state<LockStatus | null>(null);
  let lockConflict = $state<{ holderAccountId: string } | null>(null);
  let lockError = $state<string | null>(null);
  let lockLoading = $state(true);

  // ── Curation state (single source of truth) ─────────────────────────
  let selectedShas = $state<string[]>([]);
  let availableGroups = $state<RefGroup[]>([]);
  let mode = $state<PlanMode>('squash');
  let targetBranch = $state<string>('');
  let commitMessage = $state<string>(''); // squash-only

  // ── Plan readout ────────────────────────────────────────────────────
  let plan = $state<PlanResponse | null>(null);
  let planLoading = $state(false);

  // ── Execution UX ────────────────────────────────────────────────────
  let copyToastVisible = $state<boolean>(false);
  let copyToastTimer: ReturnType<typeof setTimeout> | null = null;
  let copiedRunCommand = $state<boolean>(false);
  let markShippedInFlight = $state<boolean>(false);
  let sessionEnded = $state<boolean>(false);

  // ── Derived ─────────────────────────────────────────────────────────
  const distinctAuthors = $derived(plan?.co_authors ?? []);
  const isCaller = $derived(plan?.lock_status?.is_caller ?? lock?.is_caller ?? false);
  const canRun = $derived(
    selectedShas.length > 0 &&
      targetBranch.trim().length > 0 &&
      (mode === 'preserve' || commitMessage.trim().length > 0),
  );
  const interactionsDisabled = $derived(
    lockConflict !== null || sessionEnded || !isCaller,
  );
  const planId = $derived(plan?.plan_id ?? '');
  const runCommand = $derived(planId ? `jamsesh finalize-run ${planId}` : '');

  // ── PATCH debounce (NOT in $state — identity changes shouldn't drive UI) ─
  let patchTimer: ReturnType<typeof setTimeout> | null = null;
  let patchSeq = 0;

  function schedulePatch() {
    if (interactionsDisabled) return;
    if (patchTimer !== null) clearTimeout(patchTimer);
    patchTimer = setTimeout(() => {
      void flushPatch();
    }, 300);
  }

  async function flushPatch(): Promise<void> {
    if (!lock) return;
    patchTimer = null;
    const seq = ++patchSeq;
    const body = {
      selected_commit_shas: selectedShas,
      target_branch: targetBranch,
      base_sha: plan?.base_sha ?? '',
      mode,
      commit_message: mode === 'squash' ? commitMessage : null,
    };
    const { error } = await client.PATCH(
      '/api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}',
      {
        params: { path: { orgID: orgId, sessionID: sessionId, lockID: lock.lock_id } },
        body,
      },
    );
    if (seq !== patchSeq) return; // stale; a newer PATCH has been scheduled/fired
    if (error) {
      lockError = errorMessage(error, 'Failed to update curation state.');
      return;
    }
    await refetchPlan();
  }

  function errorMessage(err: unknown, fallback: string): string {
    if (err && typeof err === 'object' && 'message' in err) {
      const m = (err as { message?: unknown }).message;
      if (typeof m === 'string' && m.length > 0) return m;
    }
    return fallback;
  }

  async function refetchPlan(): Promise<void> {
    if (!lock) return;
    planLoading = true;
    const { data, error } = await client.GET(
      '/api/orgs/{orgID}/sessions/{sessionID}/finalize-plan',
      {
        params: {
          path: { orgID: orgId, sessionID: sessionId },
          query: { lock_id: lock.lock_id },
        },
      },
    );
    planLoading = false;
    if (!error && data) {
      plan = data;
      // Refresh the squash commit message from the server when the
      // user hasn't edited it (the editor is uncontrolled at the
      // textarea level but parent holds the truth).
      if (data.mode === 'squash' && (data.commit_message ?? '') !== '' && commitMessage === '') {
        commitMessage = data.commit_message ?? '';
      }
      // Adopt server-side target_branch / mode if local is empty
      // (first plan load case).
      if (!targetBranch) targetBranch = data.target_branch ?? '';
      if (selectedShas.length === 0) {
        selectedShas = data.selected_commits.map((c) => c.sha);
      }
    }
  }

  // Build groups from the session refs list. This is the v1
  // approximation of `available_refs`; the design's RefGroup shape
  // expects per-ref commit lists, but the current API only returns
  // ref tips. We expose each ref as a single-commit group.
  function deriveGroupsFromRefs(refs: Ref[], planCommits: PlanCommit[]): RefGroup[] {
    const byRef = new Map<string, RefGroup>();
    const planByPos = new Map(planCommits.map((c) => [c.sha, c]));
    for (const r of refs) {
      const kind: RefGroup['kind'] = r.ref.includes('/draft')
        ? 'draft'
        : r.mode === 'isolated'
          ? 'isolated'
          : 'sync';
      const planMatch = planByPos.get(r.sha);
      const commit: SourceCommit = planMatch
        ? {
            sha: planMatch.sha,
            author_name: planMatch.author_name,
            author_id: planMatch.account_id ?? planMatch.author_email,
            subject: planMatch.subject,
          }
        : {
            sha: r.sha,
            author_name: r.ref.split('/').slice(-2, -1)[0] ?? r.ref,
            author_id: r.ref,
            subject: shortRefName(r.ref),
          };
      byRef.set(r.ref, { ref: r.ref, kind, tip_sha: r.sha, commits: [commit] });
    }
    return Array.from(byRef.values());
  }

  function shortRefName(ref: string): string {
    return ref.replace(/^refs\/heads\//, '');
  }

  async function loadRefs(): Promise<void> {
    const { data, error } = await client.GET('/api/orgs/{orgID}/sessions/{sessionID}/refs', {
      params: { path: { orgID: orgId, sessionID: sessionId } },
    });
    if (!error && data) {
      availableGroups = deriveGroupsFromRefs(data.refs, plan?.selected_commits ?? []);
    }
  }

  // ── Lock acquisition ────────────────────────────────────────────────
  async function acquireLock(opts: { override?: boolean } = {}): Promise<void> {
    lockLoading = true;
    lockError = null;
    const { data, error, response } = await client.POST(
      '/api/orgs/{orgID}/sessions/{sessionID}/finalize/lock',
      {
        params: { path: { orgID: orgId, sessionID: sessionId } },
        body: { override: opts.override === true },
      },
    );
    lockLoading = false;
    if (error) {
      if (response?.status === 409) {
        // ErrorEnvelope: { error, message, details? }
        const env = error as { details?: Record<string, unknown> };
        const holderId = (env.details?.held_by_account_id as string | undefined) ?? '';
        lockConflict = { holderAccountId: holderId };
        return;
      }
      lockError = errorMessage(error, 'Failed to acquire lock.');
      return;
    }
    if (data) {
      lock = data;
      lockConflict = null;
      await refetchPlan();
      await loadRefs();
    }
  }

  async function overrideLock(): Promise<void> {
    if (patchTimer !== null) clearTimeout(patchTimer);
    patchTimer = null;
    await acquireLock({ override: true });
  }

  // ── Mark shipped ────────────────────────────────────────────────────
  async function markShipped(): Promise<void> {
    if (markShippedInFlight) return;
    markShippedInFlight = true;
    // Cancel any pending PATCH; we're about to end the session.
    if (patchTimer !== null) clearTimeout(patchTimer);
    patchTimer = null;
    const { error } = await client.POST(
      '/api/orgs/{orgID}/sessions/{sessionID}/mark-shipped',
      {
        params: { path: { orgID: orgId, sessionID: sessionId } },
        body: { final_branch_name: targetBranch.trim() || null },
      },
    );
    markShippedInFlight = false;
    if (error) {
      lockError = errorMessage(error, 'Failed to mark shipped.');
      return;
    }
    sessionEnded = true;
  }

  // ── Run-locally copy ────────────────────────────────────────────────
  async function copyRunCommand(): Promise<void> {
    if (!runCommand) return;
    try {
      await navigator.clipboard.writeText(runCommand);
    } catch {
      // best-effort; surface as a soft error if the user clicked.
      lockError = 'Failed to copy to clipboard.';
      return;
    }
    copiedRunCommand = true;
    copyToastVisible = true;
    if (copyToastTimer !== null) clearTimeout(copyToastTimer);
    copyToastTimer = setTimeout(() => {
      copyToastVisible = false;
      copyToastTimer = null;
    }, 1500);
  }

  // ── Curation mutations ──────────────────────────────────────────────
  function addCommit(sha: string) {
    if (selectedShas.includes(sha)) return;
    selectedShas = [...selectedShas, sha];
    schedulePatch();
  }

  function removeCommit(sha: string) {
    selectedShas = selectedShas.filter((s) => s !== sha);
    schedulePatch();
  }

  function moveUp(index: number) {
    if (index <= 0) return;
    const next = [...selectedShas];
    [next[index - 1], next[index]] = [next[index], next[index - 1]];
    selectedShas = next;
    schedulePatch();
  }

  function moveDown(index: number) {
    if (index >= selectedShas.length - 1) return;
    const next = [...selectedShas];
    [next[index + 1], next[index]] = [next[index], next[index + 1]];
    selectedShas = next;
    schedulePatch();
  }

  function addAllInGroup(group: RefGroup) {
    const additions = group.commits
      .map((c) => c.sha)
      .filter((sha) => !selectedShas.includes(sha));
    if (additions.length === 0) return;
    selectedShas = [...selectedShas, ...additions];
    schedulePatch();
  }

  function setMode(next: PlanMode) {
    if (mode === next) return;
    mode = next;
    schedulePatch();
  }

  function setTargetBranch(e: Event) {
    targetBranch = (e.currentTarget as HTMLInputElement).value;
    schedulePatch();
  }

  function setCommitMessage(next: string) {
    commitMessage = next;
    schedulePatch();
  }

  // ── WS reactions ────────────────────────────────────────────────────
  // Backend currently emits: session.finalizing (one-shot on
  // active→finalizing), session.ended (with reason), mode.changed.
  // There's no dedicated lock-acquired/released event; override
  // detection happens via PATCH returning 409 (handled in flushPatch).
  const unsubs: Array<() => void> = [];

  function startSubscriptions(): void {
    unsubs.push(
      subscribe(sessionId, 'session.finalizing', () => {
        // Someone (possibly us, possibly an overrider) just started
        // finalizing. If we don't hold the lock, render the conflict
        // banner. If we DO hold it, no-op.
        if (lock && isCaller) return;
        // Best-effort: re-check by re-reading the lock status; the
        // simplest signal is to acquire (which is idempotent for the
        // current holder; 409 if held by someone else).
        void acquireLock({ override: false });
      }),
    );
    unsubs.push(
      subscribe(sessionId, 'session.ended', (env) => {
        const payload = env as { reason?: string };
        if (payload.reason === 'shipped') {
          sessionEnded = true;
        } else if (payload.reason === 'abandon' || payload.reason === 'timeout') {
          lockError = `Session ended (${payload.reason}).`;
        }
      }),
    );
    unsubs.push(
      subscribe(sessionId, 'mode.changed', () => {
        // A ref-mode flip may shift the source-pool grouping. Refetch
        // refs and the plan to repopulate.
        void loadRefs();
        void refetchPlan();
      }),
    );
  }

  function stopSubscriptions(): void {
    for (const u of unsubs) u();
    unsubs.length = 0;
  }

  onMount(() => {
    void (async () => {
      await acquireLock();
      startSubscriptions();
    })();
  });

  onDestroy(() => {
    // Cancel pending PATCH; ditch WS subscriptions.
    if (patchTimer !== null) clearTimeout(patchTimer);
    patchTimer = null;
    if (copyToastTimer !== null) clearTimeout(copyToastTimer);
    copyToastTimer = null;
    stopSubscriptions();
    // Best-effort release of the lock if we still hold it and the
    // session hasn't ended (we don't await — the page is going away).
    if (lock && !sessionEnded && isCaller) {
      const { lock_id } = lock;
      void client.DELETE(
        '/api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}',
        { params: { path: { orgID: orgId, sessionID: sessionId, lockID: lock_id } } },
      );
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
    {lockConflict}
    {lockError}
    {lock}
    {isCaller}
    {sessionEnded}
    onWait={() => navigate(`/orgs/${orgId}/sessions/${sessionId}`)}
    onOverride={() => void overrideLock()}
    onDismissError={() => { lockError = null; }}
  />

  <!-- Page head -->
  <section class="page-head">
    <h1>Finalize session</h1>
    <div class="sub">
      {#if plan}
        Base <code>{plan.base_sha.slice(0, 7)}</code> ·
        pick the commits to ship.
      {:else if lockLoading}
        Loading lock…
      {:else if lockConflict}
        Held by another member.
      {:else}
        Preparing plan…
      {/if}
    </div>
  </section>

  {#if !lockConflict}
    <!-- Mode bar -->
    <section class="mode-bar" aria-label="Finalization mode">
      <h3>Finalization mode</h3>
      <div class="seg" role="group" aria-label="Mode selector">
        <button
          type="button"
          class:on={mode === 'squash'}
          onclick={() => setMode('squash')}
          aria-pressed={mode === 'squash'}
          disabled={interactionsDisabled}
        >Squash into one commit</button>
        <button
          type="button"
          class:on={mode === 'preserve'}
          onclick={() => setMode('preserve')}
          aria-pressed={mode === 'preserve'}
          disabled={interactionsDisabled}
        >Preserve all commits</button>
      </div>
      <div class="mode-help">
        {#if mode === 'squash'}
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
        <div class="panel-head">
          <h2>Available commits</h2>
          <div class="meta">{availableGroups.length} refs</div>
        </div>
        <div class="panel-body">
          {#each availableGroups as group (group.ref)}
            <div class="group-card">
              <header class="group-head">
                <span class="name">{shortRefName(group.ref)}</span>
                <span class="tag {group.kind}">{group.kind}</span>
                <span class="count">{group.commits.length} commit{group.commits.length === 1 ? '' : 's'}</span>
                <span class="actions">
                  <button type="button" disabled={interactionsDisabled} onclick={() => addAllInGroup(group)}>
                    Add all
                  </button>
                </span>
              </header>
              {#each group.commits as c (c.sha)}
                {@const inCart = selectedShas.includes(c.sha)}
                <div class="commit-row" class:in-cart={inCart}>
                  <div class="body">
                    <div class="top">
                      <AuthorDot authorId={c.author_id} size={9} />
                      <span class="name">{c.author_name}</span>
                      <span class="sha">{c.sha.slice(0, 7)}</span>
                    </div>
                    <div class="msg">{c.subject}</div>
                  </div>
                  <button
                    class="add-btn"
                    type="button"
                    aria-label={inCart ? `Remove ${c.sha.slice(0, 7)}` : `Add ${c.sha.slice(0, 7)}`}
                    disabled={interactionsDisabled}
                    onclick={() => (inCart ? removeCommit(c.sha) : addCommit(c.sha))}
                  >{inCart ? '−' : '+'}</button>
                </div>
              {/each}
            </div>
          {:else}
            <p class="empty">No refs available yet.</p>
          {/each}
        </div>
      </section>

      <!-- Cart panel -->
      <section class="panel cart" aria-label="Your final branch">
        <div class="panel-head cart-head">
          <h2>Your final branch</h2>
          <span class="reorder-hint" aria-hidden="true">▲ ▼ to reorder</span>
          <div class="meta">{selectedShas.length} selected</div>
        </div>

        <div class="cart-config">
          <div class="field-row">
            <label for="target-branch">Target branch</label>
            <input
              id="target-branch"
              type="text"
              value={targetBranch}
              oninput={setTargetBranch}
              disabled={interactionsDisabled}
              placeholder="jamsesh/feature-name"
            />
          </div>

          {#if mode === 'squash'}
            <div class="msg-editor">
              <SquashMessageEditor
                message={commitMessage}
                onmessagechange={setCommitMessage}
                coAuthors={distinctAuthors}
              />
            </div>
          {/if}
        </div>

        <div class="cart-body">
          {#if selectedShas.length === 0}
            <p class="empty">No commits selected. Toggle commits on the left to add them.</p>
          {/if}
          {#each selectedShas as sha, index (sha)}
            {@const cartCommit = plan?.selected_commits.find((c) => c.sha === sha)}
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
                  disabled={interactionsDisabled || index === selectedShas.length - 1}
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
          {#if plan}
            <div class="summary-block">
              {#if mode === 'squash'}
                This will create a branch <strong>{targetBranch || '<target>'}</strong>
                from base commit <code>{plan.base_sha.slice(0, 7)}</code>,
                then squash {selectedShas.length} commits from
                {distinctAuthors.length} author{distinctAuthors.length === 1 ? '' : 's'} into
                one commit. Conflicts during the squash will be left in
                your working tree for you to resolve. Nothing will be
                pushed.
              {:else}
                This will create a branch <strong>{targetBranch || '<target>'}</strong>
                from base commit <code>{plan.base_sha.slice(0, 7)}</code>,
                then cherry-pick {selectedShas.length} commits in order.
                Conflicts during cherry-pick will be left in your
                working tree for you to resolve. Nothing will be pushed.
              {/if}
            </div>
          {/if}

          <div class="stats" aria-label="Stats">
            <div class="stat">
              <span class="label">Commits</span>
              <span class="val">{selectedShas.length}</span>
            </div>
            <div class="stat">
              <span class="label">Authors</span>
              <span class="val">{distinctAuthors.length}</span>
            </div>
            <div class="stat">
              <span class="label">Mode</span>
              <span class="val">{mode}</span>
            </div>
          </div>

          {#if runCommand}
            <div class="copy-box">
              <code>{runCommand}</code>
              <button type="button" onclick={() => void copyRunCommand()} disabled={interactionsDisabled || !canRun}>
                Copy
              </button>
            </div>
          {/if}

          <button
            class="primary"
            type="button"
            onclick={() => void copyRunCommand()}
            disabled={interactionsDisabled || !canRun}
          >
            Run locally
          </button>

          {#if sessionEnded}
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
              onclick={() => void markShipped()}
              disabled={interactionsDisabled || markShippedInFlight}
            >
              {markShippedInFlight ? 'Marking…' : 'Mark as shipped'}
            </button>
            {#if copiedRunCommand}
              <p class="ship-hint">Once you've run the command and pushed, click "Mark as shipped".</p>
            {/if}
          {/if}
        </footer>
      </section>
    </main>
  {/if}

  {#if copyToastVisible}
    <div class="toast" role="status" aria-live="polite">Copied!</div>
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

  .panel.source { border-right: 1px solid var(--color-border); background: var(--color-bg-primary); }
  .panel.source .panel-body { overflow: auto; padding: 8px 14px 24px; flex: 1; }

  .group-card {
    margin: 10px 0;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-bg-secondary);
    overflow: hidden;
  }
  .group-head {
    padding: 10px 14px;
    display: flex; gap: 10px; align-items: center;
    background: var(--color-bg-tertiary);
    border-bottom: 1px solid var(--color-border);
  }
  .group-head .name {
    font: var(--font-weight-semibold) var(--font-size-sm) var(--font-mono);
    color: var(--color-text-primary);
  }
  .group-head .tag {
    font: 10px var(--font-mono); text-transform: uppercase; letter-spacing: 0.08em;
    padding: 2px 7px; border-radius: var(--radius-full);
  }
  .group-head .tag.draft { background: var(--color-accent-muted); color: var(--color-accent); }
  .group-head .tag.isolated { background: var(--color-warning-muted); color: var(--color-warning); }
  .group-head .tag.sync { background: var(--color-bg-tertiary); color: var(--color-text-secondary); }
  .group-head .count {
    margin-left: auto;
    font: var(--font-size-xs) var(--font-mono); color: var(--color-text-tertiary);
  }
  .group-head .actions { margin-left: 8px; }
  .group-head button {
    border: 1px solid var(--color-border); background: var(--color-bg-secondary);
    padding: 3px 8px; border-radius: var(--radius-sm);
    font: var(--font-weight-medium) 11px var(--font-sans);
    color: var(--color-text-primary); cursor: pointer;
  }
  .group-head button:hover:not(:disabled) { background: var(--color-bg-inverse); color: var(--color-text-inverse); }
  .group-head button:disabled { opacity: 0.5; cursor: not-allowed; }

  .commit-row {
    padding: 9px 14px;
    display: grid; grid-template-columns: 1fr auto; gap: 8px;
    align-items: center;
    border-bottom: 1px solid var(--color-border);
  }
  .commit-row:last-child { border-bottom: 0; }
  .commit-row.in-cart { background: var(--color-accent-muted); }
  .commit-row .body { min-width: 0; }
  .commit-row .top {
    display: flex; gap: 7px; align-items: center;
    font: var(--font-size-xs) var(--font-mono);
    color: var(--color-text-tertiary);
    margin-bottom: 3px;
  }
  .commit-row .top .name { color: var(--color-text-secondary); font-weight: var(--font-weight-medium); }
  .commit-row .top .sha { color: var(--color-text-secondary); }
  .commit-row .msg {
    font-size: var(--font-size-sm); color: var(--color-text-primary);
    line-height: 1.35;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .add-btn {
    width: 26px; height: 26px;
    border: 1px solid var(--color-border-strong);
    background: var(--color-bg-primary); color: var(--color-text-primary);
    border-radius: var(--radius-sm); cursor: pointer;
    font: var(--font-weight-semibold) 14px var(--font-sans);
    line-height: 1;
  }
  .add-btn:hover:not(:disabled) { background: var(--color-bg-inverse); color: var(--color-text-inverse); }
  .add-btn:disabled { opacity: 0.5; cursor: not-allowed; }

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

  .copy-box {
    display: flex; align-items: stretch;
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md);
    overflow: hidden; background: var(--color-bg-secondary);
    margin-bottom: 10px;
  }
  .copy-box code {
    flex: 1; padding: 10px 12px;
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-mono);
    color: var(--color-text-primary);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .copy-box button {
    border: 0; border-left: 1px solid var(--color-border);
    background: var(--color-bg-tertiary); color: var(--color-text-primary);
    padding: 0 14px; cursor: pointer;
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
  }
  .copy-box button:hover:not(:disabled) { background: var(--color-bg-inverse); color: var(--color-text-inverse); }
  .copy-box button:disabled { opacity: 0.5; cursor: not-allowed; }

  .primary {
    width: 100%; padding: 12px;
    background: var(--color-bg-inverse); color: var(--color-text-inverse);
    border: 0; border-radius: var(--radius-md);
    font: var(--font-weight-semibold) var(--font-size-sm) var(--font-sans);
    cursor: pointer;
  }
  .primary:hover:not(:disabled) { opacity: 0.92; }
  .primary:disabled { opacity: 0.5; cursor: not-allowed; }

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

  .toast {
    position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%);
    padding: 10px 18px;
    background: var(--color-bg-inverse); color: var(--color-text-inverse);
    border-radius: var(--radius-md);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    box-shadow: var(--shadow-lg, 0 4px 12px rgba(0, 0, 0, 0.2));
    z-index: 100;
  }
</style>
