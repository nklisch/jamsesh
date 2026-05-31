// Auth rune store — token persistence and user state.
// Persists the short-lived access token to localStorage under jamsesh.token.
// Refresh tokens are kept memory-only for the current page lifetime so a
// reload cannot expose a long-lived credential from localStorage.
// Follows the wrapper-object pattern: Svelte 5 prohibits exporting
// a raw `$derived` value from a module; use a plain object with get
// accessors that close over the private rune variables instead.

import type { components } from '$lib/api/types.gen';
import { navigate } from '$lib/router.svelte';
import { client } from '$lib/api/client';

type MeOrgMembership = components['schemas']['MeOrgMembership'];

// PlaygroundContext is the anonymous-mode identity for a playground session.
// Intentionally separate from the authenticated-user identity (_currentUser /
// _orgs) — the two states are orthogonal and can coexist (e.g. a signed-in
// user clicks a playground share link).
export type PlaygroundContext = {
  sessionId: string;
  bearer: string;
  nickname: string;
};

const TOKEN_KEY = 'jamsesh.token';
const REFRESH_KEY = 'jamsesh.refresh';

if (typeof localStorage !== 'undefined') {
  localStorage.removeItem(REFRESH_KEY);
}

let _token = $state<string | null>(
  typeof localStorage !== 'undefined' ? localStorage.getItem(TOKEN_KEY) : null,
);
let _refresh = $state<string | null>(null);
let _currentUser = $state<{ id: string; email: string; displayName: string } | null>(null);
let _orgs = $state<MeOrgMembership[] | null>(null);

// _playgroundContext tracks the anonymous-mode bearer for a single playground
// session. null means the current view is not in playground/anonymous mode.
let _playgroundContext = $state<PlaygroundContext | null>(null);

// Guards a single in-flight /api/me call. Concurrent callers await the
// same promise; resolved-state callers return immediately.
let _loadingMe: Promise<void> | null = null;

export const auth = {
  get token(): string | null {
    return _token;
  },
  get refresh(): string | null {
    return _refresh;
  },
  get currentUser(): { id: string; email: string; displayName: string } | null {
    return _currentUser;
  },
  get orgs(): MeOrgMembership[] | null {
    return _orgs;
  },
  get isAuthenticated(): boolean {
    return _token !== null;
  },

  // playgroundContext — null when not in playground/anonymous mode; populated
  // when a joiner has exchanged a nickname for an anonymous bearer. Reading
  // this does NOT imply isAuthenticated is true; both states are independent.
  get playgroundContext(): PlaygroundContext | null {
    return _playgroundContext;
  },

  setPlaygroundContext(ctx: PlaygroundContext | null): void {
    _playgroundContext = ctx;
  },

  setTokens(access: string, refreshTok: string): void {
    _token = access;
    _refresh = refreshTok;
    localStorage.setItem(TOKEN_KEY, access);
    localStorage.removeItem(REFRESH_KEY);
  },

  // Adopts a durable browser session that carries NO refresh token (e.g. a
  // CLI resume-exchange bearer). Sets the access token, clears any stale
  // refresh, and resets cached user state so the next loadCurrentUser() runs
  // a fresh /api/me as the newly adopted account.
  setAccessOnly(access: string): void {
    _token = access;
    _refresh = null;
    _currentUser = null;
    _orgs = null;
    _loadingMe = null;
    localStorage.setItem(TOKEN_KEY, access);
    localStorage.removeItem(REFRESH_KEY);
  },

  async signOut(): Promise<void> {
    // Best-effort: tell the server to revoke all tokens for this account.
    // Capture the bearer FIRST so the server-side call can authenticate;
    // then clear local state SYNCHRONOUSLY so the UI updates immediately
    // and callers that don't await still see signed-out state. The endpoint
    // call itself completes asynchronously and never gates sign-out.
    // (feature-auth-signout-backend-revoke-frontend)
    //
    // The `if (capturedToken)` guard avoids a no-op POST when already
    // signed out. It also prevents recursion with unauthorizedMiddleware:
    // a 401 from the logout endpoint itself re-invokes signOut(), but by
    // then `_token` is already null and the recursive call captures an
    // empty bearer and skips the POST.
    const capturedToken = _token;
    _token = null;
    _refresh = null;
    _currentUser = null;
    _orgs = null;
    _loadingMe = null;
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(REFRESH_KEY);
    navigate('/login');

    if (capturedToken) {
      try {
        // bearerMiddleware reads from localStorage (now empty); pass the
        // captured token explicitly so the server can identify the account.
        await client.POST('/api/auth/logout', {
          headers: { Authorization: `Bearer ${capturedToken}` },
        });
      } catch {
        // Swallow — local state already cleared.
      }
    }
  },

  async loadCurrentUser(): Promise<void> {
    if (_currentUser !== null && _orgs !== null) return;
    if (_loadingMe !== null) return _loadingMe;

    // Capture the token at the start so we can discard the response if
    // signOut (or a sign-in as a different user) raced this call. Without
    // this guard, a stale response would repopulate _currentUser/_orgs
    // after they were cleared, and the next user would see the previous
    // user's data until reload — a cross-tenant leak on the client.
    const tokenAtStart = _token;
    _loadingMe = (async () => {
      try {
        const { data } = await client.GET('/api/me');
        if (data && _token !== null && _token === tokenAtStart) {
          _currentUser = {
            id: data.id,
            email: data.email,
            displayName: data.display_name,
          };
          _orgs = data.orgs;
        }
      } catch {
        // Leave state as-is; the App.svelte effect will retry on next
        // isAuthenticated flip if any.
      } finally {
        _loadingMe = null;
      }
    })();

    return _loadingMe;
  },

  // Append a freshly-created org to the local cache. Assigns a new array
  // (not push-in-place) so Svelte 5 $state reactivity fires.
  addOrg(org: MeOrgMembership): void {
    if (_orgs === null) _orgs = [org];
    else _orgs = [..._orgs, org];
  },
};
