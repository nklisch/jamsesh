// WebSocket subscription manager, keyed by sessionId.
//
// Each call to subscribe() for a given sessionId reuses the same
// underlying WebSocket. Messages are routed to handlers filtered by
// event `type`. subscribe() returns an unsubscribe closure.
//
// Auth: the socket is opened with
//   Sec-WebSocket-Protocol: jamsesh.bearer.<token>
// per docs/research/core-go-server-stack.md (the portal WS gateway
// authenticates via sub-protocol header, not a cookie).
//
// EventEnvelope note:
// `EventEnvelope` is not yet in docs/openapi.yaml — no WS event
// schemas have landed. Until the discriminated union is generated,
// this module types the envelope as an open-ended object. When a
// future feature adds EventEnvelope to the spec and regenerates
// types.gen.ts, swap the local alias below for the import:
//
//   import type { components } from './api/types.gen';
//   type EventEnvelope = components['schemas']['EventEnvelope'];
//
// The `subscribe` signature uses a string `type` param directly so
// the API surface stays stable across that transition.

import { auth } from '$lib/auth.svelte';

// Open-ended until EventEnvelope lands in the spec.
type EventEnvelope = { type: string; [key: string]: unknown };
type Handler = (e: EventEnvelope) => void;

// Module-level maps; one socket and one handler registry per sessionId.
const sockets = new Map<string, WebSocket>();
const handlers = new Map<string, Map<string, Set<Handler>>>();

function open(sessionId: string): WebSocket {
  if (sockets.has(sessionId)) return sockets.get(sessionId)!;

  const token = auth.token;
  if (!token) throw new Error('ws: cannot open socket — no auth token');

  const proto = `jamsesh.bearer.${token}`;
  const ws = new WebSocket(`/ws/sessions/${sessionId}`, proto);

  ws.addEventListener('message', (ev: MessageEvent) => {
    let env: EventEnvelope;
    try {
      env = JSON.parse(ev.data as string) as EventEnvelope;
    } catch {
      return;
    }
    const byType = handlers.get(sessionId);
    const set = byType?.get(env.type);
    if (set) {
      for (const h of set) h(env);
    }
  });

  ws.addEventListener('close', () => {
    sockets.delete(sessionId);
  });

  sockets.set(sessionId, ws);
  return ws;
}

/**
 * Subscribe to events of a specific `type` for the given `sessionId`.
 *
 * Opens a new WebSocket if one is not already open for this session;
 * otherwise reuses the existing connection. Returns an unsubscribe
 * closure that removes only this handler.
 */
export function subscribe(
  sessionId: string,
  type: string,
  handler: Handler,
): () => void {
  open(sessionId);

  if (!handlers.has(sessionId)) handlers.set(sessionId, new Map());
  const byType = handlers.get(sessionId)!;
  if (!byType.has(type)) byType.set(type, new Set());
  byType.get(type)!.add(handler);

  return () => {
    byType.get(type)?.delete(handler);
  };
}

/**
 * Close the WebSocket for a session and remove all handlers.
 */
export function close(sessionId: string): void {
  sockets.get(sessionId)?.close();
  sockets.delete(sessionId);
  handlers.delete(sessionId);
}
