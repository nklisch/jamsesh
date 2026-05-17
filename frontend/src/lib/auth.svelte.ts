// Auth rune store — token persistence and user state.
// Persists to localStorage under jamsesh.token / jamsesh.refresh.
// Follows the wrapper-object pattern: Svelte 5 prohibits exporting
// a raw `$derived` value from a module; use a plain object with get
// accessors that close over the private rune variables instead.

import { navigate } from '$lib/router.svelte';
import { client } from '$lib/api/client';

const TOKEN_KEY = 'jamsesh.token';
const REFRESH_KEY = 'jamsesh.refresh';

let _token = $state<string | null>(
  typeof localStorage !== 'undefined' ? localStorage.getItem(TOKEN_KEY) : null,
);
let _refresh = $state<string | null>(
  typeof localStorage !== 'undefined' ? localStorage.getItem(REFRESH_KEY) : null,
);
let _currentUser = $state<{ id: string; email: string; displayName: string } | null>(null);

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
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(REFRESH_KEY);
    navigate('/login');
  },

  async loadCurrentUser(): Promise<void> {
    try {
      const { data } = await client.GET('/api/me');
      if (data) {
        _currentUser = {
          id: data.id,
          email: data.email,
          displayName: data.display_name,
        };
      }
    } catch {
      // Network/parse failure — leave _currentUser as-is.
      // The UI handles the null state.
    }
  },
};
