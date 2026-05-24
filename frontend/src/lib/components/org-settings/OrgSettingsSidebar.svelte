<script lang="ts">
  let { orgId }: { orgId: string } = $props();
</script>

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

<style>
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
</style>
