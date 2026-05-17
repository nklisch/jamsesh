<script lang="ts">
  import { current, navigate } from '$lib/router.svelte';
  import { auth } from '$lib/auth.svelte';
  import Login from '$lib/screens/Login.svelte';
  import SessionList from '$lib/screens/SessionList.svelte';
  import SessionViewShell from '$lib/screens/SessionViewShell.svelte';
  import FinalizeView from '$lib/screens/FinalizeView.svelte';
  import NotFound from '$lib/screens/NotFound.svelte';

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
  <SessionList orgId={current.params.orgId} />
{:else if current.name === 'finalize'}
  <FinalizeView
    orgId={current.params.orgId}
    sessionId={current.params.sessionId}
  />
{:else if current.name === 'session-view'}
  <SessionViewShell
    orgId={current.params.orgId}
    sessionId={current.params.sessionId}
  />
{:else}
  <NotFound />
{/if}
