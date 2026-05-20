// Auth rune store — token persistence and user state.
// Persists to localStorage under jamsesh.token / jamsesh.refresh.
// Follows the wrapper-object pattern: Svelte 5 prohibits exporting
// a raw `$derived` value from a module; use a plain object with get
// accessors that close over the private rune variables instead.

import type { components } from '$lib/api/types.gen';
import { navigate } from '$lib/router.svelte';
import { client } from '$lib/api/client';

type MeOrgMembership = components['schemas']['MeOrgMembership'];

const TOKEN_KEY = 'jamsesh.token';
const REFRESH_KEY = 'jamsesh.refresh';

let _token = $state<string | null>(
  typeof localStorage !== 'undefined' ? localStorage.getItem(TOKEN_KEY) : null,
);
let _refresh = $state<string | null>(
  typeof localStorage !== 'undefined' ? localStorage.getItem(REFRESH_KEY) : null,
);
let _currentUser = $state<{ id: string; email: string; displayName: string } | null>(null);
let _orgs = $state<MeOrgMembership[] | null>(null);

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

  setTokens(access: string, refreshTok: string): void {
    _token = access;
    _refresh = refreshTok;
    localStorage.setItem(TOKEN_KEY, access);
    localStorage.setItem(REFRESH_KEY, refreshTok);
  },

  signOut(): void {
    _token = null;
    _refresh = null;
    _currentUser = null;
    _orgs = null;
    _loadingMe = null;
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(REFRESH_KEY);
    navigate('/login');
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
