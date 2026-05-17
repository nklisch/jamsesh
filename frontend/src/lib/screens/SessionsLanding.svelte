<script lang="ts">
  import Chrome from '$lib/components/Chrome.svelte';
  import Button from '$lib/components/Button.svelte';
  import { auth } from '$lib/auth.svelte';
</script>

<Chrome orgChip="default-org">
  {#snippet children()}
    <h1>Sessions</h1>
    <p class="placeholder">
      No sessions yet.
      <!-- Real session listing lands in epic-portal-ui-session-list -->
    </p>
    <Button variant="ghost" onclick={() => auth.signOut()}>
      {#snippet children()}Sign out{/snippet}
    </Button>
  {/snippet}
</Chrome>

<style>
  h1 {
    margin: 0 0 var(--space-3);
    font-size: var(--font-size-2xl);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.02em;
  }

  .placeholder {
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    margin: 0 0 var(--space-5);
  }
</style>
