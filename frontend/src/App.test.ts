// App.test.ts
// Covers two $effect blocks in App.svelte:
//   1. Auth-gate effect  — redirects based on auth state, route name, and
//      landing-variant from portalInfo.
//   2. Bootstrap effect  — calls auth.loadCurrentUser() exactly once on cold-load.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, waitFor, cleanup } from '@testing-library/svelte';
import App from './App.svelte';

// ── Screen-component stubs ───────────────────────────────────────────────────
// App.svelte imports nine screen components. Each one gets the lightest
// possible stub: a Svelte 5 component is a function called as
// `Component(anchor_node, props)` by svelte's mount() internals.
// A no-op function that returns {} satisfies the contract and keeps
// render() calls cheap and test output noise-free.

/* eslint-disable @typescript-eslint/no-unused-vars */
// Home and ProjectLanding stubs are spy-tracked so tests can observe which
// home-branch the template mounts. The spies survive vi.mock hoisting via the
// `(...args) => mockX(...args)` indirection (spa-test-module-mock-barrel
// pattern).
const mockHomeStub = vi.fn().mockReturnValue({});
const mockProjectLandingStub = vi.fn().mockReturnValue({});

vi.mock('$lib/screens/Login.svelte', () => ({
  default: function LoginStub(_anchor: unknown, _props: unknown) { return {}; },
}));
vi.mock('$lib/screens/Home.svelte', () => ({
  default: (...args: unknown[]) => mockHomeStub(...args),
}));
vi.mock('$lib/screens/MagicLinkExchange.svelte', () => ({
  default: function MagicLinkExchangeStub(_anchor: unknown, _props: unknown) { return {}; },
}));
vi.mock('$lib/screens/OAuthCallback.svelte', () => ({
  default: function OAuthCallbackStub(_anchor: unknown, _props: unknown) { return {}; },
}));
vi.mock('$lib/screens/SessionList.svelte', () => ({
  default: function SessionListStub(_anchor: unknown, _props: unknown) { return {}; },
}));
vi.mock('$lib/screens/SessionViewShell.svelte', () => ({
  default: function SessionViewShellStub(_anchor: unknown, _props: unknown) { return {}; },
}));
vi.mock('$lib/screens/FinalizeView.svelte', () => ({
  default: function FinalizeViewStub(_anchor: unknown, _props: unknown) { return {}; },
}));
vi.mock('$lib/screens/OrgSettings.svelte', () => ({
  default: function OrgSettingsStub(_anchor: unknown, _props: unknown) { return {}; },
}));
vi.mock('$lib/screens/InviteAccept.svelte', () => ({
  default: function InviteAcceptStub(_anchor: unknown, _props: unknown) { return {}; },
}));
vi.mock('$lib/screens/NotFound.svelte', () => ({
  default: function NotFoundStub(_anchor: unknown, _props: unknown) { return {}; },
}));
vi.mock('$lib/screens/ProjectLanding.svelte', () => ({
  default: (...args: unknown[]) => mockProjectLandingStub(...args),
}));

// ── Router mock ──────────────────────────────────────────────────────────────
// `current` is a mutable object so tests can set .name before render().
// `navigate` is a plain spy.

const mockNavigate = vi.fn();
const mockRouterCurrent = {
  name: 'home',
  params: {} as Record<string, string>,
  requiresAuth: true,
};

vi.mock('$lib/router.svelte', () => ({
  navigate: (...args: unknown[]) => mockNavigate(...args),
  get current() {
    return mockRouterCurrent;
  },
}));

// ── Auth mock ────────────────────────────────────────────────────────────────
// Mirrors the real wrapper-object shape from auth.svelte.ts.
// Tests mutate mockAuth.isAuthenticated and mockAuth.orgs directly.

const mockLoadCurrentUser = vi.fn().mockResolvedValue(undefined);
const mockAuth = {
  isAuthenticated: false as boolean,
  orgs: null as unknown[] | null,
};

vi.mock('$lib/auth.svelte', () => ({
  get auth() {
    return {
      get isAuthenticated() { return mockAuth.isAuthenticated; },
      get orgs() { return mockAuth.orgs; },
      loadCurrentUser: () => mockLoadCurrentUser(),
    };
  },
}));

// ── portalInfo mock ──────────────────────────────────────────────────────────
// Mirrors the wrapper-object shape from portalInfo.svelte.ts.
// Tests mutate mockPortalInfo fields directly; init() is a no-op.

const mockPortalInfoInit = vi.fn().mockResolvedValue(undefined);
const mockPortalInfo = {
  loaded: true as boolean,
  playgroundEnabled: false as boolean,
  landingVariant: 'login' as 'auto' | 'project' | 'login',
};

vi.mock('$lib/portalInfo.svelte', () => ({
  get portalInfo() {
    return {
      get loaded() { return mockPortalInfo.loaded; },
      get playgroundEnabled() { return mockPortalInfo.playgroundEnabled; },
      get landingVariant() { return mockPortalInfo.landingVariant; },
      init: () => mockPortalInfoInit(),
    };
  },
}));

// ── Location helper ───────────────────────────────────────────────────────────

function setLocation(pathname: string, search: string = '') {
  Object.defineProperty(window, 'location', {
    value: { ...window.location, pathname, search },
    writable: true,
    configurable: true,
  });
}

// ── Test suites ───────────────────────────────────────────────────────────────

describe('App — auth-gate $effect', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Safe defaults: unauthed user on the home route (protected).
    mockAuth.isAuthenticated = false;
    mockAuth.orgs = null;
    mockRouterCurrent.name = 'home';
    mockRouterCurrent.params = {};
    mockRouterCurrent.requiresAuth = true;
    // portalInfo defaults: loaded, login variant (safe default).
    mockPortalInfo.loaded = true;
    mockPortalInfo.playgroundEnabled = false;
    mockPortalInfo.landingVariant = 'login';
    setLocation('/');
  });

  afterEach(() => {
    cleanup();
  });

  it('redirects an authed user who lands on /login back to /', async () => {
    // Defense-in-depth: App.svelte's own gate bounces the authed user away
    // from the login screen independently of Login.svelte's effect.
    mockAuth.isAuthenticated = true;
    mockRouterCurrent.name = 'login';
    mockRouterCurrent.requiresAuth = false; // /login is a public route

    render(App);

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/'));
  });

  it('redirects an unauthed user on a protected route to /login', async () => {
    mockAuth.isAuthenticated = false;
    mockRouterCurrent.name = 'sessions';
    mockRouterCurrent.params = { orgId: 'org-1' };
    mockRouterCurrent.requiresAuth = true; // sessions requires auth

    render(App);

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/login'));
  });

  it('preserves ?return_to=<original> when redirecting an unauthed invite-accept visitor', async () => {
    // The invite-accept route gets special treatment so the user lands back on
    // the invite URL after logging in rather than the generic session list.
    mockAuth.isAuthenticated = false;
    mockRouterCurrent.name = 'invite-accept';
    mockRouterCurrent.params = { orgId: 'org-1', sessionId: 'sess-2', inviteId: 'inv-3' };
    mockRouterCurrent.requiresAuth = true; // invite-accept requires auth

    const invitePath = '/orgs/org-1/sessions/sess-2/invites/inv-3/accept';
    setLocation(invitePath, '');

    render(App);

    const expectedReturn = encodeURIComponent(invitePath);
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith(`/login?return_to=${expectedReturn}`),
    );
  });

  it('does NOT redirect an unauthed user on the login route', async () => {
    // /login declares requiresAuth: false — an unauthed visit must not
    // bounce to /login again (infinite redirect loop).
    mockAuth.isAuthenticated = false;
    mockRouterCurrent.name = 'login';
    mockRouterCurrent.requiresAuth = false; // public route

    render(App);

    // Give effects a chance to run.
    await new Promise((r) => setTimeout(r, 50));
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('does NOT redirect an unauthed user arriving via the magic-link exchange route', async () => {
    // magic-link declares requiresAuth: false — the gate must leave it alone
    // or the unauthenticated token-exchange flow can never complete.
    mockAuth.isAuthenticated = false;
    mockRouterCurrent.name = 'magic-link';
    mockRouterCurrent.requiresAuth = false; // public route

    render(App);

    await new Promise((r) => setTimeout(r, 50));
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('does NOT redirect an unauthed user on the oauth-callback route', async () => {
    // oauth-callback declares requiresAuth: false — it does its own post-
    // exchange navigation; App.svelte must stay out of its way.
    mockAuth.isAuthenticated = false;
    mockRouterCurrent.name = 'oauth-callback';
    mockRouterCurrent.requiresAuth = false; // public route

    render(App);

    await new Promise((r) => setTimeout(r, 50));
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  // ── Anonymous `/` landing-variant routing ─────────────────────────────────
  // Covers the 6 combinations from the Unit 2 acceptance criteria.

  it('unauthed + landingVariant=project → does NOT navigate (renders ProjectLanding in-place)', async () => {
    mockAuth.isAuthenticated = false;
    mockRouterCurrent.name = 'home';
    mockRouterCurrent.requiresAuth = true;
    mockPortalInfo.loaded = true;
    mockPortalInfo.landingVariant = 'project';
    mockPortalInfo.playgroundEnabled = false;

    render(App);

    await new Promise((r) => setTimeout(r, 50));
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('unauthed + landingVariant=auto + playgroundEnabled=true → navigates to /playground', async () => {
    mockAuth.isAuthenticated = false;
    mockRouterCurrent.name = 'home';
    mockRouterCurrent.requiresAuth = true;
    mockPortalInfo.loaded = true;
    mockPortalInfo.landingVariant = 'auto';
    mockPortalInfo.playgroundEnabled = true;

    render(App);

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/playground'));
  });

  it('unauthed + landingVariant=auto + playgroundEnabled=false → navigates to /login', async () => {
    mockAuth.isAuthenticated = false;
    mockRouterCurrent.name = 'home';
    mockRouterCurrent.requiresAuth = true;
    mockPortalInfo.loaded = true;
    mockPortalInfo.landingVariant = 'auto';
    mockPortalInfo.playgroundEnabled = false;

    render(App);

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/login'));
  });

  it('unauthed + landingVariant=login → navigates to /login', async () => {
    mockAuth.isAuthenticated = false;
    mockRouterCurrent.name = 'home';
    mockRouterCurrent.requiresAuth = true;
    mockPortalInfo.loaded = true;
    mockPortalInfo.landingVariant = 'login';
    mockPortalInfo.playgroundEnabled = false;

    render(App);

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/login'));
  });

  it('authed + landingVariant=project → does NOT navigate (renders Home, flag has no effect on authed)', async () => {
    mockAuth.isAuthenticated = true;
    mockAuth.orgs = [{ id: 'org-1', name: 'Acme', slug: 'acme', role: 'creator' }];
    mockRouterCurrent.name = 'home';
    mockRouterCurrent.requiresAuth = true;
    mockPortalInfo.loaded = true;
    mockPortalInfo.landingVariant = 'project';
    mockPortalInfo.playgroundEnabled = false;

    render(App);

    await new Promise((r) => setTimeout(r, 50));
    // Should NOT navigate — authed users at / always see Home.svelte.
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('portalInfo not yet loaded → does not navigate until loaded (gate holds)', async () => {
    mockAuth.isAuthenticated = false;
    mockRouterCurrent.name = 'home';
    mockRouterCurrent.requiresAuth = true;
    // Simulate the fetch still in flight.
    mockPortalInfo.loaded = false;
    mockPortalInfo.landingVariant = 'login';

    render(App);

    await new Promise((r) => setTimeout(r, 50));
    // Gate must hold while portalInfo is not yet loaded.
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('unauthed + portalInfo.loaded=false → neither Home nor ProjectLanding mounts (flash gate)', async () => {
    mockAuth.isAuthenticated = false;
    mockRouterCurrent.name = 'home';
    mockRouterCurrent.requiresAuth = true;
    mockPortalInfo.loaded = false;
    mockPortalInfo.landingVariant = 'project';

    render(App);

    await new Promise((r) => setTimeout(r, 50));
    // Gate must hold: neither home-branch screen should mount while portalInfo
    // is still loading. The auth-gate $effect's navigation guard is asserted
    // separately above — this asserts the template's mounting behavior.
    expect(mockHomeStub).not.toHaveBeenCalled();
    expect(mockProjectLandingStub).not.toHaveBeenCalled();
  });
});

describe('App — bootstrap $effect', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Start on home (protected) so the auth-gate does not fire and confound counts.
    mockRouterCurrent.name = 'home';
    mockRouterCurrent.params = {};
    mockRouterCurrent.requiresAuth = true;
    // portalInfo defaults: loaded, project variant so gate does not navigate.
    mockPortalInfo.loaded = true;
    mockPortalInfo.landingVariant = 'project';
    mockPortalInfo.playgroundEnabled = false;
    setLocation('/');
  });

  afterEach(() => {
    cleanup();
  });

  it('cold-load: calls auth.loadCurrentUser() exactly once when authed and orgs is null', async () => {
    // This is the primary acceptance criterion: the bootstrap effect must
    // initiate a single /api/me fetch on a fresh page load where the user
    // has a persisted token but user data has not been hydrated yet.
    mockAuth.isAuthenticated = true;
    mockAuth.orgs = null;

    render(App);

    await waitFor(() => expect(mockLoadCurrentUser).toHaveBeenCalledTimes(1));
  });

  it('does NOT call loadCurrentUser when authed and orgs are already loaded', async () => {
    // Idempotency guard from App.svelte's perspective: if the store already
    // holds org data (e.g., warm re-render or OAuthCallback pre-loaded it),
    // the effect must not issue a redundant /api/me request.
    mockAuth.isAuthenticated = true;
    mockAuth.orgs = [{ id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' }];

    render(App);

    // Allow a tick for effects to settle.
    await new Promise((r) => setTimeout(r, 50));
    expect(mockLoadCurrentUser).not.toHaveBeenCalled();
  });

  it('does NOT call loadCurrentUser when the user is not authenticated', async () => {
    // No token — cold-load of the login page — bootstrap must stay silent.
    mockAuth.isAuthenticated = false;
    mockAuth.orgs = null;

    render(App);

    await new Promise((r) => setTimeout(r, 50));
    expect(mockLoadCurrentUser).not.toHaveBeenCalled();
  });
});
