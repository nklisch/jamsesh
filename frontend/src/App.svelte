<script lang="ts">
  import { current, navigate } from '$lib/router.svelte';
  import { auth } from '$lib/auth.svelte';
  import Login from '$lib/screens/Login.svelte';
  import SessionsLanding from '$lib/screens/SessionsLanding.svelte';
  import NotFound from '$lib/screens/NotFound.svelte';
  import Chrome from '$lib/components/Chrome.svelte';

  // Auth gate: any non-login route requires authentication.
  // Runs reactively on every route change.
  $effect(() => {
    if (current.name !== 'login' && !auth.isAuthenticated) {
      navigate('/login');
    }
  });
</script>

{#if current.name === 'login'}
  <Login />
{:else if current.name === 'sessions'}
  <SessionsLanding />
{:else if current.name === 'session-view'}
  <!-- Placeholder: full session-view shell lands in epic-portal-ui-session-view-shell -->
  <Chrome
    orgChip={current.params.orgId}
    sessionChip={current.params.sessionId}
  >
    {#snippet children()}
      <p>Session view placeholder — landing in epic-portal-ui-session-view-shell.</p>
    {/snippet}
  </Chrome>
{:else}
  <NotFound />
{/if}
