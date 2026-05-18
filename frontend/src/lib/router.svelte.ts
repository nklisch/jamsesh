// History-API router as a rune store. Routes are matched eagerly
// (first match wins). Programmatic navigation via `navigate()`.

type Route = { pattern: RegExp; name: string; params: string[] };

const routes: Route[] = [
  { pattern: /^\/login$/,                                              name: 'login',        params: [] },
  { pattern: /^\/auth\/magic-link$/,                                   name: 'magic-link',   params: [] },
  { pattern: /^\/orgs\/([^/]+)\/sessions$/,                            name: 'sessions',     params: ['orgId'] },
  // `finalize` and `invite-accept` must come BEFORE `session-view` so the
  // more specific patterns win under first-match semantics.
  { pattern: /^\/orgs\/([^/]+)\/sessions\/([^/]+)\/finalize$/,                         name: 'finalize',      params: ['orgId', 'sessionId'] },
  { pattern: /^\/orgs\/([^/]+)\/sessions\/([^/]+)\/invites\/([^/]+)\/accept$/,         name: 'invite-accept', params: ['orgId', 'sessionId', 'inviteId'] },
  { pattern: /^\/orgs\/([^/]+)\/sessions\/([^/]+)$/,                                   name: 'session-view',  params: ['orgId', 'sessionId'] },
  { pattern: /^\/orgs\/([^/]+)\/settings$/,                             name: 'org-settings', params: ['orgId'] },
];

function match(path: string): { name: string; params: Record<string, string> } {
  for (const r of routes) {
    const m = r.pattern.exec(path);
    if (m) {
      const params: Record<string, string> = {};
      r.params.forEach((p, i) => { params[p] = decodeURIComponent(m[i + 1]); });
      return { name: r.name, params };
    }
  }
  return { name: 'not-found', params: {} };
}

let path = $state(typeof window !== 'undefined' ? window.location.pathname : '/');
let _current = $derived(match(path));

// `current` is exposed as an object with a $derived getter so consumers can
// read `current.name` and `current.params` reactively. Exporting a plain
// `$derived` value is not permitted in Svelte 5 module context.
export const current = {
  get name() { return _current.name; },
  get params() { return _current.params; },
};

export function navigate(to: string): void {
  if (typeof window === 'undefined') return;
  window.history.pushState({}, '', to);
  path = to;
}

if (typeof window !== 'undefined') {
  window.addEventListener('popstate', () => { path = window.location.pathname; });
}
