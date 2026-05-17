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

// unauthorizedMiddleware routes every 401 response through auth.signOut(),
// which clears the persisted tokens and navigates to /login. Without this,
// a stale non-null token in localStorage makes auth.isAuthenticated return
// true and the auth guard does not redirect; protected views silently fail
// to load instead of bouncing the user to sign in.
//
// auth.signOut() is idempotent — multiple parallel 401s simply re-run the
// no-op clear and re-navigate to /login.
const unauthorizedMiddleware: Middleware = {
  onResponse({ response }) {
    if (response.status === 401) {
      auth.signOut();
    }
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
