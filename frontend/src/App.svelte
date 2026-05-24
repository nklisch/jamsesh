<script lang="ts">
  import { current, navigate } from '$lib/router.svelte';
  import { auth } from '$lib/auth.svelte';
  import Login from '$lib/screens/Login.svelte';
  import Home from '$lib/screens/Home.svelte';
  import MagicLinkExchange from '$lib/screens/MagicLinkExchange.svelte';
  import OAuthCallback from '$lib/screens/OAuthCallback.svelte';
  import SessionList from '$lib/screens/SessionList.svelte';
  import SessionViewShell from '$lib/screens/SessionViewShell.svelte';
  import FinalizeView from '$lib/screens/FinalizeView.svelte';
  import OrgSettings from '$lib/screens/OrgSettings.svelte';
  import InviteAccept from '$lib/screens/InviteAccept.svelte';
  import PlaygroundLanding from '$lib/screens/PlaygroundLanding.svelte';
  import JoinerPicker from '$lib/screens/JoinerPicker.svelte';
  import SessionTombstone from '$lib/screens/SessionTombstone.svelte';
  import NotFound from '$lib/screens/NotFound.svelte';

  // Auth gate: routes that declare `requiresAuth: true` (the default) require
  // an authenticated session. Routes that declare `requiresAuth: false` are
  // public and the gate leaves them alone.
  //
  // Invite-accept gets special treatment: we preserve the full invite URL as
  // `?return_to=<original>` so that after the user logs in they land back on
  // the invite page rather than the generic session list.  All other routes
  // continue to lose context on redirect (existing behavior).
  $effect(() => {
    // Authed user landed on /login (direct visit, back button, etc.) — bounce to home.
    // Skip oauth-callback (it does its own post-exchange navigation) and magic-link
    // (it MAY be hit while still unauthed to complete the exchange).
    if (auth.isAuthenticated && current.name === 'login') {
      navigate('/');
      return;
    }

    // Unauthed user on a protected route → /login.
    // Whether a route is protected is declared on the route itself via
    // `requiresAuth: true` (the default). The old hardcoded name-based
    // allowlist is gone; the route registry is the single source of truth.
    if (current.requiresAuth && !auth.isAuthenticated) {
      if (current.name === 'invite-accept') {
        const returnTo = window.location.pathname + window.location.search;
        navigate('/login?return_to=' + encodeURIComponent(returnTo));
      } else {
        navigate('/login');
      }
    }
  });

  // Bootstrap: when the user is authenticated but we have not yet loaded
  // /api/me, fetch it. Covers cold-load with persisted tokens.
  // OAuthCallback awaits loadCurrentUser() explicitly before navigating,
  // so this effect is a no-op there (guarded inside auth.loadCurrentUser
  // via the _currentUser/_orgs check).
  $effect(() => {
    if (auth.isAuthenticated && auth.orgs === null) {
      void auth.loadCurrentUser();
    }
  });
</script>

{#if current.name === 'login'}
  <Login />
{:else if current.name === 'home'}
  <Home />
{:else if current.name === 'magic-link'}
  <MagicLinkExchange />
{:else if current.name === 'oauth-callback'}
  <OAuthCallback />
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
{:else if current.name === 'playground'}
  <PlaygroundLanding />
{:else if current.name === 'playground-join'}
  <JoinerPicker sessionId={current.params.sessionId} />
{:else if current.name === 'playground-ended'}
  <SessionTombstone sessionId={current.params.sessionId} />
{:else}
  <NotFound />
{/if}
