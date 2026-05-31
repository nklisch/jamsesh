// WebSocket subscription manager, keyed by sessionId.
//
// Each call to subscribe() for a given sessionId reuses the same
// underlying WebSocket. Messages are routed to handlers filtered by
// event `type`. subscribe() returns an unsubscribe closure.
//
// Auth: before opening a WebSocket the module POSTs to
//   POST /api/auth/ws-ticket
// to obtain a short-lived (60 s) single-use ticket, then presents it as
//   Sec-WebSocket-Protocol: jamsesh-ticket.<ticket>
// This keeps the long-lived bearer token out of the Sec-WebSocket-Protocol
// header entirely. On reconnect a fresh ticket is fetched each time.
//
// Reconnect: on an unexpected close (see shouldReconnect() below), the
// module schedules a reopen with exponential backoff (1.0s base, 30s
// cap, 1.6× multiplier, ±25% jitter). On a clean / fatal close — or
// when close(sessionId) is called explicitly — the record is dropped
// and the loop stops. The per-session status is surfaced reactively
// via the `wsStatus` rune store.
//
// Write invariant: nothing else writes to the socket today — the
// client doesn't issue commands over WS. On reconnect, the 'open'
// listener sends a `{"replay_from": <lastSeenSeq>}` frame FIRST,
// before any other writes, because the portal accepts that frame
// only as the first text frame after upgrade (gateway.go:215). The
// reconnect path runs synchronously inside the 'open' listener so the
// ordering invariant holds. On a fresh subscribe (lastSeenSeq === 0),
// no replay frame is sent — the portal's 2-second timer falls through
// and the stream goes straight to live.
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
import { client } from '$lib/api/client';

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
  // Highest envelope seq seen so far for this session. Bumped only
  // forward inside the 'message' handler; consumed by the reconnect
  // path to emit a `{"replay_from": <lastSeenSeq>}` first frame.
  // Plain field, not a rune — nothing reactive consumes it (see D2 in
  // the feature design).
  lastSeenSeq: number;
  // Pending macrotask teardown timer (Unit 1 ref-counted teardown).
  // Set by scheduleTeardown(); cancelled on entry to subscribe() or
  // explicit close(). Fires teardown() if handlerCount is still 0.
  teardownTimer: ReturnType<typeof setTimeout> | null;
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

/**
 * Fetch a single-use WS upgrade ticket from the portal.
 *
 * Returns the opaque ticket string on success, or null if the request
 * fails (network error, 401, etc.). A null result causes the open/reopen
 * paths to abort the connection attempt and drop the record.
 */
async function fetchTicket(): Promise<string | null> {
  try {
    const { data } = await client.POST('/api/auth/ws-ticket', {});
    return data?.ticket ?? null;
  } catch {
    return null;
  }
}

/**
 * Count all handlers currently registered for a session across all types.
 * Used to determine whether a teardown should proceed.
 */
function handlerCount(sessionId: string): number {
  const byType = handlers.get(sessionId);
  if (!byType) return 0;
  let n = 0;
  for (const set of byType.values()) n += set.size;
  return n;
}

/**
 * Tear down the connection for a session: close the socket, cancel the
 * reconnect timer, remove the record and handler registry, and clear the
 * status. This is the body that was formerly inline in close().
 *
 * Tearing down intentionally drops lastSeenSeq (cursor invalidation —
 * "I left the view and came back", distinct from the reconnect loop's
 * "I never left").
 */
function teardown(sessionId: string): void {
  const rec = records.get(sessionId);
  if (rec) {
    rec.closedByUs = true;
    if (rec.teardownTimer !== null) {
      clearTimeout(rec.teardownTimer);
      rec.teardownTimer = null;
    }
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

/**
 * Schedule a deferred (macrotask) teardown for a session.
 *
 * The macrotask linger absorbs the synchronous Svelte 5 effect
 * cleanup→re-subscribe window: $effect teardown runs immediately before
 * the synchronous rerun, so a macrotask timer cannot fire between cleanup
 * and the synchronous rerun body. If a new subscribe() arrives before the
 * timer fires, it cancels the teardown.
 *
 * If no record exists (already torn down), cleans up the handler map and
 * status immediately.
 */
function scheduleTeardown(sessionId: string): void {
  const rec = records.get(sessionId);
  if (!rec) {
    handlers.delete(sessionId);
    setStatus(sessionId, null);
    return;
  }
  // Use ??= so only one timer is ever pending per record.
  rec.teardownTimer ??= setTimeout(() => {
    rec.teardownTimer = null;
    if (handlerCount(sessionId) === 0) teardown(sessionId);
  }, 0);
}

function attachListeners(sessionId: string, ws: WebSocket): void {
  ws.addEventListener('message', (ev: MessageEvent) => {
    let env: EventEnvelope;
    try {
      env = JSON.parse(ev.data as string) as EventEnvelope;
    } catch {
      return;
    }
    const rec = records.get(sessionId);
    // Advance the replay cursor only forward. Envelopes without a
    // numeric `seq` (or with non-finite seq) do not move it.
    if (
      rec &&
      typeof env.seq === 'number' &&
      Number.isFinite(env.seq) &&
      env.seq > rec.lastSeenSeq
    ) {
      rec.lastSeenSeq = env.seq;
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
    // Reconnect replay: if we've already seen events for this session,
    // ask the portal to replay everything past our cursor. Sent
    // synchronously here, before any other write, so the portal's
    // first-frame contract (gateway.go:215) is satisfied. A fresh
    // subscribe has lastSeenSeq === 0 and sends nothing.
    if (rec.lastSeenSeq > 0) {
      ws.send(JSON.stringify({ replay_from: rec.lastSeenSeq }));
    }
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
      void reopen(sessionId);
    }, delay);
  });
}

async function reopen(sessionId: string): Promise<void> {
  // Capture the record identity BEFORE the async gap. After fetchTicket()
  // resolves, we require the SAME record object still to be present AND
  // live handlers to exist — a teardown + fresh resubscribe may have
  // replaced the record during the await, and an old reopen must not attach
  // an orphan socket to the new record.
  const rec = records.get(sessionId);
  if (!rec) return;

  if (!auth.token && !auth.playgroundContext?.bearer) {
    // No bearer token — can't obtain a ticket. Give up and drop the record.
    records.delete(sessionId);
    setStatus(sessionId, null);
    return;
  }

  const ticket = await fetchTicket();
  if (!ticket) {
    // Ticket fetch failed (auth expired, network error). Drop the record.
    records.delete(sessionId);
    setStatus(sessionId, null);
    return;
  }

  // Post-ticket identity guard (codex must-fix): require the SAME record
  // object (identity, not just existence) AND live handlers. A teardown +
  // fresh resubscribe may have swapped the record during the await; an
  // in-flight reopen must not attach an orphan socket.
  if (records.get(sessionId) !== rec || handlerCount(sessionId) === 0) return;

  const proto = `jamsesh-ticket.${ticket}`;
  const ws = new WebSocket(`/ws/sessions/${sessionId}`, proto);
  rec.ws = ws;
  attachListeners(sessionId, ws);
}

async function open(sessionId: string): Promise<WebSocket | null> {
  // Unit 2: reuse any existing record — whether ws is open or null
  // (mid-reconnect). A lingering record always means "open or reconnecting";
  // the reconnect loop owns it. Do NOT overwrite/reset lastSeenSeq or orphan
  // the in-flight reconnectTimer.
  const existing = records.get(sessionId);
  if (existing) return existing.ws;

  // Unit 3: resolve null instead of throwing — void open() in subscribe() is
  // then safe (no floated rejection). Surface disconnected status.
  if (!auth.token && !auth.playgroundContext?.bearer) {
    setStatus(sessionId, null);
    return null;
  }

  const ticket = await fetchTicket();
  if (!ticket) {
    // Ticket fetch failed — can't open the socket.
    return null;
  }

  // Post-ticket guard (codex must-fix): an unsubscribe/close/teardown may
  // have landed during the await. Re-check BEFORE constructing the socket.
  const now = records.get(sessionId);
  if (now) return now.ws; // someone created/owns a record meanwhile — reuse it
  if (handlerCount(sessionId) === 0) return null; // last handler left during the await — don't orphan a socket

  const proto = `jamsesh-ticket.${ticket}`;
  const ws = new WebSocket(`/ws/sessions/${sessionId}`, proto);

  const rec: ConnectionRecord = {
    ws,
    status: 'connecting',
    attempt: 0,
    closedByUs: false,
    reconnectTimer: null,
    lastSeenSeq: 0,
    teardownTimer: null,
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
 * otherwise reuses the existing connection. A ticket is fetched from
 * POST /api/auth/ws-ticket before each new socket is opened — the
 * bearer token is never placed in Sec-WebSocket-Protocol.
 *
 * Returns an unsubscribe closure that removes only this handler.
 * When the last handler for a session is removed, a brief macrotask
 * linger is scheduled before tearing down the connection — this absorbs
 * the synchronous Svelte 5 effect cleanup→re-subscribe window so that
 * an effect re-run does not thrash the socket.
 */
export function subscribe(
  sessionId: string,
  type: string,
  handler: Handler,
): () => void {
  // Cancel any pending teardown — a new subscriber has arrived.
  const pendingRec = records.get(sessionId);
  if (pendingRec?.teardownTimer !== null && pendingRec?.teardownTimer !== undefined) {
    clearTimeout(pendingRec.teardownTimer);
    pendingRec.teardownTimer = null;
  }

  void open(sessionId);

  if (!handlers.has(sessionId)) handlers.set(sessionId, new Map());
  const byType = handlers.get(sessionId)!;
  if (!byType.has(type)) byType.set(type, new Set());
  byType.get(type)!.add(handler);

  return () => {
    const set = byType.get(type);
    if (set) {
      set.delete(handler);
      // Prune empty per-type Sets to avoid accumulating empty sets over
      // long-lived churn across event types.
      if (set.size === 0) byType.delete(type);
    }
    // If this was the last handler for the session, schedule a deferred
    // teardown. The macrotask linger absorbs the synchronous Svelte 5
    // effect cleanup→re-subscribe window.
    if (handlerCount(sessionId) === 0) scheduleTeardown(sessionId);
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
  // Cancel any pending linger teardown, then execute teardown immediately.
  const rec = records.get(sessionId);
  if (rec?.teardownTimer !== null && rec?.teardownTimer !== undefined) {
    clearTimeout(rec.teardownTimer);
    rec.teardownTimer = null;
  }
  teardown(sessionId);
}
