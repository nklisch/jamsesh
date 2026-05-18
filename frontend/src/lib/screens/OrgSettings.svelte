<script lang="ts">
  import { onMount } from 'svelte';
  import { client } from '$lib/api/client';
  import { auth } from '$lib/auth.svelte';
  import { navigate } from '$lib/router.svelte';
  import AuthorDot from '$lib/components/AuthorDot.svelte';
  import type { components } from '$lib/api/types.gen';

  type Org = components['schemas']['Org'];
  type OrgSessionInvitePolicy = components['schemas']['OrgSessionInvitePolicy'];

  let { orgId }: { orgId: string } = $props();

  // ── State ──────────────────────────────────────────────────────────────────
  let org = $state<Org | null>(null);
  let loadError = $state<string | null>(null);
  let isAdmin = $state(false);

  // Editing state
  let selectedPolicy = $state<OrgSessionInvitePolicy>('members_only');
  let savedPolicy = $state<OrgSessionInvitePolicy>('members_only');
  let isDirty = $derived(selectedPolicy !== savedPolicy);

  // Save state
  let saving = $state(false);
  let saveError = $state<string | null>(null);
  let saveSuccess = $state(false);
  let saveSuccessTimer: ReturnType<typeof setTimeout> | null = null;

  // ── Load ───────────────────────────────────────────────────────────────────
  async function loadOrg(): Promise<void> {
    const [orgResult, membersResult] = await Promise.all([
      client.GET('/api/orgs/{orgID}', { params: { path: { orgID: orgId } } }),
      client.GET('/api/orgs/{orgID}/members', { params: { path: { orgID: orgId } } }),
    ]);

    if (orgResult.error) {
      loadError = 'Failed to load org settings.';
      return;
    }
    if (membersResult.error) {
      loadError = 'Failed to load org members.';
      return;
    }

    org = orgResult.data;
    selectedPolicy = org.session_invite_policy;
    savedPolicy = org.session_invite_policy;

    const currentUserId = auth.currentUser?.id;
    const me = membersResult.data?.find((m) => m.account_id === currentUserId);
    isAdmin = me?.role === 'creator';
  }

  onMount(() => {
    void loadOrg();
  });

  // ── Save ───────────────────────────────────────────────────────────────────
  async function save(): Promise<void> {
    if (!isDirty || saving) return;
    saving = true;
    saveError = null;
    saveSuccess = false;

    const { data, error } = await client.PATCH('/api/orgs/{orgID}', {
      params: { path: { orgID: orgId } },
      body: { session_invite_policy: selectedPolicy },
    });

    saving = false;

    if (error) {
      // Narrow: check for 403 vs other errors
      const anyError = error as { error?: string; message?: string };
      if (anyError.error === 'auth.insufficient_permission') {
        saveError = 'Only org creators can change this setting.';
      } else {
        saveError = anyError.message ?? 'Failed to save. Please try again.';
      }
      return;
    }

    if (data) {
      org = data;
      savedPolicy = data.session_invite_policy;
      selectedPolicy = data.session_invite_policy;
    }

    saveSuccess = true;
    if (saveSuccessTimer !== null) clearTimeout(saveSuccessTimer);
    saveSuccessTimer = setTimeout(() => {
      saveSuccess = false;
    }, 2000);
  }

  function discard(): void {
    selectedPolicy = savedPolicy;
    saveError = null;
  }
</script>

<div class="settings-shell">
  <!-- App chrome -->
  <header class="app-chrome">
    <div class="wordmark" aria-label="jamsesh">jam<span class="dot">·</span>sesh</div>
    <nav class="breadcrumb" aria-label="Breadcrumb">
      <button class="breadcrumb-link" onclick={() => navigate(`/orgs/${orgId}/sessions`)}>
        {org?.name ?? orgId}
      </button>
      <span class="sep" aria-hidden="true">/</span>
      <span class="here">settings</span>
    </nav>
    <div class="chrome-spacer"></div>
    {#if auth.currentUser}
      <AuthorDot authorId={auth.currentUser.id} size={26} title={auth.currentUser.displayName} />
    {/if}
  </header>

  {#if loadError}
    <div class="load-error" role="alert">{loadError}</div>
  {:else if !org}
    <div class="loading-shell" aria-busy="true">Loading…</div>
  {:else}
    <div class="layout">
      <!-- Sidebar -->
      <aside class="sidebar">
        <div class="sidebar-head">Org settings</div>
        <nav class="nav" aria-label="Settings sections">
          <a href={`/orgs/${orgId}/settings`} class="active" aria-current="page">
            Session invites
          </a>
          <a href="#members" class="dim" onclick={(e) => e.preventDefault()} aria-disabled="true">
            Members <span class="soon">soon</span>
          </a>
          <a href="#billing" class="dim" onclick={(e) => e.preventDefault()} aria-disabled="true">
            Billing <span class="soon">soon</span>
          </a>
          <a href="#api-keys" class="dim" onclick={(e) => e.preventDefault()} aria-disabled="true">
            API keys <span class="soon">soon</span>
          </a>
        </nav>
      </aside>

      <!-- Content pane -->
      <main class="pane">
        <h1>Session invites</h1>
        <p class="pane-sub">
          Configure who can be added to sessions in <strong>{org.name}</strong>.
        </p>

        {#if !isAdmin}
          <div class="readonly-note" role="note">
            Only org creators can change this setting. You're viewing it as a member.
          </div>
        {/if}

        {#if saveError}
          <div class="save-error-banner" role="alert">{saveError}</div>
        {/if}

        <div class="field">
          <div class="field-label">Policy</div>
          <div class="field-desc">
            Sets who is allowed to accept a session invite. Affects future invite-accept
            attempts only; existing session members are grandfathered when the policy changes.
          </div>

          <label
            class="opt"
            class:selected={selectedPolicy === 'members_only'}
            class:disabled={!isAdmin}
          >
            <input
              type="radio"
              name="policy"
              value="members_only"
              bind:group={selectedPolicy}
              disabled={!isAdmin}
            />
            <div>
              <div class="opt-label">
                Members only
                <span class="opt-tag">recommended</span>
              </div>
              <div class="opt-desc">
                Invitee must already be an org member. Strictest; matches the multi-tenancy
                default.
              </div>
            </div>
          </label>

          <label
            class="opt"
            class:selected={selectedPolicy === 'open'}
            class:disabled={!isAdmin}
          >
            <input
              type="radio"
              name="policy"
              value="open"
              bind:group={selectedPolicy}
              disabled={!isAdmin}
            />
            <div>
              <div class="opt-label">Open</div>
              <div class="opt-desc">
                Anyone with a valid email invite can join as a session-only guest. They get
                access to that session only — not to other org resources.
              </div>
            </div>
          </label>
        </div>

        <div class="pane-actions">
          {#if saveSuccess}
            <span class="save-success" role="status">Saved</span>
          {/if}
          <button
            class="btn ghost"
            type="button"
            onclick={discard}
            disabled={!isDirty || !isAdmin || saving}
          >
            Discard
          </button>
          <button
            class="btn primary"
            type="button"
            onclick={save}
            disabled={!isDirty || !isAdmin || saving}
            aria-busy={saving}
          >
            {saving ? 'Saving…' : 'Save changes'}
          </button>
        </div>
      </main>
    </div>
  {/if}
</div>

<style>
  .settings-shell {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
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
    background: var(--color-bg-secondary);
    flex-shrink: 0;
  }

  .wordmark {
    font-weight: var(--font-weight-semibold);
    font-size: var(--font-size-lg);
    letter-spacing: -0.02em;
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

  /* ── Layout ─────────────────────────────────────────────────────── */
  .layout {
    display: grid;
    grid-template-columns: 240px 1fr;
    flex: 1;
    min-height: calc(100vh - 53px);
  }

  /* ── Sidebar ─────────────────────────────────────────────────────── */
  .sidebar {
    border-right: 1px solid var(--color-border);
    padding: 24px 0;
    background: var(--color-bg-secondary);
  }

  .sidebar-head {
    padding: 0 20px 12px;
    font-size: var(--font-size-xs);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--color-text-tertiary);
    font-weight: var(--font-weight-semibold);
  }

  .nav {
    display: flex;
    flex-direction: column;
  }

  .nav a {
    padding: 8px 20px;
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    text-decoration: none;
    border-left: 2px solid transparent;
    transition: background 0.15s, color 0.15s;
  }

  .nav a:hover:not(.dim) {
    background: var(--color-bg-tertiary);
    color: var(--color-text-primary);
  }

  .nav a.active {
    background: var(--color-accent-muted);
    color: var(--color-accent);
    border-left-color: var(--color-accent);
    font-weight: var(--font-weight-medium);
  }

  .nav a.dim {
    color: var(--color-text-tertiary);
    cursor: not-allowed;
  }

  .nav a.dim:hover {
    background: transparent;
    color: var(--color-text-tertiary);
  }

  .soon {
    font-size: var(--font-size-xs);
    color: var(--color-text-tertiary);
    margin-left: 6px;
    font-weight: 400;
  }

  /* ── Content pane ────────────────────────────────────────────────── */
  .pane {
    padding: 32px 40px;
    max-width: 720px;
  }

  .pane h1 {
    margin: 0 0 4px;
    font-size: var(--font-size-xl);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.02em;
  }

  .pane-sub {
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    margin-bottom: 28px;
    margin-top: 0;
    line-height: 1.5;
  }

  /* ── Banners ─────────────────────────────────────────────────────── */
  .readonly-note {
    margin-bottom: 16px;
    padding: 10px 14px;
    background: var(--color-warning-muted);
    border-left: 3px solid var(--color-warning);
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    border-radius: 0 var(--radius-md) var(--radius-md) 0;
  }

  .save-error-banner {
    margin-bottom: 16px;
    padding: 10px 14px;
    background: var(--color-warning-muted);
    border-left: 3px solid var(--color-warning);
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    border-radius: 0 var(--radius-md) var(--radius-md) 0;
  }

  /* ── Field ───────────────────────────────────────────────────────── */
  .field {
    margin-bottom: 20px;
  }

  .field-label {
    font-weight: var(--font-weight-medium);
    font-size: var(--font-size-base);
    margin-bottom: 4px;
  }

  .field-desc {
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    margin-bottom: 12px;
    line-height: 1.5;
  }

  /* ── Radio options ───────────────────────────────────────────────── */
  .opt {
    display: flex;
    gap: 12px;
    align-items: flex-start;
    padding: 12px 14px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-bg-secondary);
    cursor: pointer;
    transition: border-color 0.15s, background 0.15s;
    margin-bottom: 8px;
  }

  .opt:hover:not(.disabled) {
    border-color: var(--color-border-strong);
  }

  .opt.selected {
    border-color: var(--color-accent);
    background: var(--color-accent-muted);
  }

  .opt.disabled {
    cursor: not-allowed;
    opacity: 0.7;
  }

  .opt input[type='radio'] {
    margin-top: 2px;
    accent-color: var(--color-accent);
    flex-shrink: 0;
  }

  .opt-label {
    font-weight: var(--font-weight-medium);
    font-size: var(--font-size-base);
  }

  .opt-tag {
    font-size: var(--font-size-xs);
    color: var(--color-text-tertiary);
    font-weight: 400;
    margin-left: 6px;
  }

  .opt-desc {
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    line-height: 1.5;
    margin-top: 4px;
  }

  /* ── Pane actions ────────────────────────────────────────────────── */
  .pane-actions {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 8px;
    padding-top: 24px;
    border-top: 1px solid var(--color-border);
    margin-top: 32px;
  }

  .save-success {
    font-size: var(--font-size-sm);
    color: var(--color-success);
    font-weight: var(--font-weight-medium);
    margin-right: auto;
  }

  .btn {
    border-radius: var(--radius-md);
    padding: 6px 14px;
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-medium);
    border: 1px solid transparent;
    cursor: pointer;
    font-family: var(--font-sans);
  }

  .btn.primary {
    background: var(--color-accent);
    color: #fff;
    border-color: var(--color-accent);
  }

  .btn.primary:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .btn.ghost {
    background: transparent;
    color: var(--color-text-primary);
    border-color: var(--color-border-strong);
  }

  .btn.ghost:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
</style>
