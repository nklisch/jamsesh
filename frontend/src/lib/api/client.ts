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
