// Tests for the WebSocket subscription manager.
//
// Mocks the global WebSocket constructor so tests run without a live
// server. Mocks client.POST so ticket fetches resolve without a network.
//
// Verifies:
//   - subscribe() POSTs to /api/auth/ws-ticket to get a ticket before opening
//   - subscribe() opens a socket with jamsesh-ticket.<ticket> sub-protocol
//   - A second subscribe() for the same sessionId reuses the socket
//   - Messages of a matching type fire the handler
//   - Messages of a non-matching type do NOT fire the handler
//   - The unsubscribe closure removes the handler
//   - close() tears down the socket and handler registry
//   - Reconnect fetches a fresh ticket (never reuses the original)

import { describe, test, expect, beforeEach, vi, afterEach } from 'vitest';

// ------------------------------------------------------------------
// MockWebSocket — a synchronous test double for global.WebSocket
// ------------------------------------------------------------------

type EventMap = {
  message: MessageEvent[];
  close: CloseEvent[];
  open: Event[];
  error: Event[];
};

class MockWebSocket {
  static instances: MockWebSocket[] = [];

  readonly url: string;
  readonly protocol: string;
  readyState: number = 0; // WebSocket.CONNECTING

  /** Every frame written via send() — order-preserving. */
  readonly sent: string[] = [];

  private listeners: { [K in keyof EventMap]?: Array<(e: EventMap[K][number]) => void> } = {};

  constructor(url: string, protocol?: string | string[]) {
    this.url = url;
    this.protocol = Array.isArray(protocol) ? protocol[0] : (protocol ?? '');
    MockWebSocket.instances.push(this);
  }

  addEventListener<K extends keyof EventMap>(
    type: K,
    listener: (e: EventMap[K][number]) => void,
  ): void {
    if (!this.listeners[type]) this.listeners[type] = [] as any;
    (this.listeners[type] as any[]).push(listener);
  }

  removeEventListener<K extends keyof EventMap>(
    type: K,
    listener: (e: EventMap[K][number]) => void,
  ): void {
    if (!this.listeners[type]) return;
    this.listeners[type] = (this.listeners[type] as any[]).filter(
      (l: any) => l !== listener,
    ) as any;
  }

  /** Trigger a fake message event (test helper). */
  emit(type: 'message', data: unknown): void;
  emit(type: 'close', code?: number): void;
  emit(type: 'open'): void;
  emit(type: string, data?: unknown): void {
    if (type === 'message') {
      const ev = new MessageEvent('message', { data: JSON.stringify(data) });
      ((this.listeners['message'] as any[]) ?? []).forEach((l) => l(ev));
    } else if (type === 'close') {
      // `data` carries the numeric close code when supplied; browsers
      // normalise no-code-supplied closures to 1006 (Abnormal Closure).
      const code = typeof data === 'number' ? data : 1006;
      const ev = new CloseEvent('close', { code });
      ((this.listeners['close'] as any[]) ?? []).forEach((l) => l(ev));
    } else if (type === 'open') {
      const ev = new Event('open');
      ((this.listeners['open'] as any[]) ?? []).forEach((l) => l(ev));
    }
  }

  close(code: number = 1000): void {
    this.readyState = 3; // WebSocket.CLOSED
    this.emit('close', code);
  }

  send(data: string | ArrayBuffer | Blob): void {
    if (typeof data === 'string') {
      this.sent.push(data);
    }
  }
  onopen: ((ev: Event) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  readonly CONNECTING = 0 as const;
  readonly OPEN = 1 as const;
  readonly CLOSING = 2 as const;
  readonly CLOSED = 3 as const;
  readonly bufferedAmount = 0;
  readonly extensions = '';
  readonly binaryType: BinaryType = 'blob';
  readonly dispatchEvent = (_e: Event) => true;
}

// ------------------------------------------------------------------
// Helpers: mock client.POST for ticket fetches
// ------------------------------------------------------------------

/**
 * Install a mock for client.POST that returns the given ticket string.
 * Must be called after vi.resetModules() so the module under test sees
 * the same client instance.
 *
 * We stub the `client` object from '$lib/api/client' so that
 *   client.POST('/api/auth/ws-ticket', {})
 * resolves with { data: { ticket, expires_in_seconds } }.
 */
async function installTicketMock(ticket: string): Promise<void> {
  const { client } = await import('$lib/api/client');
  vi.spyOn(client, 'POST').mockResolvedValue({
    data: { ticket, expires_in_seconds: 60 },
    error: undefined,
    response: new Response(),
  } as any);
}

// ------------------------------------------------------------------
// Fixtures
// ------------------------------------------------------------------

function installMockWebSocket() {
  MockWebSocket.instances = [];
  vi.stubGlobal('WebSocket', MockWebSocket);
}

function uninstallMockWebSocket() {
  vi.unstubAllGlobals();
}

// Helper: flush the microtask queue so that async open()/reopen() complete.
// The ticket fetch resolves via a mocked Promise, so a single await tick is
// enough to let the socket be created.
function flushMicrotasks(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

// ------------------------------------------------------------------
// Tests
// ------------------------------------------------------------------

describe('ws — subscribe / lifecycle', () => {
  beforeEach(async () => {
    installMockWebSocket();
    localStorage.clear();
    vi.resetModules();

    // Prime auth with a token so open() doesn't throw.
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-token', 'test-refresh');

    await installTicketMock('test-ticket-abc');
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.restoreAllMocks();
  });

  test('subscribe() opens a WebSocket for the sessionId', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub = subscribe('sess-1', 'commit.arrived', vi.fn());
    await flushMicrotasks();

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0].url).toBe('/ws/sessions/sess-1');

    unsub();
  });

  test('subscribe() uses jamsesh-ticket.<ticket> sub-protocol (not bearer)', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub = subscribe('sess-2', 'any', vi.fn());
    await flushMicrotasks();

    expect(MockWebSocket.instances[0].protocol).toBe('jamsesh-ticket.test-ticket-abc');
    // Verify the bearer token is NOT in the protocol.
    expect(MockWebSocket.instances[0].protocol).not.toContain('bearer');

    unsub();
  });

  test('subscribe() POSTs to /api/auth/ws-ticket to obtain the ticket', async () => {
    const { client } = await import('$lib/api/client');
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub = subscribe('sess-2b', 'any', vi.fn());
    await flushMicrotasks();

    expect(client.POST).toHaveBeenCalledWith('/api/auth/ws-ticket', {});

    unsub();
  });

  test('second subscribe() to same sessionId reuses the socket (no second ticket fetch)', async () => {
    const { client } = await import('$lib/api/client');
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub1 = subscribe('sess-3', 'event.a', vi.fn());
    await flushMicrotasks();
    const unsub2 = subscribe('sess-3', 'event.b', vi.fn());
    await flushMicrotasks();

    // Still only one WebSocket instance opened, and ticket fetched once.
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(client.POST).toHaveBeenCalledTimes(1);

    unsub1();
    unsub2();
  });

  test('second subscribe() to different sessionId opens a new socket', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    // Install ticket mock to return distinct tickets per call.
    const { client } = await import('$lib/api/client');
    vi.mocked(client.POST)
      .mockResolvedValueOnce({ data: { ticket: 'ticket-4a', expires_in_seconds: 60 }, error: undefined, response: new Response() } as any)
      .mockResolvedValueOnce({ data: { ticket: 'ticket-4b', expires_in_seconds: 60 }, error: undefined, response: new Response() } as any);

    const unsub1 = subscribe('sess-4a', 'ev', vi.fn());
    await flushMicrotasks();
    const unsub2 = subscribe('sess-4b', 'ev', vi.fn());
    await flushMicrotasks();

    expect(MockWebSocket.instances).toHaveLength(2);
    expect(MockWebSocket.instances[0].protocol).toBe('jamsesh-ticket.ticket-4a');
    expect(MockWebSocket.instances[1].protocol).toBe('jamsesh-ticket.ticket-4b');

    unsub1();
    unsub2();
  });

  test('message of matching type fires the handler', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    const unsub = subscribe('sess-5', 'commit.arrived', handler);
    await flushMicrotasks();

    const ws = MockWebSocket.instances[0];
    ws.emit('message', { type: 'commit.arrived', sha: 'abc123' });

    expect(handler).toHaveBeenCalledTimes(1);
    expect(handler).toHaveBeenCalledWith({ type: 'commit.arrived', sha: 'abc123' });

    unsub();
  });

  test('message of non-matching type does not fire the handler', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    const unsub = subscribe('sess-6', 'commit.arrived', handler);
    await flushMicrotasks();

    const ws = MockWebSocket.instances[0];
    ws.emit('message', { type: 'merge.succeeded', draft_sha: 'def456' });

    expect(handler).not.toHaveBeenCalled();

    unsub();
  });

  test('unsubscribe closure removes the handler — no more calls after unsub', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    const unsub = subscribe('sess-7', 'some.event', handler);
    await flushMicrotasks();

    const ws = MockWebSocket.instances[0];
    ws.emit('message', { type: 'some.event', payload: 1 });
    expect(handler).toHaveBeenCalledTimes(1);

    unsub();

    ws.emit('message', { type: 'some.event', payload: 2 });
    // Handler removed — should not have been called again.
    expect(handler).toHaveBeenCalledTimes(1);
  });

  test('multiple handlers for the same type all receive the message', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const h1 = vi.fn();
    const h2 = vi.fn();
    const unsub1 = subscribe('sess-8', 'tick', h1);
    const unsub2 = subscribe('sess-8', 'tick', h2);
    await flushMicrotasks();

    MockWebSocket.instances[0].emit('message', { type: 'tick' });

    expect(h1).toHaveBeenCalledTimes(1);
    expect(h2).toHaveBeenCalledTimes(1);

    unsub1();
    unsub2();
  });

  test('close() closes the socket and clears handlers', async () => {
    const { subscribe, close } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    subscribe('sess-9', 'ev', handler);
    await flushMicrotasks();

    const ws = MockWebSocket.instances[0];
    close('sess-9');

    // After close, emitting on the raw ws must not reach handler because
    // the handler registry was deleted.
    ws.emit('message', { type: 'ev' });
    expect(handler).not.toHaveBeenCalled();
  });

  test('clean close (code 1000) drops the record and next subscribe opens fresh', async () => {
    const { client } = await import('$lib/api/client');
    vi.mocked(client.POST)
      .mockResolvedValueOnce({ data: { ticket: 'ticket-10a', expires_in_seconds: 60 }, error: undefined, response: new Response() } as any)
      .mockResolvedValueOnce({ data: { ticket: 'ticket-10b', expires_in_seconds: 60 }, error: undefined, response: new Response() } as any);

    const { subscribe } = await import('$lib/ws.svelte');

    const unsub1 = subscribe('sess-10', 'ev', vi.fn());
    await flushMicrotasks();
    expect(MockWebSocket.instances).toHaveLength(1);

    // Simulate the server closing cleanly — reconnect is suppressed
    // for code 1000, so the record is dropped.
    MockWebSocket.instances[0].close(1000);

    // Now subscribe again — should open a fresh socket with a new ticket.
    const unsub2 = subscribe('sess-10', 'ev', vi.fn());
    await flushMicrotasks();
    expect(MockWebSocket.instances).toHaveLength(2);
    expect(MockWebSocket.instances[1].protocol).toBe('jamsesh-ticket.ticket-10b');

    unsub1();
    unsub2();
  });

  test('malformed JSON messages are silently dropped', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    const unsub = subscribe('sess-11', 'ev', handler);
    await flushMicrotasks();

    const ws = MockWebSocket.instances[0];
    // Emit a raw MessageEvent with invalid JSON directly.
    const badEv = new MessageEvent('message', { data: '{not json}}}' });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ((ws as any).listeners['message'] ?? []).forEach((l: any) => l(badEv));

    expect(handler).not.toHaveBeenCalled();

    unsub();
  });
});

// ------------------------------------------------------------------
// shouldReconnect predicate
// ------------------------------------------------------------------

describe('ws — shouldReconnect predicate', () => {
  beforeEach(async () => {
    installMockWebSocket();
    localStorage.clear();
    vi.resetModules();
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-token', 'test-refresh');
    await installTicketMock('test-ticket');
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.restoreAllMocks();
  });

  test('suppresses reconnect for clean / fatal codes', async () => {
    const { shouldReconnect } = await import('$lib/ws.svelte');
    expect(shouldReconnect(1000)).toBe(false); // Normal Closure
    expect(shouldReconnect(1003)).toBe(false); // Unsupported Data
    expect(shouldReconnect(1007)).toBe(false); // Invalid Frame Payload
    expect(shouldReconnect(1008)).toBe(false); // Policy Violation (slow-consumer kick)
  });

  test('suppresses reconnect for any 4xxx app-level code', async () => {
    const { shouldReconnect } = await import('$lib/ws.svelte');
    expect(shouldReconnect(4000)).toBe(false);
    expect(shouldReconnect(4401)).toBe(false); // reserved auth-failure
    expect(shouldReconnect(4999)).toBe(false);
    // 5000 is back to "reconnect" — outside the [4000, 5000) range.
    expect(shouldReconnect(5000)).toBe(true);
  });

  test('reconnects on transient / network codes', async () => {
    const { shouldReconnect } = await import('$lib/ws.svelte');
    expect(shouldReconnect(1001)).toBe(true); // Going Away
    expect(shouldReconnect(1006)).toBe(true); // Abnormal Closure (network drop)
    expect(shouldReconnect(1011)).toBe(true); // Internal Error
    expect(shouldReconnect(1012)).toBe(true); // Service Restart
    expect(shouldReconnect(1013)).toBe(true); // Try Again Later
    expect(shouldReconnect(1014)).toBe(true); // Bad Gateway
  });
});

// ------------------------------------------------------------------
// Reconnect loop + backoff timing
// ------------------------------------------------------------------

describe('ws — reconnect loop / exponential backoff', () => {
  beforeEach(async () => {
    installMockWebSocket();
    localStorage.clear();
    vi.resetModules();
    vi.useFakeTimers();
    // Pin Math.random so jitterFactor = 1.0 (factor in [0.75, 1.25]
    // with random=0.5 → 0.75 + 0.5*0.5 = 1.0).
    vi.spyOn(Math, 'random').mockReturnValue(0.5);
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-token', 'test-refresh');
    await installTicketMock('test-ticket');
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  test('unexpected close (1006) schedules reconnect at ~1000ms; opens a fresh socket with a new ticket', async () => {
    const { client } = await import('$lib/api/client');
    vi.mocked(client.POST)
      .mockResolvedValueOnce({ data: { ticket: 'ticket-r1-first', expires_in_seconds: 60 }, error: undefined, response: new Response() } as any)
      .mockResolvedValueOnce({ data: { ticket: 'ticket-r1-second', expires_in_seconds: 60 }, error: undefined, response: new Response() } as any);

    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-r1', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0].protocol).toBe('jamsesh-ticket.ticket-r1-first');

    // Network drop — code 1006.
    MockWebSocket.instances[0].emit('close', 1006);

    // Just before the delay elapses, no new socket yet.
    vi.advanceTimersByTime(999);
    expect(MockWebSocket.instances).toHaveLength(1);

    // At ~1000ms, the reconnect timer fires and opens a fresh socket.
    vi.advanceTimersByTime(2);
    await vi.advanceTimersByTimeAsync(0);
    expect(MockWebSocket.instances).toHaveLength(2);
    expect(MockWebSocket.instances[1].url).toBe('/ws/sessions/sess-r1');
    // Reconnect fetches a NEW ticket — never reuses the first.
    expect(MockWebSocket.instances[1].protocol).toBe('jamsesh-ticket.ticket-r1-second');
    expect(MockWebSocket.instances[1].protocol).not.toBe(MockWebSocket.instances[0].protocol);
  });

  test('second consecutive failure backs off at ~1600ms', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-r2', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    // First drop → ~1000ms wait, then reopen.
    MockWebSocket.instances[0].emit('close', 1006);
    vi.advanceTimersByTime(1000);
    await vi.advanceTimersByTimeAsync(0);
    expect(MockWebSocket.instances).toHaveLength(2);

    // Second drop on the new socket — attempt is now 1, so
    // delay = 1000 * 1.6^1 = 1600ms.
    MockWebSocket.instances[1].emit('close', 1006);
    vi.advanceTimersByTime(1599);
    expect(MockWebSocket.instances).toHaveLength(2);
    vi.advanceTimersByTime(2);
    await vi.advanceTimersByTimeAsync(0);
    expect(MockWebSocket.instances).toHaveLength(3);
  });

  test('clean close (1000) does NOT trigger reconnect', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-r3', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    MockWebSocket.instances[0].emit('close', 1000);

    // Advance well past any plausible backoff — no reconnect.
    vi.advanceTimersByTime(60_000);
    await vi.advanceTimersByTimeAsync(0);
    expect(MockWebSocket.instances).toHaveLength(1);
  });

  test('policy-violation close (1008) does NOT trigger reconnect', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-r4', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    MockWebSocket.instances[0].emit('close', 1008);

    vi.advanceTimersByTime(60_000);
    await vi.advanceTimersByTimeAsync(0);
    expect(MockWebSocket.instances).toHaveLength(1);
  });

  test('explicit close() while reconnect timer pending cancels the loop', async () => {
    const { subscribe, close } = await import('$lib/ws.svelte');
    subscribe('sess-r5', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    // Trigger a reconnect-eligible close.
    MockWebSocket.instances[0].emit('close', 1006);

    // Reconnect timer is now pending. Call close() before it fires.
    close('sess-r5');

    // Advance past the backoff window — no new socket should be created.
    vi.advanceTimersByTime(5_000);
    await vi.advanceTimersByTimeAsync(0);
    expect(MockWebSocket.instances).toHaveLength(1);
  });

  test('handlers fire on the new socket after reconnect', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    const handler = vi.fn();
    subscribe('sess-r6', 'tick', handler);
    await vi.advanceTimersByTimeAsync(0);

    MockWebSocket.instances[0].emit('close', 1006);
    vi.advanceTimersByTime(1000);
    await vi.advanceTimersByTimeAsync(0);

    expect(MockWebSocket.instances).toHaveLength(2);
    MockWebSocket.instances[1].emit('message', { type: 'tick', n: 1 });
    expect(handler).toHaveBeenCalledTimes(1);
    expect(handler).toHaveBeenCalledWith({ type: 'tick', n: 1 });
  });

  test('backoff is capped at 30_000ms', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-r7', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    // Force the attempt counter high by chaining reconnects. After
    // 8 failures the raw exponent (1000 * 1.6^8 ≈ 42 949) exceeds the
    // 30s cap.
    let i = 0;
    for (; i < 9; i++) {
      MockWebSocket.instances[i].emit('close', 1006);
      // Each successive timer fires at min(1000*1.6^i, 30000) — push
      // way past so the loop always advances.
      vi.advanceTimersByTime(31_000);
      await vi.advanceTimersByTimeAsync(0);
    }
    expect(MockWebSocket.instances.length).toBeGreaterThan(8);

    // Now trigger one more close and assert the cap holds.
    MockWebSocket.instances[i].emit('close', 1006);
    const before = MockWebSocket.instances.length;
    vi.advanceTimersByTime(29_999);
    await vi.advanceTimersByTimeAsync(0);
    expect(MockWebSocket.instances.length).toBe(before);
    vi.advanceTimersByTime(2);
    await vi.advanceTimersByTimeAsync(0);
    expect(MockWebSocket.instances.length).toBe(before + 1);
  });

  test('per-session reconnect loops run independently', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-a', 'ev', vi.fn());
    subscribe('sess-b', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    expect(MockWebSocket.instances).toHaveLength(2);

    // Drop session A only.
    const aSocket = MockWebSocket.instances.find((w) => w.url.endsWith('sess-a'))!;
    aSocket.emit('close', 1006);

    vi.advanceTimersByTime(1000);
    await vi.advanceTimersByTimeAsync(0);
    const aSockets = MockWebSocket.instances.filter((w) => w.url.endsWith('sess-a'));
    const bSockets = MockWebSocket.instances.filter((w) => w.url.endsWith('sess-b'));
    expect(aSockets.length).toBe(2); // original + reconnect
    expect(bSockets.length).toBe(1); // untouched
  });
});

// ------------------------------------------------------------------
// wsStatus rune store transitions
// ------------------------------------------------------------------

describe('ws — wsStatus rune store', () => {
  beforeEach(async () => {
    installMockWebSocket();
    localStorage.clear();
    vi.resetModules();
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0.5);
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-token', 'test-refresh');
    await installTicketMock('test-ticket');
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  test('unknown session returns null', async () => {
    const { wsStatus } = await import('$lib/ws.svelte');
    expect(wsStatus.for('never-subscribed')).toBe(null);
  });

  test("first subscribe sets 'connecting', then 'open' on open event", async () => {
    const { subscribe, wsStatus } = await import('$lib/ws.svelte');
    subscribe('sess-s1', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);
    expect(wsStatus.for('sess-s1')).toBe('connecting');

    MockWebSocket.instances[0].emit('open');
    expect(wsStatus.for('sess-s1')).toBe('open');
  });

  test("unexpected close transitions status to 'reconnecting' until next open", async () => {
    const { subscribe, wsStatus } = await import('$lib/ws.svelte');
    subscribe('sess-s2', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);
    MockWebSocket.instances[0].emit('open');
    expect(wsStatus.for('sess-s2')).toBe('open');

    MockWebSocket.instances[0].emit('close', 1006);
    expect(wsStatus.for('sess-s2')).toBe('reconnecting');

    vi.advanceTimersByTime(1000);
    await vi.advanceTimersByTimeAsync(0);
    // New socket is created but has not yet emitted 'open'.
    expect(wsStatus.for('sess-s2')).toBe('reconnecting');

    MockWebSocket.instances[1].emit('open');
    expect(wsStatus.for('sess-s2')).toBe('open');
  });

  test('explicit close() clears the status to null', async () => {
    const { subscribe, close, wsStatus } = await import('$lib/ws.svelte');
    subscribe('sess-s3', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);
    MockWebSocket.instances[0].emit('open');
    expect(wsStatus.for('sess-s3')).toBe('open');

    close('sess-s3');
    expect(wsStatus.for('sess-s3')).toBe(null);
  });

  test('clean close (1000) clears status to null (no reconnect)', async () => {
    const { subscribe, wsStatus } = await import('$lib/ws.svelte');
    subscribe('sess-s4', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);
    MockWebSocket.instances[0].emit('open');
    expect(wsStatus.for('sess-s4')).toBe('open');

    MockWebSocket.instances[0].emit('close', 1000);
    expect(wsStatus.for('sess-s4')).toBe(null);
  });
});

// ------------------------------------------------------------------
// lastSeenSeq cursor + replay_from frame on reconnect
// ------------------------------------------------------------------

describe('ws — lastSeenSeq cursor + replay_from', () => {
  beforeEach(async () => {
    installMockWebSocket();
    localStorage.clear();
    vi.resetModules();
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0.5);
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-token', 'test-refresh');
    await installTicketMock('test-ticket');
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  test('fresh subscribe does not send a replay_from frame on open', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-c1', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    const ws = MockWebSocket.instances[0];
    ws.emit('open');

    // No prior cursor → no frame sent.
    expect(ws.sent).toEqual([]);
  });

  test('reconnect after seeing events sends {"replay_from":<max-seq>} as the first frame', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-c2', 'tick', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    const first = MockWebSocket.instances[0];
    first.emit('open');
    // Pre-drop: receive a few events.
    first.emit('message', { type: 'tick', seq: 1 });
    first.emit('message', { type: 'tick', seq: 2 });
    first.emit('message', { type: 'tick', seq: 3 });

    // Unexpected close → reconnect path.
    first.emit('close', 1006);

    // Advance past backoff so the timer fires and a new socket opens.
    vi.advanceTimersByTime(1000);
    await vi.advanceTimersByTimeAsync(0);
    expect(MockWebSocket.instances).toHaveLength(2);
    const second = MockWebSocket.instances[1];

    // Before 'open' on the new socket, nothing has been written.
    expect(second.sent).toEqual([]);

    // 'open' on the new socket emits exactly one frame: the replay cursor.
    second.emit('open');
    expect(second.sent).toEqual([JSON.stringify({ replay_from: 3 })]);
  });

  test('cursor only moves forward — out-of-order / duplicate seq is a no-op', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-c3', 'tick', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    const first = MockWebSocket.instances[0];
    first.emit('open');
    first.emit('message', { type: 'tick', seq: 5 });
    // Lower seq — should NOT move the cursor back.
    first.emit('message', { type: 'tick', seq: 3 });
    // Duplicate — should NOT advance.
    first.emit('message', { type: 'tick', seq: 5 });

    // Drop + reconnect to surface the cursor on the wire.
    first.emit('close', 1006);
    vi.advanceTimersByTime(1000);
    await vi.advanceTimersByTimeAsync(0);
    const second = MockWebSocket.instances[1];
    second.emit('open');

    expect(second.sent).toEqual([JSON.stringify({ replay_from: 5 })]);
  });

  test('envelopes without a numeric seq do not advance the cursor and do not throw', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-c4', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    const first = MockWebSocket.instances[0];
    first.emit('open');
    first.emit('message', { type: 'ev' }); // no seq
    first.emit('message', { type: 'ev', seq: 'nope' }); // non-numeric
    first.emit('message', { type: 'ev', seq: Number.NaN }); // non-finite
    first.emit('message', { type: 'ev', seq: Infinity }); // non-finite

    // Then a valid one to make sure the path still works.
    first.emit('message', { type: 'ev', seq: 7 });

    first.emit('close', 1006);
    vi.advanceTimersByTime(1000);
    await vi.advanceTimersByTimeAsync(0);
    const second = MockWebSocket.instances[1];
    second.emit('open');

    // Cursor should reflect only the single valid envelope.
    expect(second.sent).toEqual([JSON.stringify({ replay_from: 7 })]);
  });

  test('explicit close() invalidates the cursor — next subscribe sends no replay frame', async () => {
    const { client } = await import('$lib/api/client');
    vi.mocked(client.POST)
      .mockResolvedValueOnce({ data: { ticket: 'ticket-c5a', expires_in_seconds: 60 }, error: undefined, response: new Response() } as any)
      .mockResolvedValueOnce({ data: { ticket: 'ticket-c5b', expires_in_seconds: 60 }, error: undefined, response: new Response() } as any);

    const { subscribe, close } = await import('$lib/ws.svelte');
    subscribe('sess-c5', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    const first = MockWebSocket.instances[0];
    first.emit('open');
    first.emit('message', { type: 'ev', seq: 42 });

    // User leaves the view.
    close('sess-c5');

    // Fresh subscribe — cursor must start at 0.
    subscribe('sess-c5', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);
    expect(MockWebSocket.instances).toHaveLength(2);
    const second = MockWebSocket.instances[1];
    second.emit('open');

    expect(second.sent).toEqual([]);
  });

  test('multiple consecutive reconnects continue to send the latest cursor', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-c6', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    // First socket: see seq 1,2,3 then drop.
    const a = MockWebSocket.instances[0];
    a.emit('open');
    a.emit('message', { type: 'ev', seq: 1 });
    a.emit('message', { type: 'ev', seq: 2 });
    a.emit('message', { type: 'ev', seq: 3 });
    a.emit('close', 1006);

    // Reconnect → second socket sends replay_from:3.
    vi.advanceTimersByTime(1000);
    await vi.advanceTimersByTimeAsync(0);
    const b = MockWebSocket.instances[1];
    b.emit('open');
    expect(b.sent).toEqual([JSON.stringify({ replay_from: 3 })]);

    // Receive more events on the new socket, then drop again.
    b.emit('message', { type: 'ev', seq: 4 });
    b.emit('message', { type: 'ev', seq: 5 });
    b.emit('close', 1006);

    // Second backoff is 1600ms (attempt=1).
    vi.advanceTimersByTime(1600);
    await vi.advanceTimersByTimeAsync(0);
    const c = MockWebSocket.instances[2];
    c.emit('open');
    expect(c.sent).toEqual([JSON.stringify({ replay_from: 5 })]);
  });
});

// ------------------------------------------------------------------
// Unit 1: Reference-counted teardown + linger
// ------------------------------------------------------------------

describe('ws — Unit 1: ref-counted teardown', () => {
  beforeEach(async () => {
    installMockWebSocket();
    localStorage.clear();
    vi.resetModules();
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0.5);
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-token', 'test-refresh');
    await installTicketMock('test-ticket');
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  test('unsubscribing the LAST handler closes the socket and removes the record', async () => {
    const { subscribe, wsStatus } = await import('$lib/ws.svelte');

    const unsub1 = subscribe('sess-t1', 'ev.a', vi.fn());
    const unsub2 = subscribe('sess-t1', 'ev.b', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    expect(MockWebSocket.instances).toHaveLength(1);
    const ws = MockWebSocket.instances[0];
    expect(wsStatus.for('sess-t1')).toBe('connecting');

    // Remove first handler — socket should still be alive (one handler remains).
    unsub1();
    vi.advanceTimersByTime(0);
    await vi.advanceTimersByTimeAsync(0);
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(ws.readyState).not.toBe(3 /* CLOSED */);

    // Remove last handler — teardown should be scheduled.
    unsub2();
    // Linger timer is a macrotask; tick it.
    vi.advanceTimersByTime(0);
    await vi.advanceTimersByTimeAsync(0);

    // ws.close() should have been called (MockWebSocket.close sets readyState=CLOSED).
    expect(ws.readyState).toBe(3 /* CLOSED */);
    // Status must be null (record deleted).
    expect(wsStatus.for('sess-t1')).toBe(null);
  });

  test('unsubscribing ONE of several handlers does NOT close the socket', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub1 = subscribe('sess-t2', 'ev', vi.fn());
    const unsub2 = subscribe('sess-t2', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    const ws = MockWebSocket.instances[0];
    unsub1();
    vi.advanceTimersByTime(100);
    await vi.advanceTimersByTimeAsync(0);

    // Still open.
    expect(ws.readyState).not.toBe(3 /* CLOSED */);

    unsub2();
    vi.advanceTimersByTime(0);
    await vi.advanceTimersByTimeAsync(0);
  });

  test('synchronous unsubscribe-all → resubscribe does NOT close the socket (linger cancels)', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub1 = subscribe('sess-t3', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    const ws = MockWebSocket.instances[0];

    // Simulate a synchronous Svelte effect re-run: unsubscribe then immediately
    // resubscribe — all within the same synchronous turn before any macrotask fires.
    unsub1();
    const unsub2 = subscribe('sess-t3', 'ev', vi.fn()); // cancels the teardown timer

    // Advance macrotask — linger timer would have fired here, but it was cancelled.
    vi.advanceTimersByTime(0);
    await vi.advanceTimersByTimeAsync(0);

    // Socket must still be open.
    expect(ws.readyState).not.toBe(3 /* CLOSED */);
    expect(MockWebSocket.instances).toHaveLength(1);

    unsub2();
    vi.advanceTimersByTime(0);
    await vi.advanceTimersByTimeAsync(0);
  });
});

// ------------------------------------------------------------------
// Unit 2: Reconnect-aware open() — cursor preserved, no orphan timer
// ------------------------------------------------------------------

describe('ws — Unit 2: reconnect-aware open()', () => {
  beforeEach(async () => {
    installMockWebSocket();
    localStorage.clear();
    vi.resetModules();
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0.5);
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-token', 'test-refresh');
    await installTicketMock('test-ticket');
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  test('subscribe() during mid-reconnect does NOT reset lastSeenSeq and does NOT spawn a second timer', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    // Subscribe and open.
    const unsub1 = subscribe('sess-u1', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    const first = MockWebSocket.instances[0];
    first.emit('open');
    // Advance the replay cursor.
    first.emit('message', { type: 'ev', seq: 10 });

    // Drop with reconnectable code → mid-reconnect (ws=null, timer pending).
    first.emit('close', 1006);
    // reconnectTimer is now pending (~1000ms); do NOT advance it.

    // Subscribe again while mid-reconnect. open() should reuse the existing record.
    const unsub2 = subscribe('sess-u1', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    // Still only one socket — open() returned the existing record's ws (null).
    expect(MockWebSocket.instances).toHaveLength(1);

    // Let the reconnect timer fire and the new socket open.
    vi.advanceTimersByTime(1000);
    await vi.advanceTimersByTimeAsync(0);

    expect(MockWebSocket.instances).toHaveLength(2);
    const second = MockWebSocket.instances[1];
    second.emit('open');

    // lastSeenSeq=10 was preserved → replay_from frame sent.
    expect(second.sent).toEqual([JSON.stringify({ replay_from: 10 })]);

    unsub1();
    unsub2();
    vi.advanceTimersByTime(0);
    await vi.advanceTimersByTimeAsync(0);
  });
});

// ------------------------------------------------------------------
// Unit 3: open() resolves null instead of throwing (no floated rejection)
// ------------------------------------------------------------------

describe('ws — Unit 3: open() null on no-token', () => {
  beforeEach(async () => {
    installMockWebSocket();
    localStorage.clear();
    vi.resetModules();
    // Do NOT set a token — auth.token stays null.
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.restoreAllMocks();
  });

  test('open() with falsy auth.token resolves null — no throw, no unhandled rejection, no socket', async () => {
    // Ensure no token is set.
    const { auth } = await import('$lib/auth.svelte');
    expect(auth.token).toBe(null);

    const { subscribe, wsStatus } = await import('$lib/ws.svelte');

    // subscribe() calls void open(); if open() threw, we'd get an unhandled
    // rejection here. The test itself failing would surface that.
    const unsub = subscribe('sess-n1', 'ev', vi.fn());
    await flushMicrotasks();

    // No socket should have been opened.
    expect(MockWebSocket.instances).toHaveLength(0);

    // Status must be null (not 'connecting' or anything else).
    expect(wsStatus.for('sess-n1')).toBe(null);

    unsub();
  });

  test('documented limitation: pre-token handler does NOT auto-open when token is set later', async () => {
    const { auth } = await import('$lib/auth.svelte');
    await installTicketMock('late-ticket');

    const { subscribe } = await import('$lib/ws.svelte');

    // Subscribe without a token — open() returns null, no socket.
    const unsub = subscribe('sess-n2', 'ev', vi.fn());
    await flushMicrotasks();
    expect(MockWebSocket.instances).toHaveLength(0);

    // Now a token arrives (login completed).
    auth.setTokens('new-token', 'new-refresh');
    await flushMicrotasks();

    // The pre-registered handler does NOT trigger an auto-open.
    // A new subscribe() is required to open the connection.
    expect(MockWebSocket.instances).toHaveLength(0);

    unsub();
  });
});

// ------------------------------------------------------------------
// Playground context (anonymous bearer) — open() and reopen() paths
// ------------------------------------------------------------------

describe('ws — playground context (no auth.token, only playgroundContext.bearer)', () => {
  beforeEach(async () => {
    installMockWebSocket();
    localStorage.clear();
    vi.resetModules();
    // Do NOT set auth.token — playground-only credential.
    const { auth } = await import('$lib/auth.svelte');
    auth.setPlaygroundContext({ sessionId: 'pg-sess', bearer: 'pg-bearer-abc', nickname: 'Guest' });

    await installTicketMock('pg-ticket-123');
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.restoreAllMocks();
  });

  test('subscribe() with playgroundContext proceeds to fetch a ticket and open the WS', async () => {
    const { auth } = await import('$lib/auth.svelte');
    // Confirm precondition: no durable token.
    expect(auth.token).toBeNull();
    expect(auth.playgroundContext?.bearer).toBe('pg-bearer-abc');

    const { subscribe } = await import('$lib/ws.svelte');

    const unsub = subscribe('pg-sess', 'event.arrived', vi.fn());
    await flushMicrotasks();

    // A WebSocket must have been opened — not bailed at the !auth.token guard.
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0].url).toBe('/ws/sessions/pg-sess');
    expect(MockWebSocket.instances[0].protocol).toBe('jamsesh-ticket.pg-ticket-123');

    unsub();
  });

  test('subscribe() with no token AND no playgroundContext does NOT open a WS', async () => {
    // Reset modules again to clear the playground context set in beforeEach.
    vi.resetModules();
    // No token, no playground context.
    const { auth } = await import('$lib/auth.svelte');
    expect(auth.token).toBeNull();
    expect(auth.playgroundContext).toBeNull();

    const { subscribe, wsStatus } = await import('$lib/ws.svelte');

    const unsub = subscribe('no-cred-sess', 'ev', vi.fn());
    await flushMicrotasks();

    expect(MockWebSocket.instances).toHaveLength(0);
    expect(wsStatus.for('no-cred-sess')).toBeNull();

    unsub();
  });

  test('reopen() with playgroundContext proceeds (not bailed at the !auth.token guard)', async () => {
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0.5);

    const { client } = await import('$lib/api/client');
    vi.mocked(client.POST)
      .mockResolvedValueOnce({ data: { ticket: 'pg-first', expires_in_seconds: 60 }, error: undefined, response: new Response() } as any)
      .mockResolvedValueOnce({ data: { ticket: 'pg-second', expires_in_seconds: 60 }, error: undefined, response: new Response() } as any);

    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('pg-sess-r', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0].protocol).toBe('jamsesh-ticket.pg-first');

    // Network drop → should reconnect using playground bearer (not bail).
    MockWebSocket.instances[0].emit('close', 1006);

    vi.advanceTimersByTime(1001);
    await vi.advanceTimersByTimeAsync(0);

    expect(MockWebSocket.instances).toHaveLength(2);
    expect(MockWebSocket.instances[1].protocol).toBe('jamsesh-ticket.pg-second');

    vi.useRealTimers();
  });
});

// ------------------------------------------------------------------
// Async cancellation (codex must-fix guards)
// ------------------------------------------------------------------

describe('ws — async cancellation guards', () => {
  beforeEach(async () => {
    installMockWebSocket();
    localStorage.clear();
    vi.resetModules();
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0.5);
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-token', 'test-refresh');
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  test('unsubscribe while fetchTicket is in flight → no WebSocket constructed', async () => {
    // Install a deferred ticket: the promise will not resolve until we let it.
    let resolveTicket!: (ticket: string) => void;
    const deferredTicket = new Promise<string>((res) => {
      resolveTicket = res;
    });

    const { client } = await import('$lib/api/client');
    vi.spyOn(client, 'POST').mockImplementation(async () => {
      const ticket = await deferredTicket;
      return { data: { ticket, expires_in_seconds: 60 }, error: undefined, response: new Response() } as any;
    });

    const { subscribe } = await import('$lib/ws.svelte');

    // Subscribe → open() is now awaiting fetchTicket().
    const unsub = subscribe('sess-ac1', 'ev', vi.fn());

    // Unsubscribe before the ticket resolves — handlerCount drops to 0.
    unsub();
    // Let the linger timer fire (teardown now scheduled).
    vi.advanceTimersByTime(0);
    await vi.advanceTimersByTimeAsync(0);

    // Now resolve the ticket — open() resumes from the await.
    resolveTicket('late-ticket');
    await vi.advanceTimersByTimeAsync(0);

    // The post-ticket guard sees handlerCount===0 and must NOT construct a socket.
    expect(MockWebSocket.instances).toHaveLength(0);
  });

  test('close() while reopen fetchTicket is in flight → no orphan socket attached', async () => {
    // Set up a deferred ticket for the reopen path.
    let resolveReopenTicket!: (ticket: string) => void;
    let callCount = 0;
    const deferredReopenTicket = new Promise<string>((res) => {
      resolveReopenTicket = res;
    });

    const { client } = await import('$lib/api/client');
    vi.spyOn(client, 'POST').mockImplementation(async () => {
      callCount += 1;
      if (callCount === 1) {
        // First call: initial open — resolves immediately.
        return { data: { ticket: 'first-ticket', expires_in_seconds: 60 }, error: undefined, response: new Response() } as any;
      }
      // Second call (reopen): deferred until we resolve it.
      const ticket = await deferredReopenTicket;
      return { data: { ticket, expires_in_seconds: 60 }, error: undefined, response: new Response() } as any;
    });

    const { subscribe, close } = await import('$lib/ws.svelte');

    // Open the initial connection.
    const unsub = subscribe('sess-ac2', 'ev', vi.fn());
    await vi.advanceTimersByTimeAsync(0);
    expect(MockWebSocket.instances).toHaveLength(1);

    // Drop with reconnectable code → reconnect timer pending.
    MockWebSocket.instances[0].emit('close', 1006);

    // Advance past the backoff — reopen() fires and calls fetchTicket()
    // (the deferred one).
    vi.advanceTimersByTime(1000);
    await vi.advanceTimersByTimeAsync(0);
    // reopen() is now suspended at await fetchTicket().

    // Tear down while reopen is in flight.
    unsub();
    vi.advanceTimersByTime(0);
    await vi.advanceTimersByTimeAsync(0);
    // The teardown has deleted the record.

    // Now resolve the deferred ticket — reopen() resumes.
    resolveReopenTicket('orphan-ticket');
    await vi.advanceTimersByTimeAsync(0);

    // The identity guard in reopen() sees records.get(id) !== rec (record was
    // deleted), so it must NOT attach a new socket.
    expect(MockWebSocket.instances).toHaveLength(1); // only the original socket
  });
});
