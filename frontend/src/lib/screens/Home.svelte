<script lang="ts">
  import { navigate } from '$lib/router.svelte';
  import { auth } from '$lib/auth.svelte';
  import { client } from '$lib/api/client';
  import Button from '$lib/components/Button.svelte';
  import Input from '$lib/components/Input.svelte';
  import Card from '$lib/components/Card.svelte';

  // 'creating' is the in-flight POST /api/orgs state; 'create-error' surfaces
  // a server failure. The list-load state machine is driven by auth.orgs:
  //   null      -> loading
  //   []        -> empty
  //   [single]  -> auto-route effect fires (no render)
  //   [a, b...] -> picker
  type CreateState = 'idle' | 'creating' | 'create-error';

  let newOrgName = $state('');
  let createState = $state<CreateState>('idle');
  let createError = $state<string | null>(null);

  // Single-org auto-route. Fires once when auth.orgs resolves to exactly one
  // entry. The _autoRouted latch prevents re-firing if the effect re-runs
  // (e.g. on reactive reads). Plain `let`, NOT `$state` — it's a one-shot
  // latch, not reactive state that consumers need to observe.
  let _autoRouted = false;
  $effect(() => {
    if (!_autoRouted && auth.orgs !== null && auth.orgs.length === 1) {
      _autoRouted = true;
      navigate(`/orgs/${auth.orgs[0].id}/sessions`);
    }
  });

  async function createOrg(e: Event) {
    e.preventDefault();
    const name = newOrgName.trim();
    if (name.length === 0 || createState === 'creating') return;
    createState = 'creating';
    createError = null;

    try {
      const { data, error } = await client.POST('/api/orgs', { body: { name } });
      if (data) {
        auth.addOrg({ id: data.id, name: data.name, slug: data.slug, role: 'creator' });
        navigate(`/orgs/${data.id}/sessions`);
        return;
      }
      createError = (error as { message?: string } | undefined)?.message ?? 'Could not create org';
      createState = 'create-error';
    } catch {
      createError = 'Could not reach the server. Try again.';
      createState = 'create-error';
    }
  }

  function roleLabel(role: string): string {
    return role.charAt(0).toUpperCase() + role.slice(1);
  }
</script>

<!-- topbar matches the mock; Login/OAuthCallback use a similar custom shape -->
<header class="topbar">
  <div class="wordmark">jam<span class="dot">·</span>sesh</div>
  <div class="user-strip">
    {#if auth.currentUser}<span class="email">{auth.currentUser.email}</span>{/if}
    <button class="signout-btn" type="button" onclick={() => auth.signOut()}>Sign out</button>
  </div>
</header>

<main class="page">
  <Card padding="lg">
    {#if auth.orgs === null}
      <p class="loading" aria-busy="true">Loading your workspaces</p>

    {:else if auth.orgs.length === 0}
      <h1>Welcome to jamsesh{auth.currentUser ? `, ${auth.currentUser.displayName}` : ''}</h1>
      <p class="lead">
        You're signed in but not in any orgs yet. Spin up your first org —
        you'll be its creator, and you can invite teammates from there.
      </p>
      {@render createForm()}

    {:else if auth.orgs.length >= 2}
      <!-- Picker: auto-route handles the length === 1 case via $effect -->
      <h1>Pick a workspace</h1>
      <p class="lead">You're in {auth.orgs.length} orgs. Click one to drop in, or create another below.</p>

      <ul class="org-list">
        {#each auth.orgs as org (org.id)}
          <li>
            <a
              class="org-row"
              href="/orgs/{org.id}/sessions"
              onclick={(e) => { e.preventDefault(); navigate(`/orgs/${org.id}/sessions`); }}
            >
              <span class="org-avatar">{org.name.charAt(0).toUpperCase()}</span>
              <span class="org-meta">
                <span class="org-name">{org.name}</span>
                <span class="org-slug">/orgs/{org.slug}/sessions</span>
              </span>
              <span class="role-badge role-{org.role}">{roleLabel(org.role)}</span>
            </a>
          </li>
        {/each}
      </ul>

      <div class="divider" aria-hidden="true">or</div>
      {@render createForm()}
    {/if}
  </Card>
</main>

{#snippet createForm()}
  <div class="create-block">
    <label class="create-label" for="new-org-name">
      {auth.orgs && auth.orgs.length === 0 ? 'Name your org' : 'Create another org'}
    </label>
    <form class="create-form" onsubmit={createOrg}>
      <Input bind:value={newOrgName} placeholder="e.g. acme" id="new-org-name" disabled={createState === 'creating'} />
      <Button variant="primary" type="submit" size="md" disabled={createState === 'creating'}>
        {#snippet children()}{createState === 'creating' ? 'Creating...' : 'Create org'}{/snippet}
      </Button>
    </form>
    <p class="form-hint">The slug is derived from the name. You become its creator.</p>
    {#if createState === 'create-error' && createError}
      <p class="form-error" role="alert">{createError}</p>
    {/if}
  </div>
{/snippet}

<style>
  .topbar {
    padding: 14px 24px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--color-border);
    background: var(--color-bg-secondary);
  }

  .wordmark {
    font-family: var(--font-sans);
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.03em;
  }

  .wordmark .dot {
    color: var(--color-accent);
  }

  .user-strip {
    display: flex;
    align-items: center;
    gap: 10px;
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
  }

  .email {
    font-family: var(--font-mono);
    font-size: 12px;
  }

  .signout-btn {
    background: transparent;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    padding: 5px 12px;
    color: var(--color-text-secondary);
    font-family: var(--font-sans);
    font-size: var(--font-size-sm);
    cursor: pointer;
    transition: background-color 120ms ease;
  }

  .signout-btn:hover {
    background: var(--color-bg-tertiary);
  }

  .page {
    max-width: 480px;
    margin: 0 auto;
    padding: 80px 24px 120px;
  }

  /* Card itself uses a slightly larger radius to match the mock */
  .page :global(.card) {
    border-radius: var(--radius-lg);
    padding: 32px 36px;
  }

  /* Loading state */
  .loading {
    margin: 0;
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    text-align: center;
    padding: var(--space-4) 0;
  }

  h1 {
    margin: 0 0 var(--space-2);
    font-size: var(--font-size-2xl);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.02em;
  }

  .lead {
    margin: 0 0 var(--space-5);
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    line-height: 1.6;
  }

  /* Org picker list */
  .org-list {
    list-style: none;
    margin: 0 0 var(--space-4);
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .org-row {
    display: grid;
    grid-template-columns: 32px 1fr auto;
    gap: 12px;
    align-items: center;
    padding: 12px 14px;
    background: var(--color-bg-primary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    cursor: pointer;
    text-decoration: none;
    color: inherit;
    transition:
      border-color 120ms ease,
      background-color 120ms ease;
  }

  .org-row:hover {
    border-color: var(--color-border-strong);
    background: var(--color-bg-tertiary);
  }

  .org-avatar {
    width: 32px;
    height: 32px;
    border-radius: var(--radius-md);
    background: var(--color-accent-muted);
    color: var(--color-accent);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-family: var(--font-mono);
    font-size: 13px;
    font-weight: var(--font-weight-semibold);
    flex-shrink: 0;
  }

  .org-meta {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .org-name {
    font-size: var(--font-size-base);
    font-weight: var(--font-weight-medium);
    color: var(--color-text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .org-slug {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--color-text-tertiary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .role-badge {
    padding: 3px 8px;
    background: var(--color-bg-tertiary);
    color: var(--color-text-secondary);
    border-radius: var(--radius-sm);
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: var(--font-weight-medium);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    white-space: nowrap;
  }

  /* Creator role uses accent-muted styling to signal ownership */
  .role-badge.role-creator {
    background: var(--color-accent-muted);
    color: var(--color-accent);
  }

  /* Divider between org list and create form */
  .divider {
    display: flex;
    align-items: center;
    gap: 12px;
    margin: var(--space-4) 0;
    color: var(--color-text-tertiary);
    font-family: var(--font-mono);
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.1em;
  }

  .divider::before,
  .divider::after {
    content: '';
    flex: 1;
    height: 1px;
    background: var(--color-border);
  }

  /* Create form block */
  .create-label {
    display: block;
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-medium);
    color: var(--color-text-secondary);
    margin-bottom: var(--space-2);
  }

  .create-form {
    display: flex;
    gap: var(--space-2);
    align-items: flex-end;
  }

  .create-form :global(.input) {
    flex: 1;
  }

  .form-hint {
    margin: var(--space-2) 0 0;
    font-size: var(--font-size-xs);
    color: var(--color-text-tertiary);
  }

  .form-error {
    margin: var(--space-2) 0 0;
    font-size: var(--font-size-sm);
    color: var(--color-text-danger, #e53e3e);
  }
</style>
