import createClient, { type Middleware } from 'openapi-fetch';
import type { paths } from './types.gen';
import { auth } from '$lib/auth.svelte';

const bearerMiddleware: Middleware = {
  onRequest({ request }) {
    const token = auth.token;
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
  async onResponse({ response }) {
    if (response.status !== 401) return;

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
      auth.signOut();
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
