<script lang="ts">
  import { current, navigate } from '$lib/router.svelte';
  import { auth } from '$lib/auth.svelte';
  import { portalInfo } from '$lib/portalInfo.svelte';
  import Login from '$lib/screens/Login.svelte';
  import Home from '$lib/screens/Home.svelte';
  import ProjectLanding from '$lib/screens/ProjectLanding.svelte';
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
  import ResumeExchange from '$lib/screens/ResumeExchange.svelte';
  import NotFound from '$lib/screens/NotFound.svelte';

  // Bootstrap: kick off portalInfo.init() immediately alongside auth so that
  // the landing-variant is resolved before the auth-gate effect makes routing
  // decisions at `/` for anonymous visitors.
  void portalInfo.init();

  // Reserved org id for anonymous playground sessions. Mirrors
  // playground.ReservedOrgID on the server.
  const PLAYGROUND_ORG_ID = 'org_playground';

  // Auth gate: routes that declare `requiresAuth: true` (the default) require
  // an authenticated session. Routes that declare `requiresAuth: false` are
  // public and the gate leaves them alone.
  //
  // Invite-accept gets special treatment: we preserve the full invite URL as
  // `?return_to=<original>` so that after the user logs in they land back on
  // the invite page rather than the generic session list.  All other routes
  // continue to lose context on redirect (existing behavior).
  //
  // Anonymous `/` (home) routing waits for portalInfo.loaded so we don't
  // decide the landing-variant before the bootstrap fetch resolves.
  $effect(() => {
    // Authed user landed on /login (direct visit, back button, etc.) — bounce to home.
    // Skip oauth-callback (it does its own post-exchange navigation) and magic-link
    // (it MAY be hit while still unauthed to complete the exchange).
    if (auth.isAuthenticated && current.name === 'login') {
      navigate('/');
      return;
    }

    const isPlaygroundSessionView =
      current.name === 'session-view' &&
      current.params.orgId === PLAYGROUND_ORG_ID;

    if (
      isPlaygroundSessionView &&
      auth.playgroundContext?.sessionId !== current.params.sessionId
    ) {
      auth.restorePlaygroundContext(current.params.sessionId);
    }

    // Anonymous playground participants hold a session-scoped bearer in
    // auth.playgroundContext instead of an account token. The shared
    // session-view route is protected for durable sessions, but playground
    // sessions route through the browser-scoped join/resume context.
    const inOwnPlaygroundSession =
      isPlaygroundSessionView &&
      auth.playgroundContext?.sessionId === current.params.sessionId;

    if (isPlaygroundSessionView && !inOwnPlaygroundSession) {
      navigate(`/playground/s/${encodeURIComponent(current.params.sessionId)}/join`);
      return;
    }

    // Unauthed user on a protected route → /login, with landing-variant
    // branching for the home route. Gate on portalInfo.loaded so the variant
    // decision isn't made before the bootstrap fetch resolves.
    if (current.requiresAuth && !auth.isAuthenticated && !inOwnPlaygroundSession) {
      if (current.name === 'home') {
        // Wait until portalInfo has resolved its bootstrap fetch.
        if (!portalInfo.loaded) return;

        const v = portalInfo.landingVariant;
        if (v === 'project') {
          // Render ProjectLanding in-place at `/` — no navigation.
          return;
        }
        if (v === 'auto' && portalInfo.playgroundEnabled) {
          navigate('/playground');
          return;
        }
        // 'login', 'auto'+!playgroundEnabled, or any fallback.
        navigate('/login');
        return;
      }

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
{:else if current.name === 'home' && !auth.isAuthenticated && portalInfo.loaded && portalInfo.landingVariant === 'project'}
  <ProjectLanding />
{:else if current.name === 'home' && (auth.isAuthenticated || portalInfo.loaded)}
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
{:else if current.name === 'playground-resume' || current.name === 'session-resume'}
  <ResumeExchange />
{:else}
  <NotFound />
{/if}
