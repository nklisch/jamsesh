<script lang="ts">
  import { onMount } from 'svelte';
  import { client } from '$lib/api/client';
  import { auth } from '$lib/auth.svelte';
  import { navigate } from '$lib/router.svelte';
  import AuthorDot from '$lib/components/AuthorDot.svelte';
  import OrgInvitePolicyEditor from '$lib/components/org-settings/OrgInvitePolicyEditor.svelte';
  import OrgSettingsSidebar from '$lib/components/org-settings/OrgSettingsSidebar.svelte';
  import type { components } from '$lib/api/types.gen';

  type Org = components['schemas']['Org'];

  let { orgId }: { orgId: string } = $props();

  // ── State ──────────────────────────────────────────────────────────────────
  let org = $state<Org | null>(null);
  let loadError = $state<string | null>(null);
  let isAdmin = $state(false);

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

    const currentUserId = auth.currentUser?.id;
    const me = membersResult.data?.find((m) => m.account_id === currentUserId);
    isAdmin = me?.role === 'creator';
  }

  onMount(() => {
    void loadOrg();
  });
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
      <OrgSettingsSidebar {orgId} />
      <OrgInvitePolicyEditor
        {orgId}
        {org}
        {isAdmin}
        onorgchanged={(updated) => { org = updated; }}
      />
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
</style>
