// Tests for the WebSocket subscription manager.
//
// Mocks the global WebSocket constructor so tests run without a live
// server. Verifies:
//   - subscribe() opens exactly one socket per sessionId
//   - A second subscribe() for the same sessionId reuses the socket
//   - Messages of a matching type fire the handler
//   - Messages of a non-matching type do NOT fire the handler
//   - The unsubscribe closure removes the handler
//   - close() tears down the socket and handler registry

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
  readyState: number = WebSocket.CONNECTING;

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
    this.readyState = WebSocket.CLOSED;
    this.emit('close', code);
  }

  // Satisfy the WebSocket interface for TS — unused in tests.
  send(_data: string | ArrayBuffer | Blob): void {}
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
// Fixtures
// ------------------------------------------------------------------

function installMockWebSocket() {
  MockWebSocket.instances = [];
  vi.stubGlobal('WebSocket', MockWebSocket);
}

function uninstallMockWebSocket() {
  vi.unstubAllGlobals();
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
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.restoreAllMocks();
  });

  test('subscribe() opens a WebSocket for the sessionId', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub = subscribe('sess-1', 'commit.arrived', vi.fn());

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0].url).toBe('/ws/sessions/sess-1');

    unsub();
  });

  test('subscribe() uses jamsesh.bearer.<token> sub-protocol', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub = subscribe('sess-2', 'any', vi.fn());

    expect(MockWebSocket.instances[0].protocol).toBe('jamsesh.bearer.test-token');

    unsub();
  });

  test('second subscribe() to same sessionId reuses the socket', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub1 = subscribe('sess-3', 'event.a', vi.fn());
    const unsub2 = subscribe('sess-3', 'event.b', vi.fn());

    // Still only one WebSocket instance opened.
    expect(MockWebSocket.instances).toHaveLength(1);

    unsub1();
    unsub2();
  });

  test('second subscribe() to different sessionId opens a new socket', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub1 = subscribe('sess-4a', 'ev', vi.fn());
    const unsub2 = subscribe('sess-4b', 'ev', vi.fn());

    expect(MockWebSocket.instances).toHaveLength(2);

    unsub1();
    unsub2();
  });

  test('message of matching type fires the handler', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    const unsub = subscribe('sess-5', 'commit.arrived', handler);

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

    const ws = MockWebSocket.instances[0];
    ws.emit('message', { type: 'merge.succeeded', draft_sha: 'def456' });

    expect(handler).not.toHaveBeenCalled();

    unsub();
  });

  test('unsubscribe closure removes the handler — no more calls after unsub', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    const unsub = subscribe('sess-7', 'some.event', handler);

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

    const ws = MockWebSocket.instances[0];
    close('sess-9');

    // After close, emitting on the raw ws must not reach handler because
    // the handler registry was deleted.
    ws.emit('message', { type: 'ev' });
    expect(handler).not.toHaveBeenCalled();
  });

  test('clean close (code 1000) drops the record and next subscribe opens fresh', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub1 = subscribe('sess-10', 'ev', vi.fn());
    expect(MockWebSocket.instances).toHaveLength(1);

    // Simulate the server closing cleanly — reconnect is suppressed
    // for code 1000, so the record is dropped.
    MockWebSocket.instances[0].close(1000);

    // Now subscribe again — should open a fresh socket.
    const unsub2 = subscribe('sess-10', 'ev', vi.fn());
    expect(MockWebSocket.instances).toHaveLength(2);

    unsub1();
    unsub2();
  });

  test('malformed JSON messages are silently dropped', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    const unsub = subscribe('sess-11', 'ev', handler);

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
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  test('unexpected close (1006) schedules reconnect at ~1000ms; opens a fresh socket', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-r1', 'ev', vi.fn());

    expect(MockWebSocket.instances).toHaveLength(1);

    // Network drop — code 1006.
    MockWebSocket.instances[0].emit('close', 1006);

    // Just before the delay elapses, no new socket yet.
    vi.advanceTimersByTime(999);
    expect(MockWebSocket.instances).toHaveLength(1);

    // At ~1000ms, the reconnect timer fires and opens a fresh socket.
    vi.advanceTimersByTime(2);
    expect(MockWebSocket.instances).toHaveLength(2);
    expect(MockWebSocket.instances[1].url).toBe('/ws/sessions/sess-r1');
    expect(MockWebSocket.instances[1].protocol).toBe('jamsesh.bearer.test-token');
  });

  test('second consecutive failure backs off at ~1600ms', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-r2', 'ev', vi.fn());

    // First drop → ~1000ms wait, then reopen.
    MockWebSocket.instances[0].emit('close', 1006);
    vi.advanceTimersByTime(1000);
    expect(MockWebSocket.instances).toHaveLength(2);

    // Second drop on the new socket — attempt is now 1, so
    // delay = 1000 * 1.6^1 = 1600ms.
    MockWebSocket.instances[1].emit('close', 1006);
    vi.advanceTimersByTime(1599);
    expect(MockWebSocket.instances).toHaveLength(2);
    vi.advanceTimersByTime(2);
    expect(MockWebSocket.instances).toHaveLength(3);
  });

  test('clean close (1000) does NOT trigger reconnect', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-r3', 'ev', vi.fn());

    MockWebSocket.instances[0].emit('close', 1000);

    // Advance well past any plausible backoff — no reconnect.
    vi.advanceTimersByTime(60_000);
    expect(MockWebSocket.instances).toHaveLength(1);
  });

  test('policy-violation close (1008) does NOT trigger reconnect', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-r4', 'ev', vi.fn());

    MockWebSocket.instances[0].emit('close', 1008);

    vi.advanceTimersByTime(60_000);
    expect(MockWebSocket.instances).toHaveLength(1);
  });

  test('explicit close() while reconnect timer pending cancels the loop', async () => {
    const { subscribe, close } = await import('$lib/ws.svelte');
    subscribe('sess-r5', 'ev', vi.fn());

    // Trigger a reconnect-eligible close.
    MockWebSocket.instances[0].emit('close', 1006);

    // Reconnect timer is now pending. Call close() before it fires.
    close('sess-r5');

    // Advance past the backoff window — no new socket should be created.
    vi.advanceTimersByTime(5_000);
    expect(MockWebSocket.instances).toHaveLength(1);
  });

  test('handlers fire on the new socket after reconnect', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    const handler = vi.fn();
    subscribe('sess-r6', 'tick', handler);

    MockWebSocket.instances[0].emit('close', 1006);
    vi.advanceTimersByTime(1000);

    expect(MockWebSocket.instances).toHaveLength(2);
    MockWebSocket.instances[1].emit('message', { type: 'tick', n: 1 });
    expect(handler).toHaveBeenCalledTimes(1);
    expect(handler).toHaveBeenCalledWith({ type: 'tick', n: 1 });
  });

  test('backoff is capped at 30_000ms', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-r7', 'ev', vi.fn());

    // Force the attempt counter high by chaining reconnects. After
    // 8 failures the raw exponent (1000 * 1.6^8 ≈ 42 949) exceeds the
    // 30s cap.
    let i = 0;
    for (; i < 9; i++) {
      MockWebSocket.instances[i].emit('close', 1006);
      // Each successive timer fires at min(1000*1.6^i, 30000) — push
      // way past so the loop always advances.
      vi.advanceTimersByTime(31_000);
    }
    expect(MockWebSocket.instances.length).toBeGreaterThan(8);

    // Now trigger one more close and assert the cap holds.
    MockWebSocket.instances[i].emit('close', 1006);
    const before = MockWebSocket.instances.length;
    vi.advanceTimersByTime(29_999);
    expect(MockWebSocket.instances.length).toBe(before);
    vi.advanceTimersByTime(2);
    expect(MockWebSocket.instances.length).toBe(before + 1);
  });

  test('per-session reconnect loops run independently', async () => {
    const { subscribe } = await import('$lib/ws.svelte');
    subscribe('sess-a', 'ev', vi.fn());
    subscribe('sess-b', 'ev', vi.fn());

    expect(MockWebSocket.instances).toHaveLength(2);

    // Drop session A only.
    const aSocket = MockWebSocket.instances.find((w) => w.url.endsWith('sess-a'))!;
    aSocket.emit('close', 1006);

    vi.advanceTimersByTime(1000);
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
    expect(wsStatus.for('sess-s1')).toBe('connecting');

    MockWebSocket.instances[0].emit('open');
    expect(wsStatus.for('sess-s1')).toBe('open');
  });

  test("unexpected close transitions status to 'reconnecting' until next open", async () => {
    const { subscribe, wsStatus } = await import('$lib/ws.svelte');
    subscribe('sess-s2', 'ev', vi.fn());
    MockWebSocket.instances[0].emit('open');
    expect(wsStatus.for('sess-s2')).toBe('open');

    MockWebSocket.instances[0].emit('close', 1006);
    expect(wsStatus.for('sess-s2')).toBe('reconnecting');

    vi.advanceTimersByTime(1000);
    // New socket is created but has not yet emitted 'open'.
    expect(wsStatus.for('sess-s2')).toBe('reconnecting');

    MockWebSocket.instances[1].emit('open');
    expect(wsStatus.for('sess-s2')).toBe('open');
  });

  test('explicit close() clears the status to null', async () => {
    const { subscribe, close, wsStatus } = await import('$lib/ws.svelte');
    subscribe('sess-s3', 'ev', vi.fn());
    MockWebSocket.instances[0].emit('open');
    expect(wsStatus.for('sess-s3')).toBe('open');

    close('sess-s3');
    expect(wsStatus.for('sess-s3')).toBe(null);
  });

  test('clean close (1000) clears status to null (no reconnect)', async () => {
    const { subscribe, wsStatus } = await import('$lib/ws.svelte');
    subscribe('sess-s4', 'ev', vi.fn());
    MockWebSocket.instances[0].emit('open');
    expect(wsStatus.for('sess-s4')).toBe('open');

    MockWebSocket.instances[0].emit('close', 1000);
    expect(wsStatus.for('sess-s4')).toBe(null);
  });
});
