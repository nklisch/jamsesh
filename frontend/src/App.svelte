<script lang="ts">
  import { current, navigate } from '$lib/router.svelte';
  import { auth } from '$lib/auth.svelte';
  import Login from '$lib/screens/Login.svelte';
  import MagicLinkExchange from '$lib/screens/MagicLinkExchange.svelte';
  import SessionList from '$lib/screens/SessionList.svelte';
  import SessionViewShell from '$lib/screens/SessionViewShell.svelte';
  import FinalizeView from '$lib/screens/FinalizeView.svelte';
  import OrgSettings from '$lib/screens/OrgSettings.svelte';
  import InviteAccept from '$lib/screens/InviteAccept.svelte';
  import NotFound from '$lib/screens/NotFound.svelte';

  // Auth gate: any non-login route requires authentication.
  // Runs reactively on every route change.
  //
  // Invite-accept gets special treatment: we preserve the full invite URL as
  // `?return_to=<original>` so that after the user logs in they land back on
  // the invite page rather than the generic session list.  All other routes
  // continue to lose context on redirect (existing behavior).
  //
  // magic-link is excluded from the auth gate — it is the unauthenticated
  // landing page for magic-link token exchange.
  $effect(() => {
    if (current.name !== 'login' && current.name !== 'magic-link' && !auth.isAuthenticated) {
      if (current.name === 'invite-accept') {
        const returnTo = window.location.pathname + window.location.search;
        navigate('/login?return_to=' + encodeURIComponent(returnTo));
      } else {
        navigate('/login');
      }
    }
  });
</script>

{#if current.name === 'login'}
  <Login />
{:else if current.name === 'magic-link'}
  <MagicLinkExchange />
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
{:else if current.name === 'org-settings'}
  <OrgSettings orgId={current.params.orgId} />
{:else if current.name === 'invite-accept'}
  <InviteAccept
    orgId={current.params.orgId}
    sessionId={current.params.sessionId}
    inviteId={current.params.inviteId}
  />
{:else}
  <NotFound />
{/if}
