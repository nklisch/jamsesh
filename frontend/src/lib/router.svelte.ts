// History-API router as a rune store. Routes are matched eagerly
// (first match wins). Programmatic navigation via `navigate()`.
//
// Every route declares `requiresAuth: boolean` (default: true). The auth gate
// in App.svelte reads this flag from `current.requiresAuth` instead of
// maintaining a separate hardcoded allowlist. Public routes (login, magic-link,
// oauth-callback) explicitly declare `requiresAuth: false`.

type Route = { pattern: RegExp; name: string; params: string[]; requiresAuth: boolean };

const routes: Route[] = [
  { pattern: /^\/$/,                                                   name: 'home',           params: [],                                    requiresAuth: true  },
  { pattern: /^\/login$/,                                              name: 'login',          params: [],                                    requiresAuth: false },
  { pattern: /^\/auth\/magic-link$/,                                   name: 'magic-link',     params: [],                                    requiresAuth: false },
  { pattern: /^\/auth\/oauth\/callback$/,                              name: 'oauth-callback', params: [],                                    requiresAuth: false },
  { pattern: /^\/orgs\/([^/]+)\/sessions$/,                            name: 'sessions',       params: ['orgId'],                             requiresAuth: true  },
  // `finalize` and `invite-accept` must come BEFORE `session-view` so the
  // more specific patterns win under first-match semantics.
  { pattern: /^\/orgs\/([^/]+)\/sessions\/([^/]+)\/finalize$/,                        name: 'finalize',      params: ['orgId', 'sessionId'],             requiresAuth: true  },
  { pattern: /^\/orgs\/([^/]+)\/sessions\/([^/]+)\/invites\/([^/]+)\/accept$/,        name: 'invite-accept', params: ['orgId', 'sessionId', 'inviteId'], requiresAuth: true  },
  { pattern: /^\/orgs\/([^/]+)\/sessions\/([^/]+)$/,                                  name: 'session-view',  params: ['orgId', 'sessionId'],             requiresAuth: true  },
  { pattern: /^\/orgs\/([^/]+)\/settings$/,                            name: 'org-settings',   params: ['orgId'],                             requiresAuth: true  },
];

function match(path: string): { name: string; params: Record<string, string>; requiresAuth: boolean } {
  for (const r of routes) {
    const m = r.pattern.exec(path);
    if (m) {
      const params: Record<string, string> = {};
      r.params.forEach((p, i) => { params[p] = decodeURIComponent(m[i + 1]); });
      return { name: r.name, params, requiresAuth: r.requiresAuth };
    }
  }
  // Unrecognised routes default to requiresAuth: true — unknown surfaces are
  // protected by default; explicit opt-out is required.
  return { name: 'not-found', params: {}, requiresAuth: true };
}

let path = $state(typeof window !== 'undefined' ? window.location.pathname : '/');
let _current = $derived(match(path));

// `current` is exposed as an object with a $derived getter so consumers can
// read `current.name`, `current.params`, and `current.requiresAuth` reactively.
// Exporting a plain `$derived` value is not permitted in Svelte 5 module context.
export const current = {
  get name() { return _current.name; },
  get params() { return _current.params; },
  get requiresAuth() { return _current.requiresAuth; },
};

export function navigate(to: string): void {
  if (typeof window === 'undefined') return;
  window.history.pushState({}, '', to);
  path = to;
}

if (typeof window !== 'undefined') {
  window.addEventListener('popstate', () => { path = window.location.pathname; });
}
