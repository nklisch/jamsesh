import createClient, { type Middleware } from 'openapi-fetch';
import type { paths } from './types.gen';
import { auth } from '$lib/auth.svelte';

// Reserved org id for anonymous playground sessions. Mirrors
// playground.ReservedOrgID on the server (reserved-org-id-local-const-mirror).
const PLAYGROUND_ORG_ID = 'org_playground';

function playgroundSessionIdForPath(pathname: string): string | null {
  const publicSummaryPrefix = '/api/playground/sessions/';
  if (pathname.startsWith(publicSummaryPrefix)) {
    const rest = pathname.slice(publicSummaryPrefix.length);
    if (rest && !rest.includes('/')) return decodeURIComponent(rest);
  }

  const orgSessionPrefix = `/api/orgs/${PLAYGROUND_ORG_ID}/sessions/`;
  if (pathname.startsWith(orgSessionPrefix)) {
    const sessionId = pathname.slice(orgSessionPrefix.length).split('/')[0];
    if (sessionId) return decodeURIComponent(sessionId);
  }

  return null;
}

// bearerForRequest picks the token to attach. Anonymous playground participants
// hold a session-scoped bearer in auth.playgroundContext rather than an account
// access token, so playground requests for that exact session must use it. All
// other requests use the account token as before.
function bearerForRequest(pathname: string): string | null {
  const pg = auth.playgroundContext;
  const playgroundSessionId = playgroundSessionIdForPath(pathname);
  if (pg && playgroundSessionId === pg.sessionId) {
    return pg.bearer;
  }
  return auth.token;
}

const bearerMiddleware: Middleware = {
  onRequest({ request }) {
    const token = bearerForRequest(new URL(request.url).pathname);
    if (token) request.headers.set('Authorization', `Bearer ${token}`);
    return request;
  },
};

// unauthorizedMiddleware inspects 401 responses and routes auth-domain
// failures through auth.signOut(), which clears the persisted tokens and
// navigates to /login. Without this, a stale non-null token in localStorage
// makes auth.isAuthenticated return true and the auth guard does not redirect;
// protected views silently fail to load instead of bouncing the user to sign in.
//
// Only 401s whose typed error envelope has an `error` field starting with
// "auth." trigger signOut (e.g. "auth.invalid_token", "auth.token_expired").
// Non-auth-domain 401s (e.g. per-resource authorization failures from a stale
// per-org scope) are NOT treated as a global session failure; they surface to
// the calling screen with their typed error envelope intact.
//
// The response body is read on a clone so downstream openapi-fetch callers can
// still consume the original body (a Response body is a single-shot stream).
// Opaque 401s (non-JSON or unparseable body) are treated as fail-open: the
// 401 surfaces to the caller rather than triggering a global signOut.
//
// auth.signOut() is idempotent — multiple parallel auth-domain 401s simply
// re-run the no-op clear and re-navigate to /login.
const unauthorizedMiddleware: Middleware = {
  async onResponse({ request, response }) {
    if (response.status !== 401) return;
    const playgroundSessionId = playgroundSessionIdForPath(new URL(request.url).pathname);

    // Clone before reading the body — downstream openapi-fetch callers also
    // need to consume it (a Response body is a single-shot stream).
    let errorCode: string | undefined;
    try {
      const cloned = response.clone();
      const body = (await cloned.json()) as { error?: unknown } | null;
      if (body && typeof body.error === 'string') {
        errorCode = body.error;
      }
    } catch {
      // Body wasn't JSON or couldn't be parsed — treat as opaque 401.
      // Per the story spec, opaque 401s surface to the caller rather than
      // trigger a global signOut.
    }

    if (errorCode && errorCode.startsWith('auth.')) {
      if (playgroundSessionId) {
        auth.clearPlaygroundContext(playgroundSessionId);
      } else {
        auth.signOut();
      }
    }
    // Otherwise: surface to the caller. Don't signOut.
  },
};

// baseUrl is the same origin in production (Vite proxy handles /api/* in dev).
// In test environments (jsdom) window.location.origin is 'http://localhost:3000'.
const baseUrl = typeof window !== 'undefined' ? window.location.origin : '';

// Pass fetch as a late-binding wrapper so tests can replace globalThis.fetch
// via vi.spyOn / vi.stubGlobal without the reference being captured at module
// load time (openapi-fetch captures baseFetch = globalThis.fetch on
// createClient() call, so we must forward through globalThis here).
const lateFetch: typeof fetch = (...args) => globalThis.fetch(...args);

export const client = createClient<paths>({ baseUrl, fetch: lateFetch });
client.use(bearerMiddleware);
client.use(unauthorizedMiddleware);
