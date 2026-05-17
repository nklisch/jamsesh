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
// Reconnect: on an unexpected close (see shouldReconnect() below), the
// module schedules a reopen with exponential backoff (1.0s base, 30s
// cap, 1.6× multiplier, ±25% jitter). On a clean / fatal close — or
// when close(sessionId) is called explicitly — the record is dropped
// and the loop stops. The per-session status is surfaced reactively
// via the `wsStatus` rune store.
//
// Write invariant: nothing else writes to the socket today — the
// client doesn't issue commands over WS. When the replay-from cursor
// lands (sibling story), the reconnect 'open' listener will send a
// `{"replay_from": <seq>}` frame first, BEFORE any other writes,
// because the portal accepts that frame only as the first text frame
// after upgrade (gateway.go:215). This module's reconnect path
// already runs synchronously inside the 'open' listener so the
// ordering invariant holds when that story merges.
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

export type WsStatus = 'connecting' | 'open' | 'reconnecting';

// Backoff constants. Sequence with factor=1.0 (no jitter):
//   1.0s, 1.6s, 2.56s, 4.10s, 6.55s, 10.49s, 16.78s, 26.84s, 30.0s …
const RECONNECT_BASE_MS = 1000;
const RECONNECT_CAP_MS = 30_000;
const RECONNECT_MULT = 1.6;
const RECONNECT_JITTER = 0.25; // ±25%

interface ConnectionRecord {
  ws: WebSocket | null;
  status: WsStatus;
  attempt: number; // 0 when open, increments per backoff tick
  closedByUs: boolean;
  reconnectTimer: ReturnType<typeof setTimeout> | null;
}

// Module-level maps; one record and one handler registry per sessionId.
const records = new Map<string, ConnectionRecord>();
const handlers = new Map<string, Map<string, Set<Handler>>>();

// Reactive per-session status. Wrapper-object pattern (mirrors
// auth.svelte.ts) so consumers get reactivity without us exporting the
// raw $state expression.
const _statuses = $state<Record<string, WsStatus | null>>({});

export const wsStatus = {
  for(sessionId: string): WsStatus | null {
    return _statuses[sessionId] ?? null;
  },
};

function setStatus(sessionId: string, s: WsStatus | null): void {
  if (s === null) {
    delete _statuses[sessionId];
  } else {
    _statuses[sessionId] = s;
  }
}

/**
 * Predicate: should the module attempt to reconnect after a close
 * event with this code?
 *
 * Returns false for:
 *  - 1000 Normal Closure (we asked for it, or server shut us down cleanly)
 *  - 1003 Unsupported Data (programmer error — reconnect can't fix)
 *  - 1007 Invalid Frame Payload (programmer error)
 *  - 1008 Policy Violation (portal kicked us as slow-consumer — reconnect
 *    would just re-fill the buffer and re-trigger the kick)
 *  - any 4xxx app-level code (auth failure etc. — the auth layer handles it)
 *
 * Returns true for everything else, including 1001 (Going Away), 1006
 * (Abnormal Closure — the common network-drop case), 1011 (Internal
 * Error), 1012/1013/1014 (Service Restart / Try Again Later / Bad
 * Gateway), and the no-code-supplied path (browsers normalise to 1006).
 */
export function shouldReconnect(code: number): boolean {
  if (code === 1000 || code === 1003 || code === 1007 || code === 1008) {
    return false;
  }
  if (code >= 4000 && code < 5000) {
    return false;
  }
  return true;
}

/**
 * Compute the next backoff delay (ms) for a given attempt index.
 *
 * delay = min(BASE * MULT^attempt, CAP) * jitterFactor
 * where jitterFactor is uniform in [1 - JITTER, 1 + JITTER].
 *
 * Tests pin Math.random to 0.5 → jitterFactor = 1.0, so delays are
 * deterministic.
 */
function backoffDelay(attempt: number): number {
  const base = Math.min(RECONNECT_BASE_MS * Math.pow(RECONNECT_MULT, attempt), RECONNECT_CAP_MS);
  const jitterFactor = 1 - RECONNECT_JITTER + Math.random() * (2 * RECONNECT_JITTER);
  return base * jitterFactor;
}

function attachListeners(sessionId: string, ws: WebSocket): void {
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

  ws.addEventListener('open', () => {
    const rec = records.get(sessionId);
    if (!rec) return;
    rec.status = 'open';
    rec.attempt = 0;
    setStatus(sessionId, 'open');
  });

  ws.addEventListener('close', (ev: CloseEvent) => {
    const rec = records.get(sessionId);
    if (!rec) return;
    rec.ws = null;

    if (rec.closedByUs || !shouldReconnect(ev.code)) {
      records.delete(sessionId);
      setStatus(sessionId, null);
      return;
    }

    rec.status = 'reconnecting';
    setStatus(sessionId, 'reconnecting');
    const delay = backoffDelay(rec.attempt);
    rec.reconnectTimer = setTimeout(() => {
      rec.reconnectTimer = null;
      rec.attempt += 1;
      reopen(sessionId);
    }, delay);
  });
}

function reopen(sessionId: string): void {
  const rec = records.get(sessionId);
  if (!rec) return;

  const token = auth.token;
  if (!token) {
    // No token to reconnect with — give up and drop the record.
    records.delete(sessionId);
    setStatus(sessionId, null);
    return;
  }

  const proto = `jamsesh.bearer.${token}`;
  const ws = new WebSocket(`/ws/sessions/${sessionId}`, proto);
  rec.ws = ws;
  attachListeners(sessionId, ws);
}

function open(sessionId: string): WebSocket {
  const existing = records.get(sessionId);
  if (existing && existing.ws) return existing.ws;

  const token = auth.token;
  if (!token) throw new Error('ws: cannot open socket — no auth token');

  const proto = `jamsesh.bearer.${token}`;
  const ws = new WebSocket(`/ws/sessions/${sessionId}`, proto);

  const rec: ConnectionRecord = {
    ws,
    status: 'connecting',
    attempt: 0,
    closedByUs: false,
    reconnectTimer: null,
  };
  records.set(sessionId, rec);
  setStatus(sessionId, 'connecting');

  attachListeners(sessionId, ws);
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
 *
 * Cancels any pending reconnect timer and clears the per-session
 * status rune. This is the cursor-invalidation path (a closed-then-
 * resubscribed flow is conceptually "I left the view and came back",
 * which is distinct from the reconnect loop's "I never left").
 */
export function close(sessionId: string): void {
  const rec = records.get(sessionId);
  if (rec) {
    rec.closedByUs = true;
    if (rec.reconnectTimer !== null) {
      clearTimeout(rec.reconnectTimer);
      rec.reconnectTimer = null;
    }
    rec.ws?.close();
    records.delete(sessionId);
  }
  handlers.delete(sessionId);
  setStatus(sessionId, null);
}
